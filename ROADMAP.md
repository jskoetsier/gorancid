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

## Phase 3: Remaining Device Parsers — IN PROGRESS

Port the remaining ~30 device parsers from Perl to Go, in batches.

Started with upstream alias compatibility for the Cisco family so native parsing follows real `rancid.types` resolution (`ios -> cisco`) instead of repo-only testdata.

- [ ] Batch 1: aos, cat5k, csm, escape, firew
- [ ] Batch 2: foundry, hitachi, hp5, mrtd
- [ ] Batch 3: netscaler, netscreen, procurve, riverstone
- [ ] Batch 4: remaining types (all others)
- [ ] Expect fallback shrinks to zero as each batch ships

## Phase 4: Remove Expect Dependency

All device types have Go-native collectors. Remove Expect/Tcl dependency entirely.

- [ ] Remove `FallbackCollector` and Expect subprocess path
- [ ] Remove Perl `rancid` binary dependency
- [ ] Standalone Go binary — no external runtime dependencies

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

## Phase 9: Web UI

Single-page app backed by the Phase 5 API.

- [ ] Config browser with syntax highlighting
- [ ] Diff viewer per device per run
- [ ] Device fleet status overview
