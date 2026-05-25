# API Enhancements Implementation Summary

## Overview
This document describes the new API endpoints added to Decypharr to provide comprehensive health monitoring, debrid account status, queue statistics, and runtime metrics.

## New Endpoints

### 1. Health Check API (`GET /api/health`)

**Purpose:** Provides detailed health status of all Decypharr components with latency measurements and connectivity checks.

**Response Format:**
```json
{
  "status": "healthy|degraded|unhealthy",
  "uptime_seconds": 12345,
  "version": "v1.2.3",
  "checks": {
    "qbit_api": {
      "status": "up",
      "latency_ms": 2
    },
    "webdav": {
      "status": "up",
      "latency_ms": 1
    },
    "storage": {
      "status": "up",
      "latency_ms": 5
    },
    "mount": {
      "status": "up",
      "detail": {
        "type": "rclone"
      }
    },
    "debrid_providers": [
      {
        "name": "torbox",
        "status": "up",
        "latency_ms": 150,
        "account_expiry": "2025-12-01T00:00:00Z",
        "username": "user@example.com",
        "premium": true
      }
    ],
    "arr_connections": [
      {
        "name": "sonarr",
        "status": "up",
        "latency_ms": 50
      }
    ]
  }
}
```

**Status Logic:**
- `healthy`: All components operational
- `degraded`: Non-critical services (arr connections, webdav) have issues
- `unhealthy`: Critical services (storage, mount, debrid) are down

**HTTP Status Codes:**
- `200 OK`: healthy or degraded
- `503 Service Unavailable`: unhealthy

**Components Checked:**
- **qbit_api**: qBittorrent emulation layer
- **webdav**: WebDAV service
- **storage**: Database backend
- **mount**: Mount manager (rclone/DFS)
- **debrid_providers**: All configured debrid services with account info
- **arr_connections**: Connectivity to all configured *arr instances

### 2. Debrid Account Status (`GET /api/debrid/status`)

**Purpose:** Returns detailed account information for all configured debrid providers.

**Response Format:**
```json
{
  "providers": [
    {
      "name": "torbox",
      "type": "torbox",
      "status": "active",
      "premium": true,
      "expiry": "2025-12-01T00:00:00Z",
      "username": "user@example.com",
      "email": "user@example.com",
      "points": 1234
    },
    {
      "name": "realdebrid",
      "type": "realdebrid",
      "status": "error",
      "error": "connection timeout"
    }
  ]
}
```

**Fields:**
- `name`: Provider identifier
- `type`: Provider type (torbox, realdebrid, alldebrid, debridlink)
- `status`: "active" or "error"
- `premium`: Whether account has premium status
- `expiry`: Premium expiration date (ISO 8601)
- `username`: Account username
- `email`: Account email
- `points`: Available points/credits (if applicable)
- `error`: Error message if status is "error"

### 3. Queue Summary (`GET /api/queue/summary`)

**Purpose:** Lightweight endpoint providing queue statistics without listing all items.

**Response Format:**
```json
{
  "total": 27,
  "by_state": {
    "downloading": 5,
    "completed": 15,
    "error": 7,
    "queued": 0
  },
  "by_category": {
    "sonarr": 20,
    "radarr": 7
  },
  "by_protocol": {
    "torrent": 25,
    "usenet": 2
  },
  "errors": [
    {
      "hash": "abc123",
      "name": "Some.Show.S01E01",
      "error": "download client unavailable",
      "added_on": "2026-05-25T10:30:00Z"
    }
  ]
}
```

**Fields:**
- `total`: Total number of items in queue
- `by_state`: Breakdown by download state
- `by_category`: Breakdown by arr category
- `by_protocol`: Breakdown by protocol (torrent/usenet)
- `errors`: List of items currently in error state with details

### 4. Stats/Metrics (`GET /api/stats`)

**Purpose:** Exposes the existing stats collector data via the API.

**Response Format:**
```json
{
  "system": {
    "heap_alloc_mb": "125.50MB",
    "memory_used": "89.23MB",
    "gc_cycles": 42,
    "goroutines": 156,
    "num_cpu": 8,
    "os": "linux",
    "arch": "amd64",
    "go_version": "go1.21.0",
    "uptime_seconds": 86400,
    "uptime": "24h0m0s",
    "start_time": "2026-05-24 10:30:00"
  },
  "debrids": [
    {
      "profile": {
        "name": "torbox",
        "username": "user@example.com",
        "premium": 1234567890,
        "expiration": "2025-12-01T00:00:00Z"
      },
      "library": {
        "total": 250,
        "bad": 5,
        "active_links": 12
      },
      "accounts": []
    }
  ],
  "mount": {
    "ready": true,
    "enabled": true,
    "type": "rclone",
    "detail": {}
  },
  "usenet": {},
  "active_streams": {
    "count": 3,
    "streams": []
  },
  "storage": {
    "db_size": 1048576,
    "total_entries": 250
  },
  "queue": {
    "pending": 5
  },
  "arrs": {
    "count": 2,
    "names": ["sonarr", "radarr"]
  },
  "repair": {
    "enabled": true,
    "active": false,
    "health": {
      "healthy": 240,
      "broken": 5,
      "unknown": 5
    }
  }
}
```

## Implementation Files

### New Files Created
1. **`pkg/server/api_health.go`**: Health check endpoint implementation
   - `handleHealth()`: Main handler
   - `checkQbitAPI()`: Check qBit emulation
   - `checkWebDAV()`: Check WebDAV service
   - `checkStorage()`: Check database
   - `checkMount()`: Check mount manager
   - `checkDebridProviders()`: Check all debrid services
   - `checkArrConnections()`: Check arr connectivity
   - `determineOverallStatus()`: Calculate overall health status

2. **`pkg/server/api_status.go`**: Debrid status and queue summary endpoints
   - `handleDebridStatus()`: Debrid account status handler
   - `handleQueueSummary()`: Queue summary handler

### Modified Files
1. **`pkg/server/routes.go`**: Added new API routes under `/api` group:
   ```go
   // Health and Status
   r.Get("/health", s.handleHealth)
   r.Get("/stats", s.stats.Handler())
   r.Get("/debrid/status", s.handleDebridStatus)
   r.Get("/queue/summary", s.handleQueueSummary)
   ```

## Implementation Notes

### Design Decisions

1. **Health Check Granularity**: Each component check includes latency measurement for performance monitoring.

2. **Status Levels**: Three-tier status system:
   - `healthy`: All systems operational
   - `degraded`: Non-critical failures (arr connections)
   - `unhealthy`: Critical failures (storage, debrid, mount)

3. **Reuse of Existing Infrastructure**: 
   - Stats endpoint uses existing `pkg/stats/collector.go`
   - Queue operations leverage existing `manager.Queue()` methods
   - Debrid checks use existing `Client.GetProfile()` interface

4. **Error Handling**: All endpoints handle errors gracefully and return partial data when possible.

5. **Performance**: 
   - Health checks are fast (<100ms typical)
   - Queue summary uses in-memory filtering
   - Stats collector runs background updates every 5 seconds

### Authentication

All endpoints are protected by the existing auth middleware (`authMiddleware`) in the `/api` route group.

### Testing Recommendations

1. **Health Endpoint**: Test with various component failures to verify status calculation
2. **Debrid Status**: Test with multiple providers configured
3. **Queue Summary**: Test with empty queue, full queue, and error states
4. **Stats**: Verify data accuracy against actual system state

### Future Enhancements

Potential improvements for future iterations:

1. Add pagination to error list in queue summary
2. Include historical metrics in stats endpoint
3. Add WebSocket support for real-time health updates
4. Include bandwidth usage per debrid provider
5. Add configurable health check intervals
6. Support for custom health check plugins
