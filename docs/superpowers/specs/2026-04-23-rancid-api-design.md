# rancid-api Design

**Date:** 2026-04-23  
**Status:** Approved  
**Scope:** Add a JSON REST API to the existing `cmd/rancid-ui` binary

---

## Overview

Extend `rancid-ui` with a JSON API that exposes on-demand single-device collection and read access to configs, diffs, and group status. No new binary; no authentication (trusted network, same model as `rancid-ui`).

---

## Architecture

A new file `cmd/rancid-ui/api.go` contains an `apiServer` struct registered alongside the existing HTML routes. The two server types (`server` for HTML, `apiServer` for JSON) share `config.Config` but own their concerns independently.

```
cmd/rancid-ui/
  main.go       — wire both server and apiServer, register all routes
  server.go     — existing HTML handlers (rename from inline in main.go if needed)
  api.go        — new: apiServer struct + JSON handlers
```

All API routes are prefixed `/api/v1/`. Responses are `application/json` except `/config` and `/diff` which return `text/plain`.

```go
type apiServer struct {
    cfg        config.Config
    sysconfdir string   // path to rancid.types.base / rancid.types.conf
    cloginrc   string   // path to .cloginrc
    timeout    time.Duration // per-device collect timeout, default 60s
}
```

---

## Endpoints

### `POST /api/v1/groups/{group}/collect?device={hostname}`

Triggers synchronous single-device collection.

**Request:** `?device={hostname}` query param required; 400 if missing.  
**Validation:** group must be in `LIST_OF_GROUPS`; hostname validated against `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`.

**Flow:**
1. Load `router.db` for the group — 404 if device not found
2. Load device type specs from `sysconfdir`
3. Look up credentials from `.cloginrc`
4. Run `GoCollector.Run(ctx)` with configurable timeout (default 60s)
5. On success: `git.Add` for `configs/{hostname}`, then read staged diff via `git.Diff` (before commit)
6. `git.Commit` — no email notification
7. Include diff captured in step 5 in the response

**Response 200:**
```json
{"hostname": "ac2401", "status": "ok", "diff": "--- a/configs/ac2401\n+++ ..."}
```
```json
{"hostname": "ac2401", "status": "failed", "error": "connect: command timed out"}
```

Both success and failure return HTTP 200. The `status` field distinguishes outcome. `diff` is omitted when empty or on failure.

No locking against concurrent cron runs — last write wins before git commit, which is acceptable.

---

### `GET /api/v1/groups/{group}/devices/{hostname}/config`

Returns the latest collected config for a device.

**Response:** `text/plain`, content of `$BASEDIR/{group}/configs/{hostname}`.  
**404** if the file does not exist (device not yet collected).

---

### `GET /api/v1/groups/{group}/devices/{hostname}/diff`

Returns the unified diff introduced by the last git commit that touched this device's config.

**Response:** `text/plain`, output of `git log -1 -p --follow -- configs/{hostname}`.  
**200 with empty body** if no commit history exists yet.

---

### `GET /api/v1/groups/{group}/status`

Returns per-device status for a group.

**Response:**
```json
[
  {"hostname": "ac2401", "type": "cisco", "status": "up", "last_commit": "2026-04-23T13:10:00Z"},
  {"hostname": "ag241", "type": "junos", "status": "up", "last_commit": ""},
  ...
]
```

**Data sources:**
- `hostname`, `type`, `status`: from `router.db`
- `last_commit`: from `git log -1 --format=%cI -- configs/{hostname}` per device; empty string if no history

**Implementation note:** Runs one `git log` subprocess per device. With ~265 devices this is ~265 subprocesses on a cold read — acceptable for an on-demand endpoint.

Sorted alphabetically by hostname.

---

## New `pkg/git` function

```go
// LastCommitTime returns the timestamp of the most recent commit that touched path,
// or zero time if no such commit exists.
func LastCommitTime(dir, path string) (time.Time, error)
```

Uses `git log -1 --format=%cI -- {path}`, parses RFC3339 output.

---

## Changes to `main.go`

```go
// Additional flags
collectTimeout = flag.Duration("collect-timeout", 60*time.Second, "per-device collection timeout for API")
sysconfdir     = flag.String("sysconfdir", defaultSysconfdir(), "path to rancid.types.base/.conf")
cloginrc       = flag.String("cloginrc", defaultCloginrc(), "path to .cloginrc")

// Register API routes
api := &apiServer{cfg: cfg, sysconfdir: *sysconfdir, cloginrc: *cloginrc, timeout: *collectTimeout}
mux.Handle("POST /api/v1/groups/{group}/collect", http.HandlerFunc(api.handleCollect))
mux.Handle("GET /api/v1/groups/{group}/devices/{host}/config", http.HandlerFunc(api.handleDeviceConfig))
mux.Handle("GET /api/v1/groups/{group}/devices/{host}/diff", http.HandlerFunc(api.handleDeviceDiff))
mux.Handle("GET /api/v1/groups/{group}/status", http.HandlerFunc(api.handleGroupStatus))
```

`defaultSysconfdir()` returns `RANCID_SYSCONFDIR` env if set, else `/usr/local/rancid/etc`.  
`defaultCloginrc()` returns `$HOME/.cloginrc`.

---

## Error handling

All JSON error responses use:
```json
{"error": "human-readable message"}
```

| Condition | Status |
|-----------|--------|
| Unknown group | 404 |
| Invalid/missing device param | 400 |
| Device not in router.db | 404 |
| Config file not found | 404 |
| Internal error (git, file I/O) | 500 |
| Collection failed | 200 with `"status":"failed"` |

---

## Testing

- Unit tests in `cmd/rancid-ui/api_test.go` using `httptest.NewRecorder`
- Test `handleGroupStatus`: stub router.db + git repo, verify JSON shape
- Test `handleDeviceConfig`: verify 200/404 paths
- Test `handleDeviceDiff`: verify empty-history path returns 200 empty body
- Test `handleCollect`: 400 on missing `?device`, 404 on unknown group — full collect path is integration-only (requires live device)

---

## Out of scope

- Authentication / API keys
- Async collection (fire-and-forget + job polling)
- Group-level (all-devices) collect trigger
- Pagination on `/status`
