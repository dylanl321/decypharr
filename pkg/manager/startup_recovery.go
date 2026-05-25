package manager

import (
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// recoverQueueOnStartup resumes interrupted queue work after a restart.
func (m *Manager) recoverQueueOnStartup() {
	entries, err := m.storage.FilterQueued(func(e *storage.Entry) bool {
		return e.State == storage.EntryStateDownloading
	})
	if err != nil || len(entries) == 0 {
		return
	}

	m.logger.Info().Int("count", len(entries)).Msg("Recovering queued entries after startup")

	for _, entry := range entries {
		if entry == nil {
			continue
		}
		// Clear orphaned in-flight flag from a crash mid-pull
		if entry.IsDownloading {
			entry.IsDownloading = false
			_ = m.queue.Update(entry)
		}

		switch entry.Status {
		case debridTypes.TorrentStatusDownloaded:
			if !entry.IsComplete {
				m.logger.Info().Str("name", entry.Name).Msg("Resuming post-debrid action after restart")
				go m.processAction(entry)
			}
		default:
			// Active debrid downloads resume via processQueuedEntries
		}
	}
}
