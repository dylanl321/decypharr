## Learned User Preferences

- Prefer a global `usenet.backend` config switch (`nntp` | `debrid`) for all NZBs rather than per-arr routing.
- Design NZB provider support behind an extensible interface; implement Torbox first.
- When implementing attached plans: do not edit the plan file; use existing todos (mark in_progress, complete all).
- Update CHANGELOG before committing significant feature work.
- Add a distinct download `phase` field for observability; keep qBittorrent-compatible `state` values for *arr compatibility.
- Prefer pre-flight cache checks across all configured debrid providers before submit, using the first cached match in config order.
- New config fields must be configurable through all three channels: `/api/config` JSON, `DECYPHARR_*` env vars, and the Web UI.
- Treat 429 / rate-limit / `too_many_active_downloads` as retryable; do not mark them as permanent queue errors.
- Default post-completion cleanup to the `download` action only; never auto-remove `symlink` / `strm` entries because mounts depend on them.

## Learned Workspace Facts

- Decypharr is a Go app for torrent/Usenet downloads via debrid providers with *arr integration.
- Web UI is server-rendered Go templates and vanilla JS under `pkg/server/` (not a SPA); docs live in a separate Astro site under `docs/`.
- Common deployment uses dual-debrid (e.g., TorBox + RealDebrid) with multiple *arr instances.
- NZB ingestion supports two backends via `usenet.backend`: NNTP, or `debrid` routed through `usenet.debrid` (TorBox is the only NZB-capable debrid today).
- `category_paths` maps Arr/category names to download paths with case-insensitive keys.
- `prefer_cached_provider` checks all configured debrid providers for cache before submit.
- qBittorrent API compatibility for *arr apps lives in `pkg/server/qbit/`; it emulates the qBit client and is not where user-facing settings are exposed.
- Per-debrid capability flags (`allow_torrents`, `allow_nzbs`, `max_active_downloads`, `minimum_free_slot`, `remove_on_complete`) drive protocol-aware routing and slot-aware filtering; `updateDebrid()` fills sensible defaults from nil pointers (e.g., `allow_nzbs` true only for NZB-capable providers).
- TorBox `GetAvailableSlots()` subtracts active torrent + usenet usage (from `/api/torrents/mylist` + `/api/usenet/mylist`) from plan max plus `additional_concurrent_slots`.
- TorBox `requestdl` permalinks (torrent and usenet) set `DownloadLink.SkipValidation = true` to avoid HEAD-request 429s against the API quota.
- On Windows PowerShell, heredoc-style `git commit -m` fails; write the message to a temp file and use `git commit -F <file>`.
