# Comprehensive REST API Documentation

This document describes the complete REST API for Decypharr, providing full programmatic access to all functionality.

## Base URL
All API endpoints are prefixed with `/api` (or your configured `URLBase` + `/api`).

## Authentication
All API endpoints (except `/version` and setup routes) require authentication via:
- **Session Cookie**: After logging in via `/login`
- **API Token**: Pass as `X-API-Key` header (get token from `/api/config` or `/api/refresh-token`)

---

## 1. Queue Management APIs

### Get Single Queue Item
**GET** `/api/queue/{hash}`

Get detailed information about a single queue item by info hash.

**Response:**
```json
{
  "info_hash": "abc123...",
  "name": "Example Torrent",
  "state": "downloading",
  "progress": 45.2,
  "size": 1234567890,
  "category": "sonarr",
  ...
}
```

### Retry/Requeue Failed Item
**POST** `/api/queue/{hash}/retry`

Retry a failed queue item by recreating the import request.

**Response:**
```json
{
  "status": "queued"
}
```

### Pause Downloading Item
**POST** `/api/queue/{hash}/pause`

Pause an actively downloading item.

**Response:**
```json
{
  "status": "paused"
}
```

### Resume Paused Item
**POST** `/api/queue/{hash}/resume`

Resume a paused item.

**Response:**
```json
{
  "status": "downloading"
}
```

### Delete All Completed Items
**DELETE** `/api/queue/completed`

Remove all completed items from the queue.

**Response:** `200 OK`

### Delete All Error Items
**DELETE** `/api/queue/errors`

Remove all items in error state from the queue.

**Response:** `200 OK`

### Retry All Error Items
**POST** `/api/queue/retry-all-errors`

Retry all items currently in error state.

**Response:**
```json
{
  "retried": 5,
  "failed": 2,
  "total": 7
}
```

---

## 2. Arr Management APIs (Full CRUD)

### List All Arrs
**GET** `/api/arrs`

Get all configured arr connections.

**Response:**
```json
[
  {
    "name": "sonarr",
    "host": "http://localhost:8989",
    "type": "sonarr",
    "cleanup": true,
    "skip_repair": false,
    ...
  }
]
```

### Add New Arr
**POST** `/api/arrs`

Add a new arr connection.

**Request Body:**
```json
{
  "name": "radarr",
  "host": "http://localhost:7878",
  "token": "your-api-key",
  "cleanup": true,
  "skip_repair": false,
  "download_uncached": false,
  "selected_debrid": "realdebrid"
}
```

**Response:** `201 Created` with arr object

### Update Existing Arr
**PUT** `/api/arrs/{name}`

Update an existing arr connection.

**Request Body:** Same as Add (partial updates supported)

**Response:** `200 OK` with updated arr object

### Delete Arr
**DELETE** `/api/arrs/{name}`

Remove an arr connection.

**Response:** `200 OK`

### Test Arr Connection
**POST** `/api/arrs/{name}/test`

Test connectivity to an arr.

**Response:**
```json
{
  "success": true,
  "message": "Connection successful"
}
```
Or:
```json
{
  "success": false,
  "error": "connection refused"
}
```

---

## 3. Debrid Provider Management

### List Debrid Providers
**GET** `/api/debrid/providers`

Get all configured debrid providers.

**Response:**
```json
[
  {
    "name": "realdebrid",
    "type": "realdebrid",
    "enabled": true
  },
  {
    "name": "alldebrid",
    "type": "alldebrid",
    "enabled": true
  }
]
```

### Test Debrid Provider
**POST** `/api/debrid/providers/{name}/test`

Test connectivity to a debrid provider.

**Response:**
```json
{
  "success": true,
  "message": "Connection successful",
  "profile": {
    "username": "user@example.com",
    "expiration": "2024-12-31T23:59:59Z",
    ...
  }
}
```

---

## 4. Notification Settings

### Get Notification Config
**GET** `/api/notifications/config`

Get current notification configuration.

**Response:**
```json
{
  "enabled": true,
  "webhook_url": "https://discord.com/api/webhooks/...",
  "callback_url": "http://localhost:8080/callback",
  "events": ["download_complete", "download_failed"]
}
```

### Update Notification Config
**PUT** `/api/notifications/config`

Update notification settings.

**Request Body:**
```json
{
  "enabled": true,
  "webhook_url": "https://discord.com/api/webhooks/...",
  "callback_url": "http://localhost:8080/callback",
  "events": ["download_complete", "repair_complete"]
}
```

**Response:** `200 OK` with updated config

---

## 5. Mount Status/Info

### Get Mount Status
**GET** `/api/mount/status`

Get mount manager status and statistics.

**Response:**
```json
{
  "enabled": true,
  "ready": true,
  "type": "rclone",
  "stats": {
    "core": { ... },
    "memory": { ... },
    "bandwidth": { ... }
  }
}
```

### Refresh Mount
**POST** `/api/mount/refresh`

Trigger a mount refresh/rescan.

**Query Parameters:**
- `dirs` (optional): Comma-separated list of directories to refresh (default: `__all__`)

**Response:**
```json
{
  "status": "refreshed"
}
```

---

## 6. Log Viewing

### Get Recent Logs
**GET** `/api/logs`

Retrieve recent log entries from the in-memory buffer.

**Query Parameters:**
- `limit` (optional): Number of log entries to return (default: 100, max: 1000)
- `level` (optional): Filter by log level (`error`, `warn`, `info`, `debug`)

**Response:**
```json
{
  "logs": [
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "level": "info",
      "message": "Started processing torrent abc123",
      "fields": {}
    }
  ],
  "count": 100,
  "limit": 100
}
```

---

## 7. System/Service Control

### Restart Service
**POST** `/api/restart`

Trigger a graceful service restart.

**Response:**
```json
{
  "status": "restarting"
}
```

### Get Version Info
**GET** `/api/version`

Get application version information (same as `/version` but under `/api` namespace).

**Response:**
```json
{
  "version": "v1.0.0",
  "commit": "abc123",
  "build_date": "2024-01-15",
  "go_version": "go1.21"
}
```

---

## Existing APIs (Already Available)

### Health Check
**GET** `/api/health`

System health status.

### Statistics
**GET** `/api/stats`

Get system statistics.

### Debrid Status
**GET** `/api/debrid/status`

Get debrid service status.

### Queue Summary
**GET** `/api/queue/summary`

Get queue summary statistics.

### List Torrents (Paginated)
**GET** `/api/torrents`

List all torrents with filtering, sorting, and pagination.

**Query Parameters:**
- `page`: Page number (default: 1)
- `limit`: Items per page (default: 20, max: 100)
- `search`: Search term
- `category`: Filter by category
- `state`: Filter by state
- `sort_by`: Sort field (`name`, `size`, `added_on`, `progress`, `category`, `state`)
- `sort_order`: `asc` or `desc`

### Delete Torrent
**DELETE** `/api/torrents/{category}/{hash}`

Delete a single torrent.

**Query Parameters:**
- `removeFromDebrid`: Set to `true` to also remove from debrid service

### Delete Multiple Torrents
**DELETE** `/api/torrents?hashes={hash1,hash2,...}`

Delete multiple torrents by comma-separated hashes.

### Add Content
**POST** `/api/add`

Add new torrents or NZBs (multipart form data).

### Repair APIs
Complete repair/health-check operations under `/api/repair/*`.

### Browse APIs
WebDAV-style file browser under `/api/browse/*`.

### Config APIs
**GET** `/api/config` - Get full configuration
**POST** `/api/config` - Update configuration
**POST** `/api/refresh-token` - Refresh API token
**POST** `/api/update-auth` - Update authentication settings

---

## Error Responses

All endpoints return standard HTTP status codes:
- `200 OK`: Success
- `201 Created`: Resource created
- `400 Bad Request`: Invalid input
- `404 Not Found`: Resource not found
- `409 Conflict`: Resource already exists
- `500 Internal Server Error`: Server error

Error responses include a message:
```json
{
  "error": "Error message here"
}
```
Or plain text error message.

---

## Implementation Notes

### Log Buffer
- Stores the last 1000 log entries in memory
- Thread-safe ring buffer implementation
- Integrated with zerolog for automatic capture
- Does not persist across restarts

### Queue Operations
- Retry operations reconstruct import requests from stored entry data
- Pause/resume updates entry state in the database
- Bulk operations operate on filtered entry sets

### Arr Management
- Changes are persisted to config.json
- Validation happens before save
- Test endpoint validates connectivity before adding

### Mount Refresh
- Delegates to the configured mount manager (rclone/DFS/external)
- Directories parameter allows selective refresh

### Debrid Provider Test
- Calls `GetProfile()` to verify connectivity
- Returns profile information on success

---

## Usage Examples

### cURL Examples

**Add a new arr:**
```bash
curl -X POST http://localhost:8282/api/arrs \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-token" \
  -d '{
    "name": "radarr",
    "host": "http://localhost:7878",
    "token": "your-radarr-api-key",
    "cleanup": true
  }'
```

**Retry all failed items:**
```bash
curl -X POST http://localhost:8282/api/queue/retry-all-errors \
  -H "X-API-Key: your-api-token"
```

**Get recent error logs:**
```bash
curl "http://localhost:8282/api/logs?limit=50&level=error" \
  -H "X-API-Key: your-api-token"
```

**Test debrid connection:**
```bash
curl -X POST http://localhost:8282/api/debrid/providers/realdebrid/test \
  -H "X-API-Key: your-api-token"
```

---

## Migration from Browser UI

All functionality previously only available via the browser UI is now accessible via API:
- ✅ Full arr CRUD operations
- ✅ Queue item retry/pause/resume
- ✅ Bulk queue operations (delete completed/errors, retry all)
- ✅ Notification configuration
- ✅ Mount management
- ✅ Log viewing
- ✅ Service restart
- ✅ Debrid provider testing

This enables full automation and integration with external tools, scripts, and services.
