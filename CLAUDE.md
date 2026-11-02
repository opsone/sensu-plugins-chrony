# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Sensu Go check verifying that the system clock is synchronised by chrony, written in Go with `github.com/sensu/sensu-plugin-sdk`. Published as a Sensu asset (static binary, `linux/amd64` only, see `ARCHS` in the Makefile). Same layout and tooling as `opsone/sensu-plugins-systemd` (single binary, sources at the root).

## Common Commands

```bash
make test          # go vet + unit tests
make build         # local binary at the root
make integration   # Docker: two chronyd NTP servers (one 300ms ahead via libfaketime) + a chronyd -x client
make dist          # tests, then tar.gz per arch + sha512 + asset YAML in dist/
make release       # dist + GitHub release of the current tag (gh)
```

Version comes from `git describe --tags` (override with `VERSION=`), injected via ldflags into the SDK `version` package.

## Architecture

- `main.go`: runs `chronyc -n -c tracking` (CSV, 14 fields), `parseTracking` then `evaluate` (pure, table-tested in `main_test.go`).
- States: `Not synchronised` or chronyc failure (daemon down) → CRITICAL; offset (`System time`, CSV positive = slow) and age of `Ref time` against warn/crit thresholds; invalid args, chronyc not found, unparsable output → UNKNOWN.
- Options are declared with `sensu.PluginConfigOption`, each with a `CHECK_CHRONY_*` env var. `-h` is reserved by cobra for `--help` (redefining it panics).
- Integration containers run with `--init`: PID 1 must reap the chronyd processes killed between scenarios.
