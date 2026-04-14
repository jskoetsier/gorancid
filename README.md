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
| `.cloginrc` | Login credentials (user, password, enable, method) |
| `rancid.types.base` / `rancid.types.conf` | Device type registry (scripts, modules, commands) |

CLI binaries match the original flag interface and exit codes, so existing cron jobs and wrapper scripts work without changes.

## Binaries

| Binary | Replaces | Purpose |
|--------|----------|---------|
| `clogin` | `clogin` / `plogin` | Interactive device login using `.cloginrc`, with device-type lookup from `router.db` |
| `rancid-cvs` | `rancid-cvs` | Initialize git repos and group directory structure |
| `control-rancid` | `control_rancid` | Per-group collection orchestrator |
| `rancid-run` | `rancid-run` | Cron entry point — iterates groups, calls control-rancid |

## Current Status

**Phase 2 complete (v0.2.0)** — native SSH collection and Go parsers ship for `ios`, `iosxr`, `junos`, `nxos`, and `fortigate`, with Expect/Perl fallback retained for unsupported types.

**Phase 3 in progress** — remaining device families are being ported in batches, starting with upstream alias compatibility so parser selection follows real `rancid.types` resolution.

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
├── collect/     # FallbackCollector (→ Perl rancid) + Collector interface
└── notify/      # Email diff notifications via sendmail
```

All logic lives in `pkg/`. CLI binaries in `cmd/` are thin wrappers that call library functions.

## License

GORANCID is licensed under the same terms as the upstream RANCID project.
