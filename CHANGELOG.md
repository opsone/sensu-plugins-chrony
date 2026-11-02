# Changelog

## [0.2.0] - 2026-11-02

### Changed
- Rewritten in Go: static binary `check-chrony` (was `check-chrony.rb`), no Ruby runtime, one asset for every Debian release.
- The offset checked is the current offset of the system clock (`System time`) instead of `Last offset`.
- Clock not synchronised and chronyd stopped/unreachable are CRITICAL (were OK / UNKNOWN).
- `--chronyc-cmd` defaults to `chronyc` in `PATH`.
- Every option can be set with a `CHECK_CHRONY_*` environment variable.

### Added
- `--warn-age` / `--crit-age`: age of the last clock update (catches unreachable NTP servers).
- `--timeout` for `chronyc`.

### Removed
- `--warn-stratum` / `--crit-stratum`.

## [0.1.1]

Last Ruby version.
