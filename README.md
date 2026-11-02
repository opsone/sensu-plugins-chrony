# check-chrony

Sensu Go check verifying that the system clock is synchronised by [chrony](https://chrony-project.org/).

Reads `chronyc -c tracking` (CSV output, fixed fields). Static Go binary (no CGO, no Ruby runtime): one asset for every Linux distribution, `amd64` only.

## Behavior

| Situation | State |
|---|---|
| Synchronised, offset and last update within thresholds | OK |
| System clock off by `--warn-offset` ms or more (default 50) | WARNING |
| System clock off by `--crit-offset` ms or more (default 100) | CRITICAL |
| Last clock update `--warn-age` seconds old or more (default 3600) | WARNING |
| Last clock update `--crit-age` seconds old or more (default 14400) | CRITICAL |
| Clock not synchronised (`Leap status: Not synchronised`) | CRITICAL |
| chronyd stopped or not answering | CRITICAL |
| Invalid argument, `chronyc` not found, unreadable output | UNKNOWN |

- The offset is the current offset of the system clock from NTP time (`System time` in `chronyc tracking`), slow or fast.
- The age of the last update catches NTP servers becoming unreachable: chrony then keeps reporting a synchronised clock (free-running on its last estimate) and only the reference time stops moving.
- The worst state wins; the output always shows the source, stratum, offset and age:

```
check-chrony OK: synchronised to 192.0.2.10 (stratum 2), offset 0.012ms slow, last sync 1m4s ago
check-chrony CRITICAL: synchronised to 192.0.2.10 (stratum 2), offset 150.040ms slow (critical >= 100ms), last sync 2s ago
check-chrony CRITICAL: clock not synchronised (no usable NTP source)
check-chrony CRITICAL: chronyc tracking failed: 506 Cannot talk to daemon
```

## Usage

```
check-chrony [flags]

  -c, --chronyc-cmd string   Path to the chronyc binary (default "chronyc")
      --warn-offset float    Warning if the system clock is off by this many milliseconds or more (default 50)
      --crit-offset float    Critical if the system clock is off by this many milliseconds or more (default 100)
      --warn-age int         Warning if the last clock update is this many seconds old or more (default 3600)
      --crit-age int         Critical if the last clock update is this many seconds old or more (default 14400)
      --timeout int          chronyc timeout in seconds (default 10)
```

Every option can also be set with an environment variable:

| Option | Variable | Annotation |
|---|---|---|
| `--chronyc-cmd` | `CHECK_CHRONY_CHRONYC_CMD` | `chrony_chronyc_cmd` |
| `--warn-offset` / `--crit-offset` | `CHECK_CHRONY_WARN_OFFSET` / `CHECK_CHRONY_CRIT_OFFSET` | `chrony_warn_offset` / `chrony_crit_offset` |
| `--warn-age` / `--crit-age` | `CHECK_CHRONY_WARN_AGE` / `CHECK_CHRONY_CRIT_AGE` | `chrony_warn_age` / `chrony_crit_age` |
| `--timeout` | `CHECK_CHRONY_TIMEOUT` | `chrony_timeout` |

The Sensu agent does not need to run as root: `chronyc tracking` is answered to any local user (UDP 127.0.0.1:323, chrony default `cmdallow`).

## Check definition

Every option is passed through `env_vars`, filled per host from entity annotations with token substitution (applied by the agent to the whole check definition, `env_vars` included).

Naming rule: an annotation is named after its environment variable, lowercased, without the `CHECK_` prefix (`CHECK_CHRONY_WARN_OFFSET` → `chrony_warn_offset`). All options are optional: `| default ""` leaves the variable empty, which is ignored, so the option keeps its built-in default.

```yaml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-chrony
spec:
  command: check-chrony
  env_vars:
    - CHECK_CHRONY_CHRONYC_CMD={{ .annotations.chrony_chronyc_cmd | default "" }}
    - CHECK_CHRONY_WARN_OFFSET={{ .annotations.chrony_warn_offset | default "" }}
    - CHECK_CHRONY_CRIT_OFFSET={{ .annotations.chrony_crit_offset | default "" }}
    - CHECK_CHRONY_WARN_AGE={{ .annotations.chrony_warn_age | default "" }}
    - CHECK_CHRONY_CRIT_AGE={{ .annotations.chrony_crit_age | default "" }}
    - CHECK_CHRONY_TIMEOUT={{ .annotations.chrony_timeout | default "" }}
  runtime_assets:
    - opsone/sensu-plugins-chrony
  interval: 300
  timeout: 30
  publish: true
  subscriptions:
    - linux
```

Annotations, only where the defaults do not fit, e.g. in the agent configuration (`/etc/sensu/agent.yml`):

```yaml
annotations:
  chrony_warn_offset: "200"     # ms
  chrony_crit_offset: "500"     # ms
  chrony_crit_age: "86400"      # s
```

## Migrating from the Ruby version (≤ 0.1.1)

- The command loses its `.rb` suffix: `check-chrony.rb` → `check-chrony`.
- `--warn-stratum` / `--crit-stratum` are removed (a stratum of 16 means "not synchronised", now detected directly). A check definition still passing them fails with UNKNOWN (unknown flag).
- The offset checked is the current offset of the system clock (`System time`) instead of the offset measured at the last update (`Last offset`).
- New CRITICAL states: clock not synchronised (was OK when the offset was small), chronyd stopped or unreachable (was UNKNOWN).
- New check of the age of the last clock update (`--warn-age`, `--crit-age`).
- `--chronyc-cmd` defaults to `chronyc` looked up in `PATH` (was `/usr/bin/chronyc`).
- The asset no longer depends on the Ruby runtime asset, nor on the Debian release.

## Build and release

```sh
make test          # go vet + unit tests
make integration   # runs the check against real chronyd instances in Docker
```

Release:

```sh
git tag 0.2.0 && git push origin 0.2.0
make release       # version taken from the tag, or forced: make release VERSION=0.2.0
sensuctl create -f dist/opsone-sensu-plugins-chrony_0.2.0.yml
```

`make release` runs `make dist`, which runs the tests then produces in `dist/`:

- one archive per architecture listed in `ARCHS` (`linux/amd64`) in Sensu asset layout (`bin/check-chrony`);
- the sha512 checksums file;
- `opsone-sensu-plugins-chrony_<version>.yml`, the asset definition pointing to the GitHub release of the tag.

It then creates the GitHub release of the tag with `gh`, uploads the archives and checksums, and uses the asset definition as release description.

`check-chrony version` prints the version, commit and build date.
