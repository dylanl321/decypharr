## Learned User Preferences

- Prefer a global `usenet.backend` config switch (`nntp` | `debrid`) for all NZBs rather than per-arr routing.
- Design NZB provider support behind an extensible interface; implement Torbox first.
- When implementing attached plans: do not edit the plan file; use existing todos (mark in_progress, complete all).
- Update CHANGELOG before committing significant feature work.
- Add a distinct download `phase` field for observability; keep qBittorrent-compatible `state` values for *arr compatibility.
- Prefer pre-flight cache checks across all configured debrid providers before submit, using the first cached match in config order.

## Learned Workspace Facts

- Decypharr is a Go app for torrent/Usenet downloads via debrid providers with *arr integration.
- Web UI is server-rendered Go templates and vanilla JS under `pkg/server/` (not a SPA); docs live in a separate Astro site under `docs/`.
- Common deployment uses dual-debrid (e.g., TorBox + RealDebrid) with multiple *arr instances.
- NZB ingestion currently requires NNTP; Torbox exposes a NZB/usenet API for provider-mediated downloads.
- `category_paths` maps Arr/category names to download paths with case-insensitive keys.
- `prefer_cached_provider` checks all configured debrid providers for cache before submit.
- qBittorrent API compatibility for *arr apps lives in `pkg/server/qbit/`.
