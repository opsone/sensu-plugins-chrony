#!/usr/bin/env bash
# Runs check-chrony against a real chronyd synchronised to NTP servers in Docker.
set -euo pipefail
cd "$(dirname "$0")"

id=$$
image=check-chrony-integration
network=check-chrony-$id
good=check-chrony-ntp-good-$id
fast=check-chrony-ntp-fast-$id
runner=check-chrony-runner-$id

docker build -q -t "$image" . >/dev/null
docker network create "$network" >/dev/null
cleanup() {
  docker rm -f "$runner" "$good" "$fast" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# NTP servers serving their local clock; the "fast" one runs 300ms ahead (libfaketime).
# -x: chronyd never touches the host clock, no privilege needed.
server_conf='printf "local stratum 3\nallow all\n" > /etc/chrony/chrony.conf'
for c in "$good" "$fast"; do
  docker run -d --init --name "$c" --network "$network" "$image" >/dev/null
done
docker exec "$good" bash -c "$server_conf && chronyd -x"
docker exec "$fast" bash -c "$server_conf && LD_PRELOAD=\$(echo /usr/lib/*/faketime/libfaketimeMT.so.1) FAKETIME=+0.3 chronyd -x"
docker run -d --init --name "$runner" --network "$network" "$image" >/dev/null

# (Re)starts the client chronyd with the given NTP server, polling every 250ms.
client() {
  docker exec "$runner" bash -c "pkill -x chronyd; while pgrep -x chronyd >/dev/null; do sleep 0.1; done
    printf 'server $1 iburst minpoll -2 maxpoll -2\n' > /etc/chrony/chrony.conf && chronyd -x"
}
wait_synced() {
  for _ in $(seq 30); do
    docker exec "$runner" chronyc -n -c tracking | grep -q ',Normal$' && return
    sleep 1
  done
  echo "chronyd did not synchronise" >&2
  exit 1
}

failures=0
expect() {
  local want=$1 desc=$2; shift 2
  local out rc=0
  # Run as an unprivileged user, like sensu-agent does.
  out=$(docker exec "$runner" runuser -u nobody -- check-chrony "$@" 2>&1) || rc=$?
  out=$(tail -1 <<<"$out")
  if [[ $rc -eq $want ]]; then
    echo "ok   [$rc] $desc -> $out"
  else
    echo "FAIL [$rc, want $want] $desc -> $out"
    failures=$((failures + 1))
  fi
}

client "$good"
wait_synced
expect 0 "synchronised"
expect 3 "warn offset above crit offset" --warn-offset 200 --crit-offset 100
expect 3 "negative age" --warn-age -1
expect 3 "chronyc not found" -c /nope/chronyc

docker exec "$good" pkill -x chronyd
sleep 3
expect 1 "server lost, last sync too old (warning)" --warn-age 2
expect 2 "server lost, last sync too old (critical)" --warn-age 1 --crit-age 2

client "$fast"
wait_synced
expect 2 "clock 150ms+ off"
expect 1 "clock off, warning only" --crit-offset 1000
expect 0 "clock off, within thresholds" --warn-offset 900 --crit-offset 1000

client 192.0.2.1
expect 2 "no reachable server, never synchronised"

docker exec "$runner" pkill -x chronyd
expect 2 "chronyd stopped"

exit $failures
