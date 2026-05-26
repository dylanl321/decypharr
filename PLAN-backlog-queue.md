# Backlog Queue: Accept-Then-Process Architecture

## Problem
Decypharr currently rejects torrent/NZB grabs when providers are unavailable (slots exhausted, DMCA, etc.), causing Sonarr/Radarr to retry every 12s in a tight loop. A real download client should accept the grab immediately and work on fulfilling it in the background.

## Design

### Core Principle
**Always accept grabs from arrs.** Never return an error from `torrents/add` or `sabnzbd/api?mode=addfile`. Instead, persist the request internally and process it asynchronously.

### New Entry State: `pending`

Add a new state to the entry lifecycle:

```
pending → downloading → [debrid_fetching → downloading → importing → complete]
           ↓
         error (after timeout or permanent failure on ALL providers)
```

- `pending` = accepted but not yet submitted to any debrid provider
- Existing `downloading` state = successfully submitted to a provider

### Entry Sub-States for Pending (stored as `pending_reason`)

- `slot_exhausted` — all providers at slot limit
- `provider_blocked` — all eligible providers returned DMCA/451 for this hash
- `no_eligible_provider` — no provider supports this protocol
- `rate_limited` — all providers rate-limited

### Changes Required

#### 1. `pkg/manager/processor.go` — `AddNewTorrent()`

Current: If `SendToDebrid` fails (non-slot error), return error to HTTP handler → arr gets error.

New: If `SendToDebrid` fails for ANY retryable reason, create the entry with state=`pending` and persist it. Return nil (success) so the HTTP handler returns 200 to the arr.

Only truly permanent errors (e.g., invalid magnet, no configured providers at all) should return an error.

```go
func (m *Manager) AddNewTorrent(ctx context.Context, importReq *ImportRequest) error {
    debridTorrent, err := m.SendToDebrid(ctx, importReq)
    if err != nil {
        // Determine if retryable
        if isRetryableSubmitError(err) {
            return m.queue.AddPending(importReq, classifyPendingReason(err))
        }
        return err  // truly permanent (e.g. no providers configured)
    }
    // ... existing success path
}
```

#### 2. `pkg/storage/entry.go` — New state constant

```go
const (
    EntryStatePending     EntryState = "pending"
    // ... existing states
)
```

Add fields to Entry:
```go
type Entry struct {
    // ... existing fields
    PendingReason    string     `json:"pending_reason,omitempty"`
    PendingAttempts  int        `json:"pending_attempts"`
    LastAttemptAt    *time.Time `json:"last_attempt_at,omitempty"`
    BlockedProviders []string   `json:"blocked_providers,omitempty"`  // providers that returned DMCA for this hash
}
```

#### 3. `pkg/manager/workers.go` — New background worker: `processPendingEntries()`

Runs on a ticker (every 30s–60s). For each pending entry:
1. Check if any provider now has slots available
2. Skip providers in `BlockedProviders` for this entry
3. Attempt `SendToDebrid` with eligible providers
4. On success: promote to `downloading` state
5. On failure: increment `PendingAttempts`, update `LastAttemptAt`
6. If pending for > `max_pending_hours` (config, default 6h): mark as error so arr can re-grab a different release

Use exponential backoff per entry: retry after `min(30s * 2^attempts, 15min)`.

#### 4. `pkg/manager/queue.go` — `AddPending()` method

```go
func (q *Queue) AddPending(importReq *ImportRequest, reason string) error {
    entry := &storage.Entry{
        InfoHash:      importReq.Magnet.InfoHash,
        Name:          importReq.Magnet.Name,
        // ... fill fields same as AddNewTorrent success path
        State:         storage.EntryStatePending,
        PendingReason: reason,
        Phase:         storage.DownloadPhaseQueued,
        CreatedAt:     time.Now(),
    }
    // Store the full ImportRequest so we can replay it later
    entry.ImportRequest = importReq  // or serialize to JSON field
    return q.Add(entry)
}
```

#### 5. `pkg/server/qbit/torrent.go` — Always return success

Current `addTorrent`:
```go
err = q.manager.AddNewTorrent(ctx, importReq)
if err != nil {
    return // returns error status to Sonarr
}
```

New: `AddNewTorrent` no longer returns errors for slot/DMCA issues, so the handler naturally returns 200.

#### 6. `pkg/server/sabnzbd/handlers.go` — Same for NZB path

Same principle — `AddNewNZB` should accept and backlog.

#### 7. `internal/config/config.go` — New config fields

```json
{
  "max_pending_hours": 6,
  "pending_retry_interval_seconds": 30,
  "pending_max_retry_interval_seconds": 900
}
```

Also env vars: `DECYPHARR_MAX_PENDING_HOURS`, etc.

#### 8. qBit API: Report pending items in `torrents/info`

Pending entries should appear in the qBit torrent list so Sonarr sees them as "in progress" (not missing). Map pending state to qBit `stalledDL` or `metaDL` state so Sonarr knows the client has it but hasn't started downloading.

#### 9. SABnzbd API: Report pending NZBs in queue

Same — pending NZBs should appear in `/sabnzbd/api?mode=queue` as "Queued" items.

#### 10. UI: Pending/Backlog visibility

- Add "Pending" as a filter option in the Queue page state dropdown
- Show pending entries with their reason, attempt count, time waiting, blocked providers
- Add "Retry Now" button per entry
- Add "Cancel" button (removes from queue, lets arr re-grab)
- Summary pill showing pending count

#### 11. DMCA Provider Blocklist (per-entry)

When RD returns 451 for a specific hash:
- Record `realdebrid` in `entry.BlockedProviders`
- Future retry attempts skip RD for THIS entry
- Other entries still try RD normally
- This is NOT a global blocklist — just per-hash

### qBit State Mapping for Pending Entries

| Pending Reason | qBit State | Sonarr Interprets As |
|---|---|---|
| slot_exhausted | `stalledDL` | Waiting (won't re-grab) |
| provider_blocked | `stalledDL` | Waiting (won't re-grab) |
| rate_limited | `metaDL` | Fetching metadata (won't re-grab) |

### Timeline Events

New timeline event kinds:
- `pending_accepted` — "Accepted, waiting for available provider"
- `pending_retry_failed` — "Retry #N failed: {reason}"
- `pending_promoted` — "Provider available, starting download"
- `pending_expired` — "Timed out after {hours}h, marked as error"

### Cleanup: Remove from Provider After Local Download

Once a torrent/NZB is fully downloaded locally and imported by the arr, the entry on the debrid provider is just consuming a slot. Add a cleanup mechanism:

1. **Auto-cleanup on completion** (opt-in config: `remove_completed_from_provider: true`):
   - After entry state transitions to `complete` (arr has imported), call `DeleteTorrent(id)` on the provider
   - Only for `download` action (not `symlink`/`strm` where the remote file IS the content)
   - Already partially implemented via `remove_on_complete` per-debrid config — make sure it works for pending→promoted→completed flow too

2. **Manual cleanup from UI**:
   - "Remove from provider" button on completed entries in the Queue page
   - Calls provider's delete API for that specific torrent/NZB
   - Shows confirmation since this is irreversible (can't re-download without re-submitting)
   - Batch action: select multiple completed items → "Remove all from provider"

3. **Bulk cleanup API endpoint**:
   - `POST /api/torrents/cleanup` — removes all completed entries from their providers
   - `DELETE /api/torrents/{category}/{hash}?removeFromDebrid=true` — already exists, make sure it works for pending entries too

4. **Pending entry cleanup**:
   - "Cancel" button on pending entries removes from Decypharr queue
   - Does NOT need to call provider delete (never submitted)
   - Sonarr/Radarr will see it disappear from qBit list and re-grab if episode still monitored

### Testing

1. Set TorBox minimum_free_slot higher than available → verify grab accepted, appears as pending
2. Submit DMCA'd hash → verify accepted, RD blocklisted per-entry, retries only TorBox
3. Wait for slots to free → verify auto-promotion to downloading
4. Wait past max_pending_hours → verify marked as error, removed from Sonarr queue
5. Manual retry from UI → verify re-attempts immediately

### Files to Modify

- `pkg/storage/entry.go` — new state, new fields
- `pkg/storage/types.go` — EntryStatePending constant
- `pkg/manager/processor.go` — accept-then-process logic
- `pkg/manager/workers.go` — pending processor worker
- `pkg/manager/queue.go` — AddPending method
- `pkg/server/qbit/torrent.go` — always-accept
- `pkg/server/qbit/http.go` — map pending to qBit state
- `pkg/server/sabnzbd/handlers.go` — always-accept for NZBs
- `pkg/server/assets/js/dashboard.js` — pending filter/display
- `pkg/server/templates/index.html` — pending summary pill
- `internal/config/config.go` — new config fields
- `internal/config/env.go` — env var mappings
- `CHANGELOG.md` — document the feature
