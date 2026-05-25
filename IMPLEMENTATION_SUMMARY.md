# API Enhancement Implementation Summary

## Overview
This implementation adds comprehensive REST API endpoints to Decypharr, eliminating the need to use the browser UI for any functionality. All operations are now accessible programmatically.

## Files Created

### 1. `pkg/server/logbuffer.go`
**Purpose:** Thread-safe ring buffer for capturing recent log entries

**Key Features:**
- Stores last 1000 log entries in memory
- Implements `zerolog.LevelWriter` for integration with the logging system
- Provides `GetRecent()` and `FilterByLevel()` methods
- Thread-safe using `sync.RWMutex`

**Structure:**
```go
type LogEntry struct {
    Timestamp time.Time
    Level     string
    Message   string
    Fields    map[string]interface{}
}

type LogBuffer struct {
    mu      sync.RWMutex
    entries []LogEntry
    pos     int
    size    int
    wrapped bool
}
```

### 2. `pkg/server/api_queue.go`
**Purpose:** Queue management endpoints (CRUD + Actions)

**Endpoints Implemented:**
- `GET /api/queue/{hash}` - Get single queue item details
- `POST /api/queue/{hash}/retry` - Retry/requeue a failed item
- `POST /api/queue/{hash}/pause` - Pause downloading item
- `POST /api/queue/{hash}/resume` - Resume paused item
- `DELETE /api/queue/completed` - Remove all completed items
- `DELETE /api/queue/errors` - Remove all error items
- `POST /api/queue/retry-all-errors` - Retry all failed items

**Key Methods:**
- Uses `s.manager.Queue().GetTorrent()` to fetch individual items
- Uses `s.manager.Queue().ReQueue()` to retry items
- Uses `s.manager.Queue().UpdateWhere()` for pause/resume state changes
- Uses `s.manager.Queue().DeleteWhere()` for bulk deletions
- Uses `s.manager.Queue().ListFilter()` to enumerate error items

### 3. `pkg/server/api_arrs.go`
**Purpose:** Full CRUD operations for arr connections

**Endpoints Implemented:**
- `POST /api/arrs` - Add new arr connection
- `PUT /api/arrs/{name}` - Update existing arr
- `DELETE /api/arrs/{name}` - Remove arr connection
- `POST /api/arrs/{name}/test` - Test arr connectivity

**Key Methods:**
- Uses `s.manager.Arr().AddOrUpdate()` to add/update arrs
- Uses `arr.Validate()` to test connectivity
- Uses `s.manager.Arr().SyncToConfig()` to persist changes
- Validates required fields (name, host, token) before saving

### 4. `pkg/server/api_system.go`
**Purpose:** System-level APIs (mount, notifications, logs, restart, debrid)

**Endpoints Implemented:**
- `GET /api/mount/status` - Get mount status and stats
- `POST /api/mount/refresh` - Trigger mount refresh
- `GET /api/notifications/config` - Get notification config
- `PUT /api/notifications/config` - Update notification config
- `GET /api/logs` - Get recent log entries (with filtering)
- `POST /api/restart` - Graceful service restart
- `GET /api/version` - Version info (consistent with `/version`)
- `GET /api/debrid/providers` - List debrid providers
- `POST /api/debrid/providers/{name}/test` - Test debrid connectivity

**Key Methods:**
- Uses `s.manager.MountManager()` to access mount operations
- Uses `config.Get().Notifications` for notification settings
- Uses `s.logBuffer` for log retrieval
- Uses `s.Restart()` for service restart
- Uses `s.manager.ProviderClient()` to test debrid connections

## Files Modified

### 1. `pkg/server/server.go`
**Changes:**
- Added `logBuffer *LogBuffer` field to Server struct
- Initialize log buffer in `New()` function with capacity of 1000 entries

### 2. `pkg/server/routes.go`
**Changes:**
- Added all new API routes under the `/api` route group
- Organized routes by category (Queue, Arr, Debrid, Notifications, Mount, Logs, System)
- Added consistent RESTful URL patterns

## Implementation Details

### Queue Retry Logic
When retrying a failed item:
1. Fetch the entry from queue storage using `GetTorrent(hash)`
2. Reconstruct the `manager.ImportRequest` from stored entry data
3. Lookup the arr configuration by category name
4. Rebuild the magnet link from stored info hash and name
5. Call `Queue.ReQueue()` to re-add to processing queue

### Pause/Resume State Management
- Pause: Changes state from `EntryStateDownloading` to `EntryStatePausedDL`
- Resume: Changes state from `EntryStatePausedDL` or `EntryStatePausedUP` back to `EntryStateDownloading`
- Uses `Queue.UpdateWhere()` with predicate and update functions

### Arr Management Persistence
1. All arr CRUD operations update the in-memory arr storage
2. Changes are synced to config using `SyncToConfig()`
3. Config is persisted to disk with `cfg.Save()`
4. Validation happens before any persistence

### Log Buffer Integration
- Log buffer is initialized at server startup
- Captures logs as they're written via zerolog integration
- Stores entries in a circular buffer (oldest entries are overwritten)
- Provides filtering by level and limit on retrieval

### Mount Refresh
- Delegates to the configured mount manager implementation
- Supports selective directory refresh via query parameter
- Defaults to refreshing `__all__` if no directories specified

## API Design Patterns

### Consistent Response Format
- Success: JSON response with `200 OK` or `201 Created`
- Error: HTTP error status with plain text or JSON error object
- All responses use `utils.JSONResponse()` for consistency

### RESTful URL Structure
- Resources: `/api/{resource}` (e.g., `/api/arrs`, `/api/queue`)
- Single item: `/api/{resource}/{id}` (e.g., `/api/queue/{hash}`)
- Actions: `/api/{resource}/{id}/{action}` (e.g., `/api/queue/{hash}/retry`)
- Nested resources: `/api/{resource}/{id}/{subresource}`

### HTTP Methods
- `GET` - Retrieve resources
- `POST` - Create new resources or trigger actions
- `PUT` - Update existing resources
- `DELETE` - Remove resources

### Authentication
All new endpoints inherit authentication from the existing `/api` route group:
- Session-based auth via cookie
- Token-based auth via `X-API-Key` header
- Protected by `s.authMiddleware`

## Compatibility

### Backward Compatibility
- All existing API endpoints remain unchanged
- New endpoints follow the same authentication and authorization patterns
- No breaking changes to existing functionality

### Manager Interface Usage
All endpoints use existing manager methods:
- `s.manager.Queue()` - Queue operations
- `s.manager.Arr()` - Arr storage operations
- `s.manager.MountManager()` - Mount operations
- `s.manager.ProviderClient()` - Debrid client access
- `s.manager.Notifications` - Notification service

No new manager methods were required; the implementation uses existing interfaces.

## Testing Recommendations

### Manual Testing
1. **Queue Operations:**
   - Add items and test pause/resume/retry
   - Verify bulk delete operations
   - Test retry-all-errors with multiple failed items

2. **Arr Management:**
   - Add new arr, verify in config file
   - Update arr settings, verify changes persist
   - Test validation with invalid credentials
   - Delete arr, verify removal from config

3. **System Operations:**
   - Check mount status with different mount types
   - Trigger mount refresh
   - View logs with different filters
   - Test restart (verify graceful shutdown)

4. **Debrid Provider Testing:**
   - List all configured providers
   - Test connectivity for each provider
   - Verify profile information is returned

### Integration Testing
- Test with curl/httpie for raw API access
- Test with Postman for full request/response inspection
- Verify all endpoints return proper status codes
- Check authentication enforcement

## Documentation

### API Documentation
- Complete API reference in `API_DOCUMENTATION.md`
- Includes request/response examples
- Covers all query parameters and request bodies
- Provides curl examples for common operations

### Migration Guide
The API documentation includes a "Migration from Browser UI" section showing which UI operations now have API equivalents.

## Performance Considerations

### Log Buffer
- Ring buffer implementation avoids unbounded memory growth
- Capped at 1000 entries (configurable via `NewLogBuffer(capacity)`)
- No disk I/O or persistence overhead
- Thread-safe but doesn't block log writers

### Queue Operations
- Bulk operations use existing `DeleteWhere()` and `UpdateWhere()` methods
- No N+1 query problems
- Retry-all uses batch processing with individual error handling

### Arr Operations
- Config save only happens after validation
- In-memory sync before disk write
- Minimal config file overhead

## Future Enhancements

Potential additions for future iterations:
1. Batch queue operations (pause/resume multiple items)
2. Queue priority management
3. Advanced log filtering (by component, time range)
4. WebSocket endpoint for real-time log streaming
5. Export logs to file
6. Scheduled tasks API (for repair scheduler)
7. Metrics/telemetry API for monitoring
8. Debrid provider add/remove/update endpoints

## Summary

This implementation successfully addresses all requirements:
- ✅ Queue Management (7 endpoints)
- ✅ Arr Management (4 endpoints) 
- ✅ Debrid Provider Management (2 endpoints)
- ✅ Notification Settings (2 endpoints)
- ✅ Mount Status/Control (2 endpoints)
- ✅ Log Viewing (1 endpoint)
- ✅ System Control (2 endpoints)

**Total: 20 new API endpoints** providing comprehensive programmatic access to all Decypharr functionality.

All functionality previously requiring the browser UI is now accessible via REST API, enabling full automation and integration with external tools.
