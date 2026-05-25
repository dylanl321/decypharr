package manager

import (
	"fmt"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// processStaleQueueEntries marks or recovers queue items stuck without progress.
func (m *Manager) processStaleQueueEntries() {
	cfg := config.Get()
	if cfg == nil {
		return
	}

	staleHours := cfg.StaleDownloadHours
	stuckMinutes := cfg.StuckCompleteMinutes
	if stuckMinutes <= 0 {
		stuckMinutes = 30
	}

	entries := m.queue.ListFilter("", config.ProtocolAll, storage.EntryStateDownloading, nil, "", true)
	now := time.Now()

	for _, entry := range entries {
		if entry == nil || entry.IsComplete {
			continue
		}

		// Stuck at debrid-complete but local pull never finished
		if stuckMinutes > 0 {
			stuckCutoff := now.Add(-time.Duration(stuckMinutes) * time.Minute)
			debridDone := entry.Status == debridTypes.TorrentStatusDownloaded || entry.DebridProgress >= 1.0
			if debridDone && entry.State == storage.EntryStateDownloading && entry.UpdatedAt.Before(stuckCutoff) {
				if entry.IsDownloading {
					m.logger.Warn().Str("name", entry.Name).Msg("Stuck local pull detected, resetting IsDownloading")
					entry.IsDownloading = false
					_ = m.queue.Update(entry)
					if entry.Status == debridTypes.TorrentStatusDownloaded {
						go m.processAction(entry)
					}
					continue
				}
				m.logger.Warn().Str("name", entry.Name).Msg("Stuck at debrid-complete, re-running post-download action")
				go m.processAction(entry)
				continue
			}
		}

		if staleHours <= 0 {
			continue
		}

		staleCutoff := now.Add(-time.Duration(staleHours) * time.Hour)
		if entry.UpdatedAt.After(staleCutoff) {
			continue
		}

		placement := entry.GetActiveProvider()
		if placement != nil && !placement.LastProgressAt.IsZero() && placement.LastProgressAt.After(staleCutoff) {
			continue
		}

		if entry.DebridProgress > 0.01 || entry.Progress > 0.01 {
			continue
		}

		if entry.Status == debridTypes.TorrentStatusDownloading && entry.Seeders == 0 {
			m.logger.Warn().Str("name", entry.Name).Msg("Stale download with no progress, marking as error")
			entry.MarkAsError(fmt.Errorf("stale: no progress for %d hours", staleHours))
			_ = m.queue.Update(entry)
		}
	}
}
