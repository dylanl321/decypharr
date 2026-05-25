# Changelog

## [Unreleased] - API Enhancement Fork (dylanl321/decypharr)

### Added

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
