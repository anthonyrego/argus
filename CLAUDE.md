# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

argus is a self-hosted NVR for Amcrest / Dahua IP cameras. A single Go binary watches each camera's event stream, records motion clips, serves a live HLS / MJPEG view, exposes a REST + SSE API, embeds a React SPA, and pushes APNs notifications to a companion iOS app. SQLite (pure Go, no CGO) holds cameras, events, recordings, devices, and admin credentials.

## Commands

```bash
./scripts/dev.sh                # Go backend (:8080, -debug) + Vite dev server (:5173, /api proxied to :8080)
go run . -config config.yaml    # backend only
go build -o argus .             # production binary (will embed web/dist if previously built)

cd web && npm install           # one-time
cd web && npm run dev           # frontend only (Vite, proxies /api → :8080)
cd web && npm run build         # emits to internal/server/dist/ — required before `go build` for an embedded SPA
cd web && tsc -b                # type-check only (also run as part of `npm run build`)

go vet ./...                    # there are currently no Go tests in the repo
```

The iOS app under `ios/` is a separate Xcode project, not built by Go. See `ios/README.md` for the one-time Xcode setup; there are no command-line build steps from this repo.

## Architecture

`main.go` wires the components in this order, and the dependency graph matters:

```
store (SQLite)
  └── eventmgr ── AddSink ──▶ recorder ── OnClipOpened ─▶ apns.Pusher
        │                       │
        │                       └── OnRecordingInserted ─▶ eventmgr.BroadcastRecording (SSE)
        │
        └── Subscribe / SubscribeRecordings ──▶ server (SSE /api/events/stream)

streamer (on-demand HLS) ──▶ server
server (chi) ── embeds ──▶ web/dist (go:embed internal/server/frontend.go)
```

Key design facts that are hard to recover by reading any single file:

- **Cameras live in SQLite, not config.yaml.** `config.yaml` only has server addr, DB path, APNs creds, and recorder timing. Cameras are CRUD'd via the web UI; `eventmgr.Sync()` is called after every camera write to reconcile running listeners against the desired set (and is also triggered on connection-param changes, not just create/delete).
- **Event source is Dahua's `eventManager.cgi?action=attach`**, a long-lived multipart HTTP stream with digest auth (`internal/events/dahua.go`). One listener per enabled camera, reconnects with exponential backoff. We deliberately do not use ONVIF — see `memory/project_event_source.md`.
- **Two fan-out paths from eventmgr.** Synchronous `EventSink`s (the recorder) cannot drop events. Buffered `Subscribe` channels (SSE clients) drop on slow consumers — keep them best-effort. Recordings have their own subscribe channel because they're emitted by the recorder *after* the clip is finalized, not at event time.
- **Recorder uses ffmpeg segments + concat demuxer**, not direct RTSP capture. One ffmpeg per camera continuously remuxes the main RTSP stream into ~1s MPEG-TS segments under `<recordings.dir>/.work/<camera>/`. A motion Start marks a trigger; segments covering `[trigger - pre_roll, trigger + post_roll]` are concatenated into a final MP4. Repeated Starts extend the post-roll, bounded by `max_clip_sec` from the original trigger. `OnClipOpened` fires once per clip (on the opening Start), so push notifications don't spam during extensions.
- **Streamer is on-demand HLS.** The first hit to `/api/cameras/{id}/hls/index.m3u8` spawns ffmpeg; sessions are reaped after 30s of inactivity. Sessions are also recreated when camera host/creds change (tracked via `camHash`).
- **Camera MJPEG path is sub-stream only.** Dahua's MJPEG CGI doesn't work on the main stream, so the main stream is reached via HLS instead. See `memory/project_camera_mjpeg_limits.md`. The `proxy` package handles digest auth for snapshot.jpg / stream.mjpg (sub-stream).
- **Frontend embedding via `go:embed all:dist`** (`internal/server/frontend.go`). The `dist/` directory is committed with a `.gitkeep` so the package builds even when the frontend hasn't been built; `Frontend()` returns `nil` in that case and the server replies with a "frontend not built" message instead of serving the SPA. `npm run build` writes into `../internal/server/dist/` (configured in `web/vite.config.ts`), not `web/dist/`.
- **Two auth paths share one bearer-token mechanism.**
  - `POST /api/login` — admin username/password, returns a device row + token for the browser. A bootstrap admin (`admin` / `admin`, `must_change_password=1`) is seeded by `store.EnsureAdminSeeded` on first start.
  - `POST /api/pair/complete` — an authenticated browser calls `/api/pair/start` to mint a 6-digit code; the phone POSTs the code to `/api/pair/complete` to claim a per-device API token. Both endpoints are rate-limited (`attemptLimiter`).
  - All `/api/*` endpoints other than `login` and `pair/complete` require a bearer token (`Authorization: Bearer …` *or* `?token=…` query for `<img>`/`<video>` tags that can't set headers). Tokens map 1:1 to rows in `devices`.
- **APNs is conditional.** `apns.Config.PushEnabled()` is false if any of team_id / key_id / key_path / bundle_id is empty; the server starts without a pusher in that case. When enabled, `OnClipOpened` triggers one push per clip with the snapshot as an attachment (handled by the iOS Notification Service Extension).
- **SQLite driver is `modernc.org/sqlite`** — pure Go, no CGO toolchain required. PRAGMAs (`journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=1`) are set in `store.Open`.

## Conventions worth knowing

- `internal/store/store.go` owns *all* SQL. Other packages take typed `store.Camera` / `store.Event` / `store.Recording` / `store.Device` values; do not reach into `*sql.DB` from elsewhere.
- The `Camera.Password` field has `json:"-"` and is never returned; an empty `password` field in an update request means "keep existing" (`store.UpdateCamera`).
- All timestamps in SQLite are RFC3339Nano UTC strings; the `time.Time` parsing in `scan*` helpers is the only place this format is handled.
- `slog` is the logger; the binary takes `-debug` for `LevelDebug`.

## Memory notes

The user's auto-memory under `~/.claude/projects/-Users-rego-projects-argus/memory/` contains additional context (iOS design decisions, Xcode scaffolding gotchas, Tailscale setup, the parked vLLM/Gemma inference plan). Consult `MEMORY.md` there when working on iOS or networking concerns.
