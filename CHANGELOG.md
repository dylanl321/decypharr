# Changelog

All notable changes from the **Decypharr reliability and dual-debrid roadmap**, **API enhancements**, and **web UI** work.

## [Unreleased]

### Added

#### UI redesign: app shell, Overview, and provider-first stats

- **App shell:** New collapsible left sidebar + compact topbar in `templates/layout.html` with provider status pills and a global theme toggle. Active-route highlighting and density-friendly typography land in `assets/styles.css`.
- **Overview page:** New `/overview` route, handler, template, and `assets/js/overview.js` showing KPIs (active downloads, throughput, library size, error rate), per-provider cards with trend sparklines, an active-downloads strip, and a live bandwidth-cap panel sourced from `/api/bandwidth`.
- **Queue redesign:** `templates/index.html` and `assets/js/dashboard.js` now expose provider filter chips, per-row provider color stripes, summary state pills, a cozy/compact density toggle, and a context-menu entry that opens a vertical history timeline drawer for any entry.
- **Stats redesign:** `templates/stats.html` reorders tabs to make Providers the default and adds a KPI grid + per-provider cards with ApexCharts sparklines fed by the existing `/api/stats` polling buffer.
- **Charting:** ApexCharts is now vendored locally via `download-assets.js` and conditionally loaded on Overview, Stats, and Queue pages. A shared `providerColor()` helper plus CSS palette tokens keep provider colors consistent across pages.

#### History timeline per queue entry

- **Model:** `storage.Entry` gains an in-memory `Timeline []TimelineEvent` log with kinds `added`, `queued`, `debrid_submitted`, `debrid_ready`, `local_download_start`, `local_download_done`, `symlinked`, `imported`, `error`, and `removed`. Events carry `provider`, `message`, `bytes`, and `duration` fields.
- **Persistence:** Timeline entries live in a new `timeline` sidecar bucket on the hybrid store (JSON-encoded, keyed by infohash) so the existing protobuf `Entry` record is unchanged. `queue.{Add,Update,Delete}` and `GetTorrent` transparently round-trip the timeline.
- **API/UI:** New `GET /api/queue/{hash}/timeline` endpoint plus a vertical timeline drawer in the Queue UI that auto-refreshes, falls back to a synthesized history for older entries, and supports "copy as text".

#### Local download bandwidth throttling

- **Config:** New global `download.bandwidth_limit` and per-debrid `bandwidth_limit` fields. A shared `config.ParseBandwidth` helper accepts `"10MB/s"`, `"1.5MiB/s"`, `"500KB/s"`, or raw bytes/sec.
- **Enforcement:** `pkg/manager/bandwidth.go` introduces a `bandwidthController` with global + per-provider `rate.Limiter`s. `localDownloader` swaps in a custom `http.RoundTripper` that wraps response bodies with `throttledBody`, so `grab` downloads honor the more restrictive of the two caps without breaking connection pooling.
- **Settings UI:** Config page now has a "Global Bandwidth Cap" input under download settings and a "Bandwidth Limit" input on each debrid card; values round-trip through `/api/config` and a dedicated `GET/PUT /api/bandwidth` endpoint.
- **Observability:** `BandwidthSnapshot` exposes effective caps and total bytes shaped, surfaced on the Overview bandwidth panel.

### Added

#### Multi-provider routing and slot limits

- **Per-debrid protocol toggles:** `allow_torrents` and `allow_nzbs` opt each debrid in or out of torrent and NZB submissions. Defaults preserve current behavior — `allow_torrents` is `true` for all providers, `allow_nzbs` is `true` for Torbox and `false` for the others.
- **Slot-aware provider selection:** `SendToDebrid` and `SendToNZBDebrid` now filter providers by `GetAvailableSlots()` against `minimum_free_slot` *before* submitting. When every eligible provider is exhausted, the request returns `too_many_active_downloads` and the queue retries.
- **Torbox active-slot accounting:** `GetAvailableSlots()` queries `/api/torrents/mylist` and `/api/usenet/mylist`, subtracting active+seeding entries from the plan max plus any `additional_concurrent_slots`. Honors a new optional per-debrid `max_active_downloads` cap.
- **Optional torrent debrid pin:** New global `torrent_debrid` field mirrors `usenet.debrid` for torrents — pin all torrent submissions to one provider name without reordering the debrids array.
- **Post-completion cleanup:** New global `cleanup_on_complete` block (`remove_from_provider`, `remove_from_queue`, `actions`, `delay`) and per-debrid `remove_on_complete` flag automatically delete completed entries from the debrid (stops Torbox seeding, frees slots) and/or from Decypharr's queue. Cleanup is gated to `actions: ["download"]` by default to keep symlink/strm mounts intact.
- **Config UI:** New per-debrid checkboxes (Allow torrents / Allow NZBs / Remove on complete) and `max_active_downloads` field; new global "Cleanup on complete" section and torrent debrid pin under Reliability & Queue.

### Changed

- **Torbox torrent permalinks:** `fetchDownloadLink` now sets `SkipValidation: true` to avoid burning API quota on HEAD validation against `/api/torrents/requestdl` redirects (matches the existing usenet permalink fix in `f9b8c22`).
- **Transient link failures:** HTTP 429 and other retryable validation errors are no longer cached as permanent failures, and `markAsError` keeps such entries in the `downloading` state so the queue can retry instead of requiring manual cleanup.

#### NZB debrid provider backend

- **`usenet.backend`:** Global switch between direct NNTP (`nntp`, default) and debrid-mediated NZB downloads (`debrid`).
- **`usenet.debrid`:** Names the debrid provider used when backend is `debrid` (Torbox supported initially).
- **Torbox usenet API:** Submit NZB files, poll status, fetch HTTP download links via `/api/usenet/*` endpoints.
- **Manager routing:** Debrid-backed NZBs reuse torrent HTTP download/stream/link paths; NNTP path unchanged.
- **Config UI:** Backend selector and Torbox debrid picker on the Usenet settings tab.

### Fixed

#### Stuck at 100% without HTTP pull (#1, #3)

- **Root cause:** When debrid reported completion, `processAction` set `IsDownloading=true` for the local HTTP pull. If the pull failed (commonly because `files` was still empty), the error was logged but `IsDownloading` was never cleared, so `processQueuedEntries` skipped the entry indefinitely.
- **Error handling:** `processAction` now calls `downloader.markAsError()` on failed post-debrid actions, moving the entry to `error` and clearing `IsDownloading`.
- **File backfill:** New `backfillEntryFromDebrid()` in `pkg/manager/entry_sync.go` populates `entry.Files` and provider placement metadata from the debrid API when a torrent completes—mirroring the logic in `processNewTorrent` / `processSyncTorrent`.
- **Completion flow:** `processQueuedTorrent` backfills files and runs `processAction` **inline** (not in a separate goroutine) while `processingEntries` is still held, avoiding duplicate polls and races.
- **TorBox edge case:** Treats debrid as complete when `Progress >= 1.0`, files are present, and status is not `error`, even if status is still `downloading`.

### Added

#### Pre-flight cache check across providers (#6)

- **`pkg/manager/debrid_select.go`:** Parallel `IsAvailable()` checks (5s timeout) across all configured debrids before magnet submit.
- **`orderedDebridClients()`:** Returns clients in `debrids[]` config order instead of undefined map iteration order.
- **`selectCachedProvider()`:** Picks the first cached provider in config order; skips AllDebrid (no cache API).
- **`SendToDebrid`:** When `prefer_cached_provider` is enabled (default), submits only to the cached provider if one is found; otherwise falls back to sequential try-all behavior.
- **Config:** `prefer_cached_provider` (`*bool`, default `true` when omitted).

#### Category-to-path mapping (#10)

- **Config:** `category_paths` map (case-insensitive keys), e.g. `"sonarr": "/downloads/tv"`.
- **`config.ResolveCategoryPath()`** in `internal/config/paths.go`: resolves save path from map or falls back to `download_folder/<arrName>`.
- **Wired into:** torrent import (`processor.go`), NZB import (`usenet.go`), qBittorrent category listing (`pkg/server/qbit/http.go`).

#### Automatic stale / stuck queue handling (#12)

- **`pkg/manager/stale.go`:** `processStaleQueueEntries()` runs on a 5-minute scheduler job.
- **Stuck at debrid-complete:** Entries with `status=downloaded` or `debrid_progress >= 1` still in `downloading` state for longer than `stuck_complete_minutes` (default **30**) reset `IsDownloading` and re-run `processAction`.
- **Stale with no progress:** Entries at ~0% progress for longer than `stale_download_hours` are marked `error` with a clear message.
- **Config:** `stale_download_hours`, `stuck_complete_minutes`.

#### Download lifecycle phases and progress (#2, #11)

- **Entry fields:** `phase`, `debrid_progress`, `local_progress` on `storage.Entry`.
- **Phase constants:** `queued`, `debrid_fetching`, `downloading`, `importing`, `complete` (qBittorrent `state` remains compatible: `downloading` / `pausedUP` / `error`).
- **Combined progress:** For `action=download`, overall `progress` = 50% debrid + 50% local during HTTP pull (`overallProgress()` in `downloader.go`).
- **qBit API:** Exposes `phase`, `debrid_progress`, `local_progress` on torrent info responses.
- **Persistence:** New protobuf fields on `EntryProto` and `ProviderEntryProto` (`storage.proto`, `storage.pb.go`, `proto.go`).

#### Provider placement stall tracking (#4)

- **`ProviderEntry`:** `last_progress_at`, `last_progress_value` updated via `touchProviderProgress()` when debrid progress advances—used for failover and stale detection.

#### Notifications (#5)

- **New event:** `debrid_ready` — fired when debrid finishes caching and the local HTTP pull is about to start.
- **Existing events:** `download_failed` now reliably fires when post-debrid action fails (via `markAsError`).

#### Health endpoint (#8)

- **`GET /api/health`** (public, no auth): Returns `healthy`, `degraded`, or `unhealthy` with structured checks:
  - **debrids:** Per-provider `GetProfile()` ping (5s timeout each).
  - **arrs:** `ValidateCtx()` with **10s** timeout (fixes hangs from unbounded Arr HTTP client on health checks).
  - **disk:** Writable test file in each `category_paths` entry and `download_folder`.
  - **queue:** Count and hashes of items stuck at debrid-complete for >30 minutes.
- **Docker healthcheck:** `cmd/healthcheck/main.go` also queries `/api/health`; treats `degraded` as healthy, `unhealthy` as failure.

#### Queue recovery after restart (#9)

- **`pkg/manager/startup_recovery.go`:** `recoverQueueOnStartup()` runs at worker start.
- Clears orphaned `IsDownloading=true` from crashes mid-pull.
- Resumes entries with `status=downloaded` that are not yet `is_complete` via `processAction`.

#### Provider failover on timeout (#4)

- **`pkg/manager/failover.go`:** `processProviderFailover()` scheduled every 15m when `failover_timeout_hours > 0`.
- If active provider has had no progress for the configured duration, probes other providers for cache and runs `Fixer.FixTorrent()` to cascade to an alternate debrid.

#### Debrid rate limits API (#13)

- **`GET /api/debrid/rate-limits`** (auth required): Returns configured `main`, `repair`, and `download` rate limit strings per debrid from config.

#### Web UI overhaul

- **Navigation:** Shared nav partial (`templates/partials/nav.html`) driven by `ui_layout.go`; **Health** and **Stats** in primary nav (including mobile); **Logs** consistently links to `/debug/logs`; **logout** at `/logout` with session user menu.
- **System Health page (`/health`):** Polls `GET /api/health` every 60s; shows debrid, *arr, disk, and stuck-queue checks with links to the dashboard for stuck hashes.
- **Dashboard:** Phase column; split debrid/local progress bars when applicable; **Queued** state filter (`phase=queued` in API); `?search=` URL pre-fill.
- **Settings — Reliability tab:** UI for `prefer_cached_provider`, `category_paths`, `stale_download_hours`, `stuck_complete_minutes`, `failover_timeout_hours`, `retries`, `categories`, `skip_auto_move`, `allow_samples`, `skip_multi_season`; save confirmation before restart.
- **Notifications:** `debrid_ready` event checkbox in Settings.
- **Download:** Arr category dropdown from `GET /api/arrs`; category path hint from config; fixed duplicate default on post-download action.
- **Stats:** Inline script extracted to `stats.js`; **Ingests** tab loads `GET /debug/ingests`.
- **Templates:** `partials/setup_alert.html`, `partials/config_reliability.html`; template `dict` helper; auth pages hide main nav.

### Changed

- **`processNewTorrent`:** Uses `backfillEntryFromDebrid` and synchronous `processAction` when already cached at submit time.
- **`pkg/arr/arr.go`:** `Validate()` delegates to `ValidateCtx()` with a 10s timeout; health and other callers can pass their own context.
- **`pkg/manager/workers.go`:** Registers stale-queue (5m) and failover (15m) jobs; calls `recoverQueueOnStartup()` on startup.

### New files

| File | Purpose |
|------|---------|
| `pkg/manager/entry_sync.go` | Debrid file/placement backfill and progress timestamps |
| `pkg/manager/debrid_select.go` | Ordered clients and parallel cache pre-check |
| `pkg/manager/stale.go` | Stuck / stale queue processor |
| `pkg/manager/startup_recovery.go` | Post-restart queue resume |
| `pkg/manager/failover.go` | Stall-based provider failover |
| `pkg/server/health.go` | `/api/health` handler |
| `pkg/server/debrid_status.go` | `/api/debrid/rate-limits` handler |
| `internal/config/paths.go` | `ResolveCategoryPath()` helper |

### Modified files

| File | Summary |
|------|---------|
| `pkg/manager/processor.go` | Backfill, inline `processAction`, cache-first submit, phases, `debrid_ready` notify |
| `pkg/manager/downloader.go` | Phase transitions, `debrid_progress` / `local_progress`, combined progress |
| `pkg/manager/workers.go` | Startup recovery, stale and failover jobs |
| `pkg/manager/usenet.go` | Category path resolution |
| `pkg/storage/types.go` | Phase constants, progress fields, provider stall fields |
| `pkg/storage/proto.go` | Proto conversion for new fields |
| `pkg/storage/storage.proto` | New entry and provider proto fields |
| `pkg/storage/storage.pb.go` | Generated struct fields (manual patch if `protoc` unavailable) |
| `internal/config/config.go` | New config keys and `PreferCached()` |
| `internal/config/notification.go` | `debrid_ready` event |
| `pkg/server/routes.go` | Health and rate-limit routes |
| `pkg/server/qbit/types.go` | Phase and progress in qBit JSON |
| `pkg/server/qbit/http.go` | Category paths in category API |
| `cmd/healthcheck/main.go` | `/api/health` integration |
| `pkg/server/ui.go`, `ui_layout.go`, `template_funcs.go` | Layout data, nav items, logout |
| `pkg/server/templates/layout.html`, `health.html`, `stats.html`, `config.html` | Nav shell, health page, stats/ingests, reliability tab |
| `pkg/server/assets/js/dashboard.js`, `config.js`, `download.js`, `health.js`, `stats.js` | Queue UX, reliability settings, download arrs, health/stats pages |
| `pkg/server/api.go` | Queued filter via `phase` |

### Configuration reference

```json
{
  "prefer_cached_provider": true,
  "category_paths": {
    "sonarr": "/downloads/tv",
    "radarr": "/downloads/movies"
  },
  "stale_download_hours": 24,
  "stuck_complete_minutes": 30,
  "failover_timeout_hours": 6,
  "max_downloads": 4,
  "notifications": {
    "enabled": true,
    "events": ["debrid_ready", "download_complete", "download_failed"]
  }
}
```

| Key | Default | Description |
|-----|---------|-------------|
| `prefer_cached_provider` | `true` | Parallel cache check before submit; use first cached debrid in config order |
| `category_paths` | _(none)_ | Override per-category download paths (case-insensitive) |
| `stale_download_hours` | `0` (off) | Mark 0%-progress downloads as error after N hours |
| `stuck_complete_minutes` | `30` | Retry/resume debrid-complete items stuck without local finish |
| `failover_timeout_hours` | `0` (off) | Switch provider via repair cascade after N hours without progress |
| `max_downloads` | `0` (unlimited) | Concurrent HTTP file pulls per torrent (existing; use 3–5 for NAS) |

### API reference

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/health` | No | Service health: debrids, arrs, disk, stuck queue |
| `GET` | `/api/debrid/rate-limits` | Yes | Configured rate limits per debrid |
| `GET` | `/health` | Yes | System health dashboard (HTML) |
| `GET` | `/logout` | Yes | End session and redirect to login |
| `GET` | `/debug/ingests` | Yes | Active debrid ingests (Stats UI) |

### Testing recommendations

1. Submit a magnet cached on Real-Debrid but not TorBox — confirm RD is selected without a long TorBox 0% stall.
2. Let TorBox complete with an initially empty `files` map — confirm HTTP pull starts and files land on disk.
3. Force a pull failure (bad path / permissions) — confirm entry becomes `error`, not stuck at `downloading` with `IsDownloading=true`.
4. Redeploy with a queue entry at `status=downloaded` — confirm startup resumes the local pull.
5. `curl http://localhost:8282/api/health` — confirm checks return within ~15s even when an Arr is down.
6. Open `/health` in the web UI — confirm status badges match the API and stuck items link to the dashboard.
7. Settings → Reliability — save `category_paths` and confirm Download shows the path hint for the selected Arr.

### Notes

- **Parallel HTTP downloads (#7):** Already supported via `max_downloads` and `sourcegraph/conc/pool` in `processTorrentDownload`; no new code—set `max_downloads` to 3–5 for multi-file season packs.
- **Per-entry `callback_url`:** Still stored on entries but not yet wired to the notification service (global `notifications.callback_url` only).
- **Protobuf:** Regenerate `pkg/storage/storage.pb.go` with `protoc` when available after pulling these changes; the repo includes a manual field patch for CI environments without `protoc`.

#### Case-insensitive Arr / category matching

- **`pkg/arr/arr.go`:** `Get()` falls back to `strings.EqualFold`; `GetOrCreate()` delegates to `Get()` before creating a manual entry.
- **`pkg/manager/queue.go`:** qBit-compat torrent list filters categories with `EqualFold` (e.g. Sonarr polls `sonarr`, entries stored as `Sonarr`).
- **`pkg/server/qbit/context.go`:** Config Arr lookup in `authenticate()` uses `EqualFold` for `download_uncached` inheritance.
- **`pkg/server/api.go`:** Dashboard torrent list category filter uses `EqualFold`.

### Added

#### REST API enhancements (merged from feature/api-enhancements)

#### Health & Monitoring
- `GET /api/health` — Component-level health checks with latency measurements
  - Checks: qbit_api, webdav, storage, mount, debrid_providers, arr_connections
  - Returns overall status: `healthy`, `degraded`, or `unhealthy`
  - Includes uptime and version info
- `GET /api/stats` — Runtime metrics (system, debrid, mount, queue, repair stats)
- `GET /api/version` — Version info under `/api/` prefix for consistency
- `GET /api/logs` — Recent log entries with level filtering (`?level=error&limit=50`)

#### Debrid Provider Management
- `GET /api/debrid/status` — Account details (expiry, premium status, username, points)
- `GET /api/debrid/providers` — List configured providers with types
- `POST /api/debrid/providers/{name}/test` — Test connectivity to specific provider

#### Queue Management
- `GET /api/queue/summary` — Lightweight queue counts by state/category/protocol
- `GET /api/queue/{hash}` — Get single queue item details
- `POST /api/queue/{hash}/retry` — Retry a failed item
- `POST /api/queue/{hash}/pause` — Pause a downloading item
- `POST /api/queue/{hash}/resume` — Resume a paused item
- `DELETE /api/queue/completed` — Clear all completed items
- `DELETE /api/queue/errors` — Clear all errored items
- `POST /api/queue/retry-all-errors` — Retry all items in error state

#### Arr Management (full CRUD)
- `POST /api/arrs` — Add a new arr connection
- `PUT /api/arrs/{name}` — Update an existing arr
- `DELETE /api/arrs/{name}` — Remove an arr
- `POST /api/arrs/{name}/test` — Test arr connectivity

#### Configuration & System
- `GET /api/notifications/config` — Get notification settings
- `PUT /api/notifications/config` — Update notification settings
- `GET /api/mount/status` — Mount status (type, ready state)
- `POST /api/mount/refresh` — Trigger mount refresh/rescan
- `POST /api/restart` — Trigger graceful restart

### New Files
- `pkg/server/api_health.go` — Health check endpoint implementation
- `pkg/server/api_status.go` — Debrid status and queue summary
- `pkg/server/api_queue.go` — Queue management endpoints
- `pkg/server/api_arrs.go` — Arr CRUD operations
- `pkg/server/api_system.go` — System/mount/notifications/logs/restart
- `pkg/server/logbuffer.go` — Thread-safe ring buffer for log viewing API

### Notes
- All new endpoints are protected by existing auth middleware
- Fully backward compatible — no existing endpoints modified
- Designed for headless/automation use (monitoring scripts, cron jobs)
- Public `GET /api/health` (Docker/reliability) vs authenticated `GET /api/health/components` (component latency checks)
