# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.3.3] - 2026-04-14

### Added

- **pkg/connect**: native `TelnetSession` (minimal RFC 854 negotiation, username/password prompts, shared command/prompt loop with SSH)
- **cmd/rancid-ui**: read-only local web server — fleet view from all `router.db` files, per-device on-disk config browser (Prism.js from cdnjs), last `git log -1 -p` diff per `configs/<hostname>`
- **pkg/git**: `LastCommitPatch` helper for the UI

### Changed

- **pkg/connect**: `NewSession` returns `(Session, error)` and selects **ssh** or **telnet** from `.cloginrc` `method` list order; `ErrNoNativeTransport` when neither is usable
- **cmd/clogin**: native mode uses `NewSession` (SSH or Telnet); duplicate parser registration removed in favor of `devicetype.RegisterMissingParsers`
- **cmd/control-rancid**: logs an explicit `.cloginrc` hint when collection fails with `ErrNoNativeTransport`

## [0.3.2] - 2026-04-14

### Added

- **pkg/collect**: `GoCollector` and `CollectDevice` — shared in-process collection (SSH + parsers) used by `cmd/rancid` and `cmd/control-rancid`
- **pkg/devicetype**: `RegisterMissingParsers` — centralizes dynamic parser alias / generic registration

### Changed

- **cmd/control-rancid**: uses `GoCollector` instead of shelling out to the `rancid` binary
- **pkg/connect**: removed Expect/clogin subprocess `Session` implementation; `NewSession` returns a native session and requires a usable `method` entry in `.cloginrc` when native collection is requested
- **pkg/parse/generic**: `DeviceOpts()` enables native SSH for long-tail device types (broad prompt heuristic)

### Removed

- **pkg/collect**: `FallbackCollector` (Perl `rancid` subprocess)
- **pkg/connect**: `ExpectSession` / `expect.go`

## [0.3.1] - 2026-04-14

### Changed

- **cmd/clogin**: Added `-C` / `-config` support for selecting `rancid.conf`, and derived `rancid.types.*` lookup from the chosen config path
- **pkg/config**: `router.db` parsing now accepts both `hostname:type:status` and `hostname;type;status`
- **pkg/config**: `.cloginrc` parsing now supports multi-value `password` entries, `userpassword`, and braced method definitions
- **pkg/connect**: Native SSH now supports keyboard-interactive auth and proper interactive terminal behavior (raw mode, PTY sizing, resize propagation)
- **cmd/clogin**: Native vs legacy login selection now follows device-family behavior more accurately, with explicit transport notices
- **pkg/parse**: Prompt detection widened for Cisco, Juniper, and FortiGate interactive sessions so native SSH login no longer stalls on initial prompts

## [0.3.0] - 2026-04-14

### Added

- **cmd/clogin**: Go replacement for `clogin` / `plogin` with `.cloginrc` support, `router.db` device-type lookup, native SSH for supported parsers, and fallback to legacy login scripts where required
- **cmd/rancid**: Full Go parser coverage across the `rancid.types.{base,conf}` surface via dedicated parsers, alias registration, and a generic parser for long-tail device types
- **pkg/parse/generic**: Generic Go parser for device types without a dedicated parser implementation yet

### Changed

- **pkg/connect**: SSH sessions now correctly execute setup and enable commands before interactive use
- **pkg/connect**: Session selection now prefers native SSH transport only when SSH methods are available, otherwise falling back to legacy login scripts
- **ci**: Build pipeline now publishes `clogin` and `rancid` artifacts and uses a Go toolchain compatible with `go.mod`
- **roadmap**: Phase 3 marked complete; Phase 4 now tracks transport dependency removal and the Web UI work

## [0.2.0] - 2026-04-09

### Added

- **pkg/connect**: Native SSH connector using `golang.org/x/crypto/ssh` with PTY allocation, prompt detection, and command execution
- **pkg/connect**: Expect subprocess fallback for legacy login-script execution
- **pkg/parse**: Device parser registry and shared filter options
- **pkg/parse/ios**: Cisco IOS parser with metadata extraction and RANCID-style filtering
- **pkg/parse/iosxr**: Cisco IOS-XR parser with metadata extraction and config normalization
- **pkg/parse/junos**: Juniper JunOS parser with version/config parsing and retry-trigger handling
- **pkg/parse/nxos**: Cisco NX-OS parser with metadata extraction and config filtering
- **pkg/parse/fortigate**: Fortinet FortiGate parser with system-status/config parsing and secret filtering
- **cmd/rancid**: Per-device collector binary replacing the Perl `rancid` entrypoint for supported device types

### Changed

- **README / ROADMAP**: Project status updated from orchestration-only to native collection plus core parsers

## [0.1.0] - 2026-04-09

### Added

- **pkg/config**: Parse `rancid.conf` into `config.Config` (env-var style KEY=value format, including BASEDIR, LIST_OF_GROUPS, FILTER_PWDS, FILTER_OSC, PAR_COUNT, MAILDOMAIN, MAILHEADERS, MAILOPTS, SENDMAIL)
- **pkg/config**: Parse `router.db` into `[]config.Device` (hostname:type:status format, comments and blank lines ignored)
- **pkg/config**: Parse `.cloginrc` into `config.CredStore` with first-match-wins glob lookup (add user/password/enablepassword/method directives)
- **pkg/devicetype**: Load `rancid.types.base` and `rancid.types.conf` with base-type protection (conf cannot override base types), alias resolution with cycle detection
- **pkg/git**: Git subprocess wrapper — `Init`, `Add`, `Commit`, `Diff` (no libgit2 dependency)
- **pkg/par**: Parallel worker pool bounded by `PAR_COUNT`, returns indexed results
- **pkg/collect**: `FallbackCollector` delegates per-device collection to the original Perl `rancid` binary — non-zero exit treated as `StatusFailed`, not a hard error
- **pkg/notify**: `BuildMessage` and `SendDiff` for email diff notifications via sendmail subprocess, with Precedence: bulk headers and MAILHEADERS override support
- **cmd/rancid-cvs**: Initialize git repos and per-group directory structure under BASEDIR (replaces `bin/rancid-cvs`)
- **cmd/control-rancid**: Per-group orchestrator — reads router.db, runs parallel collection, commits to git, emails diffs (replaces `bin/control_rancid`)
- **cmd/rancid-run**: Cron entry point — iterates LIST_OF_GROUPS, invokes control-rancid per group with logging (replaces `bin/rancid-run`)
