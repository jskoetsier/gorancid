# gorancid

Go rewrite of [RANCID](https://www.shrubbery.net/rancid/) (Really Awesome New Cisco confIg Differ) — a drop-in replacement that collects network device configurations, stores diffs in git, and emails changes.

## Why

RANCID is written in Perl, Tcl/Expect, and C. It works, but the stack is brittle and hard to extend. Gorancid replaces the orchestration layer in Go while keeping full compatibility with existing configs, credentials, and device type definitions.

## Compatibility

Gorancid reads the same configuration files as upstream RANCID:

| File | Purpose |
|------|---------|
| `rancid.conf` | Base directory, groups, mail settings, filter options |
| `router.db` | Device inventory (`hostname:type:status`) |
| `.cloginrc` | Login credentials (user, password, enable, **method** — see below) |
| `rancid.types.base` / `rancid.types.conf` | Device type registry (scripts, modules, commands) |

CLI binaries match the original flag interface and exit codes, so existing cron jobs and wrapper scripts work without changes.

### Native transport (`.cloginrc` `method`)

For **collection** (`rancid`, `control-rancid`) and **native** `clogin`, gorancid opens the first supported entry in declaration order:

- `ssh` or `ssh:PORT` — Go SSH client (default when `method` is omitted).
- `telnet` or `telnet:PORT` — Go Telnet client with username/password prompt handling (best-effort; not all vendor banners are covered).

If neither appears (for example only `rsh`), collection fails with a clear error. Interactive `clogin` can still fall back to legacy Perl/Tcl login scripts when the device type has no native `DeviceOpts` profile or negotiation fails.

## Binaries

| Binary | Replaces | Purpose |
|--------|----------|---------|
| `clogin` | `clogin` / `plogin` | Interactive device login using `.cloginrc`, with device-type lookup from `router.db` |
| `rancid` | `rancid` | Per-device collector using Go parsers for all known device types |
| `rancid-cvs` | `rancid-cvs` | Initialize git repos and group directory structure |
| `control-rancid` | `control_rancid` | Per-group collection orchestrator |
| `rancid-run` | `rancid-run` | Cron entry point — iterates groups, calls control-rancid |
| `rancid-ui` | _(new)_ | Read-only local web UI — fleet table, config browser, last per-device git diff |

## Current Status

**Phase 3 complete (v0.3.0)** — all device types in `rancid.types.{base,conf}` now have Go parser coverage, using dedicated parsers for the core families and a generic Go parser for the remaining long tail.

**Phase 4 in progress (v0.3.6)** — in-process collection over **SSH or Telnet** (no Expect for transport). `cmd/rancid-ui` provides a local fleet/config/diff browser. Interactive `clogin` still falls back to legacy login scripts when native transport is unavailable (for example unknown device types without `DeviceOpts`).

## Building

```bash
go build ./...
```

Individual binaries:

```bash
go build -o rancid-cvs ./cmd/rancid-cvs/
go build -o control-rancid ./cmd/control-rancid/
go build -o rancid-run ./cmd/rancid-run/
go build -o clogin ./cmd/clogin/
go build -o rancid ./cmd/rancid/
go build -o rancid-ui ./cmd/rancid-ui/
```

Run the UI (defaults to loopback `127.0.0.1:8080`):

```bash
./rancid-ui -C /path/to/rancid.conf
```

Cross-compile for Linux:

```bash
GOOS=linux GOARCH=amd64 go build -o rancid-cvs ./cmd/rancid-cvs/
```

## Testing

```bash
go test ./...
```

## Installation

1. Build the binaries (or download from CI artifacts)
2. Copy to `/usr/local/rancid/bin/` alongside the existing Perl scripts
3. Point cron at the Go `rancid-run` binary instead of the original
4. Existing `rancid.conf`, `router.db`, `.cloginrc`, and `rancid.types.*` files are read as-is

## Architecture

```
pkg/
├── config/      # rancid.conf, router.db, .cloginrc parsers
├── devicetype/  # rancid.types.{base,conf} registry with alias resolution
├── git/         # Git subprocess wrapper (Init, Add, Commit, Diff)
├── par/         # Parallel worker pool bounded by PAR_COUNT
├── collect/     # GoCollector / CollectDevice (SSH or Telnet + Go parsers)
└── notify/      # Email diff notifications via sendmail
```

All logic lives in `pkg/`. CLI binaries in `cmd/` are thin wrappers that call library functions.

## License

GORANCID is licensed under the same terms as the upstream RANCID project.
