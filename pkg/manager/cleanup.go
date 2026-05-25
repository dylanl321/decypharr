package manager

import (
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// maybeCleanupOnComplete is invoked from completeEntry to optionally delete the entry from
// its debrid provider(s) and/or remove it from Decypharr's queue. It is safe to call as a
// goroutine and is a no-op when no cleanup flags are set.
func (m *Manager) maybeCleanupOnComplete(entry *storage.Entry) {
	if entry == nil {
		return
	}

	cfg := config.Get()
	cleanup := cfg.CleanupOnComplete

	// Per-debrid `remove_on_complete` should still trigger provider cleanup even when no
	// global flag is set. Treat that as an implicit provider-cleanup intent.
	hasPerDebridFlag := false
	for _, dc := range cfg.Debrids {
		if dc.RemovesOnComplete() {
			hasPerDebridFlag = true
			break
		}
	}

	if !cleanup.RemovesFromProvider() && !cleanup.RemovesFromQueue() && !hasPerDebridFlag {
		return
	}

	if !actionAllowedForCleanup(cleanup, entry.Action) {
		m.logger.Debug().
			Str("infohash", entry.InfoHash).
			Str("action", string(entry.Action)).
			Msg("Cleanup skipped: action not in cleanup_on_complete.actions")
		return
	}

	if d, err := utils.ParseDuration(cleanup.Delay); err == nil && d > 0 {
		time.Sleep(d)
	}

	m.runCleanup(entry, cleanup, cfg)
}

func (m *Manager) runCleanup(entry *storage.Entry, cleanup config.CleanupOnComplete, cfg *config.Config) {
	globalProvider := cleanup.RemovesFromProvider()

	for name, placement := range entry.Providers {
		if placement == nil {
			continue
		}
		shouldRemove := globalProvider
		if dc := findDebridConfig(cfg, name); dc != nil && dc.RemovesOnComplete() {
			shouldRemove = true
		}
		if !shouldRemove {
			continue
		}
		if err := m.RemoveFromProvider(entry, placement); err != nil {
			m.logger.Warn().
				Err(err).
				Str("infohash", entry.InfoHash).
				Str("provider", name).
				Msg("Cleanup: failed to remove placement from provider")
			continue
		}
		m.logger.Info().
			Str("infohash", entry.InfoHash).
			Str("provider", name).
			Msg("Cleanup: removed completed placement from provider")
	}

	if cleanup.RemovesFromQueue() {
		// Provider placements are already handled above; pass false to avoid double-deletion.
		if err := m.DeleteEntry(entry.InfoHash, false); err != nil {
			m.logger.Warn().
				Err(err).
				Str("infohash", entry.InfoHash).
				Msg("Cleanup: failed to remove completed entry from queue")
			return
		}
		m.logger.Info().
			Str("infohash", entry.InfoHash).
			Str("name", entry.Name).
			Msg("Cleanup: removed completed entry from queue")
	}
}

func actionAllowedForCleanup(c config.CleanupOnComplete, action config.DownloadAction) bool {
	allowed := c.Actions
	if len(allowed) == 0 {
		// Default: only `download` actions are eligible for cleanup. Symlink/strm need the
		// provider copy to remain mounted/streamable.
		allowed = []config.DownloadAction{config.DownloadActionDownload}
	}
	for _, a := range allowed {
		if a == action {
			return true
		}
	}
	return false
}

func findDebridConfig(cfg *config.Config, name string) *config.Debrid {
	for i := range cfg.Debrids {
		if cfg.Debrids[i].Name == name {
			return &cfg.Debrids[i]
		}
	}
	return nil
}