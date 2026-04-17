# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.4.2] - 2026-04-17

### Changed

- **cmd/clogin**: removed legacy login-script fallback (`clogin`/`jlogin`/`fnlogin` Tcl subprocess path); devices without a native ssh or telnet method now fail with a clear error instead of shelling out to the Perl login scripts — the last Expect/Tcl dependency is gone
- **pkg/devicetype**: removed `LoginScript` field from `DeviceSpec`; the `login` directive in `rancid.types.{base,conf}` is now silently ignored
- **pkg/connect**: removed unused `loginScript string` parameter from `NewSession` signature
- **pkg/connect**: removed deprecated `ErrNoNativeSSH` alias (use `ErrNoNativeTransport`)
- **release**: bump embedded `-V` / version strings to `0.4.2` for all binaries; Phase 4 marked complete in ROADMAP

## [0.4.1] - 2026-04-16

### Fixed

- **pkg/connect**: `readUntilPrompt` could hang forever on devices that accepted the SSH connection but never emitted a matching prompt (e.g., certain Cisco routers, load balancers with non-standard CLI, or SSL-VPN appliances). The underlying `ssh.Session` stdout does not honour `SetReadDeadline`, so the plain blocking `Read` bypassed both the provided timeout and the ctx cancellation. `Read` now runs on a short-lived goroutine gated by a `select` on ctx/timeout/result, so `ErrTimeout` is returned after the configured window and the stuck worker slot is released. Symptom on live runs: `control-rancid` stalled at ~90 devices with leaked SSH session goroutines; after the fix the same 261-device `observium` group completes to 243 successful collects with bounded error paths.

### Added

- **pkg/connect**: Legacy CBC cipher support (`aes128-cbc`, `aes192-cbc`, `aes256-cbc`, `3des-cbc`) alongside the modern GCM/CTR/ChaCha20 ciphers. Required for older Cisco IOS devices that only offer CBC ciphers in their SSH key exchange (observed symptom: `no common algorithm for client to server cipher`).

## [0.4.0] - 2026-04-16

### Added

- **pkg/parse/aeos**: Arista EOS parser with section-based processing for `show version`, `show boot-config`, `show env all`, `show inventory`, `show boot-extensions`, `show extensions`, `diff startup-config running-config`, and `show running-config`. Includes metadata extraction (model, serial, version), password/secret/SNMP community filtering, consecutive `!` collapsing, and EOS prompt pattern support (including `[HH:MM]` timestamp prefix)
- **pkg/connect**: SCP protocol support for downloading device configuration files over the existing SSH connection (`SCPDownload` method on `SSHSession`, `SCPDownloader` interface, `SCPConfigFile` field on `DeviceOpts`)
- **pkg/connect**: Legacy key exchange algorithm support (`diffie-hellman-group-exchange-sha1`, `diffie-hellman-group1-sha1`) for older network devices that don't support modern algorithms
- **pkg/connect**: Automatic `--More--` pager prompt handling in SSH `readUntilPrompt` — detects pager prompts and sends space to continue output
- **pkg/collect**: SCP-first collection with SSH fallback — when `SCPConfigFile` is set, the collector downloads the config via SCP first; if SCP fails (e.g., not enabled on device), it falls back to running config commands via SSH
- **pkg/parse/fortigate**: `#private-encryption-key=` and `FortinetPasswordMask` line filtering for SCP-downloaded configs; case-insensitive `System time` / `Cluster uptime` regex matching
- **pkg/devicetype/coverage**: `"aeos"` added to `moduleParsers` map for Arista EOS device type resolution

### Changed

- **pkg/parse/fortigate**: `DeviceOpts()` now sets `SCPConfigFile: "fgt-config"` to use SCP-based config download instead of `show full-configuration` over interactive SSH, and adds VDOM-aware setup commands (`config global` / `config system console` / `set output standard` / `end` / `end`)
- **pkg/collect**: `CollectDevice` dispatches to `collectSCPAndSSH` when `DeviceOpts.SCPConfigFile` is set, otherwise uses the standard `collectOutput` path
- **pkg/collect**: Command echo re-injection in `collectOutput` — `RunCommand` strips command echoes, but parsers need them for section boundary detection; the collector now prepends each command name before its output
- **cmd/rancid**: Uses `spec.Timeout` from device type configuration (e.g., `fortigate-full;timeout;90`) instead of hardcoded 30s

### Fixed

- Empty config output (0 bytes) caused by `RunCommand` stripping command echoes that parsers relied on for section detection
- FortiGate `fortiscp` devices producing no output because no `command` entries existed in `rancid.types.conf`
- FortiGate `show full-configuration` timing out at 30s — now uses SCP download with spec timeout support
- FortiGate `--More--` pager output appearing in collected configs despite setup commands

## [0.3.6] - 2026-04-15

### Added

- **ci**: GitLab pipeline now builds Debian packages for `amd64` and `arm64` with `nfpm`, publishing `.deb` artifacts that install the gorancid binaries under `/usr/local/rancid/bin`

### Changed

- **release**: bump embedded `-V` / version strings to `0.3.6` for all binaries; refresh README Phase 4 version marker

## [0.3.5] - 2026-04-15

### Changed

- **cmd/rancid-ui**: remove external Prism.js / cdnjs dependencies from the config and diff views, render with local inline styles only, and add a restrictive Content Security Policy for the read-only UI
- **release**: bump embedded `-V` / version strings to `0.3.5` for all binaries; refresh README Phase 4 version marker

## [0.3.4] - 2026-04-15

### Changed

- **release**: bump embedded `-V` / version strings to `0.3.4` for all binaries; refresh README and ROADMAP status for Phase 4

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
