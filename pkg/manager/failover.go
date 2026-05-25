package manager

import (
	"context"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// processProviderFailover switches stalled torrents to another cached provider when configured.
func (m *Manager) processProviderFailover(ctx context.Context) {
	cfg := config.Get()
	if cfg == nil || cfg.FailoverTimeoutHours <= 0 {
		return
	}

	timeout := time.Duration(cfg.FailoverTimeoutHours) * time.Hour
	cutoff := time.Now().Add(-timeout)

	entries := m.queue.ListFilter("", config.ProtocolTorrent, storage.EntryStateDownloading, nil, "", true)
	for _, entry := range entries {
		if entry == nil || entry.ActiveProvider == "" {
			continue
		}
		if entry.Status == debridTypes.TorrentStatusDownloaded {
			continue
		}
		placement := entry.GetActiveProvider()
		if placement == nil {
			continue
		}
		if placement.LastProgressAt.IsZero() || placement.LastProgressAt.After(cutoff) {
			continue
		}
		if placement.Progress > 0.01 {
			continue
		}

		clients := m.orderedDebridClients("")
		if cached, ok := m.selectCachedProvider(ctx, entry.InfoHash, clients); ok && cached.Config().Name != entry.ActiveProvider {
			m.logger.Info().
				Str("name", entry.Name).
				Str("from", entry.ActiveProvider).
				Str("to", cached.Config().Name).
				Msg("Failover: switching to cached provider")

			result, err := m.fixer.FixTorrent(ctx, entry, true)
			if err != nil {
				m.logger.Error().Err(err).Str("name", entry.Name).Msg("Failover fix failed")
				continue
			}
			if result != nil && result.Success {
				entry.ActiveProvider = result.NewDebrid
				_ = m.queue.Update(entry)
			}
		}
	}
}
