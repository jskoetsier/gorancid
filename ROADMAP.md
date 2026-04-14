# Roadmap

## Phase 1: Orchestration Layer — COMPLETE (v0.1.0)

Drop-in Go replacement for `rancid-run`, `control_rancid`, and `rancid-cvs`. Per-device collection still delegates to the original Perl `rancid` script.

- [x] Config parsers (rancid.conf, router.db, .cloginrc)
- [x] Device type registry (rancid.types.base + rancid.types.conf)
- [x] Git subprocess wrapper
- [x] Parallel worker pool (PAR_COUNT)
- [x] FallbackCollector (→ Perl rancid)
- [x] Email diff notifications
- [x] CLI binaries (rancid-cvs, control-rancid, rancid-run)

## Phase 2: SSH Connector + Core Device Parsers — COMPLETE (v0.2.0)

Replace Expect for the 5 most common device types with native Go SSH sessions.

- [x] `pkg/connect` — Go SSH connector using `golang.org/x/crypto/ssh`
- [x] `pkg/connect` — Expect subprocess fallback for legacy devices
- [x] `pkg/parse` — `DeviceParser` interface
- [x] `pkg/parse/ios` — Cisco IOS parser
- [x] `pkg/parse/iosxr` — Cisco IOS-XR parser
- [x] `pkg/parse/junos` — Juniper JunOS parser
- [x] `pkg/parse/nxos` — Cisco NX-OS parser
- [x] `pkg/parse/fortigate` — Fortinet FortiGate parser
- [x] `cmd/rancid` — per-device collector binary (replaces Perl `rancid`)

## Phase 3: Remaining Device Parsers — COMPLETE (v0.3.0)

Complete Go parser coverage for all device types from `rancid.types.{base,conf}`.

- [x] Upstream alias compatibility for parser selection (`ios -> cisco`, `ios-nx -> cisco-nx`, etc.)
- [x] Dedicated Go parsers retained for core families (`ios`, `iosxr`, `junos`, `nxos`, `fortigate`)
- [x] Generic Go parser added for the remaining device-type long tail
- [x] `cmd/rancid` now uses Go parsing across the full `rancid.types` surface
- [x] Expect remains transport-only where native SSH collection is not yet appropriate

## Phase 4: Remove Expect Dependency

All device types have Go parser coverage. Remove Expect/Tcl transport dependency entirely.

Current status after `v0.3.1`: native interactive `clogin` has been hardened for the supported SSH families with real `.cloginrc`, `router.db`, auth, terminal, and prompt compatibility. Legacy login-script fallback still remains for the unsupported device long tail.

- [ ] Remove `FallbackCollector` and Expect subprocess path
- [ ] Remove legacy login-script transport dependency
- [ ] Standalone Go binary — no external runtime dependencies
- [ ] Web UI: config browser with syntax highlighting
- [ ] Web UI: diff viewer per device per run
- [ ] Web UI: device fleet status overview

## Phase 5: REST API

Add `cmd/rancid-api` — HTTP endpoint backed by the library.

- [ ] `POST /groups/{group}/collect` — trigger on-demand collection
- [ ] `GET /devices/{hostname}/config` — retrieve latest config
- [ ] `GET /devices/{hostname}/diff` — retrieve last diff
- [ ] `GET /groups/{group}/status` — collection status per device

## Phase 6: MCP Server

Add `cmd/rancid-mcp` — expose gorancid as MCP tools for AI agents.

- [ ] `list_devices` — list devices and status from router.db
- [ ] `get_config` — retrieve stored config from VCS
- [ ] `get_diff` — retrieve last diff for a device
- [ ] `connect_device` — open live session to a device via pkg/connect
- [ ] `run_command` — execute a command on a connected device
- [ ] `collect_device` — trigger on-demand collection
- [ ] `list_groups` — list configured rancid groups

## Phase 7: Structured Output

Populate `ParsedConfig.Metadata` fully and write JSON alongside plain-text configs.

- [ ] Version, model, serial, uptime extraction per device type
- [ ] Write `<hostname>.json` alongside `<hostname>` in VCS
- [ ] Plain-text output unchanged — VCS diffs stay readable

## Phase 8: Observability

- [ ] Prometheus metrics: collection duration, success/failure counts, last-seen timestamps
- [ ] Structured JSON logging (replace flat log files under LOGDIR)
- [ ] `cmd/rancid-status` — terminal dashboard for group collection health
