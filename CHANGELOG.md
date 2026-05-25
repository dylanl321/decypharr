# Changelog

All notable changes from the **Decypharr reliability and dual-debrid roadmap** implementation.

## [Unreleased]

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

### Testing recommendations

1. Submit a magnet cached on Real-Debrid but not TorBox — confirm RD is selected without a long TorBox 0% stall.
2. Let TorBox complete with an initially empty `files` map — confirm HTTP pull starts and files land on disk.
3. Force a pull failure (bad path / permissions) — confirm entry becomes `error`, not stuck at `downloading` with `IsDownloading=true`.
4. Redeploy with a queue entry at `status=downloaded` — confirm startup resumes the local pull.
5. `curl http://localhost:8282/api/health` — confirm checks return within ~15s even when an Arr is down.

### Notes

- **Parallel HTTP downloads (#7):** Already supported via `max_downloads` and `sourcegraph/conc/pool` in `processTorrentDownload`; no new code—set `max_downloads` to 3–5 for multi-file season packs.
- **Per-entry `callback_url`:** Still stored on entries but not yet wired to the notification service (global `notifications.callback_url` only).
- **Protobuf:** Regenerate `pkg/storage/storage.pb.go` with `protoc` when available after pulling these changes; the repo includes a manual field patch for CI environments without `protoc`.
