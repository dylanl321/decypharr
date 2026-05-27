package manager

import (
	"errors"
	"math"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// processTransientLocalRetries re-invokes local pull for downloading entries left in a
// transient error state (e.g. Torbox 502) after exponential backoff.
func (m *Manager) processTransientLocalRetries() {
	cfg := config.Get()
	if cfg == nil {
		return
	}
	minRetry := cfg.PendingRetryIntervalSeconds
	if minRetry <= 0 {
		minRetry = 30
	}
	maxRetry := cfg.PendingMaxRetryIntervalSeconds
	if maxRetry <= 0 {
		maxRetry = 900
	}

	entries := m.queue.ListFilter("", config.ProtocolAll, storage.EntryStateDownloading, nil, "", false)
	now := time.Now()
	for _, entry := range entries {
		if entry == nil || entry.IsDownloading || entry.LastError == "" {
			continue
		}
		if !m.canRetryLocalPull(entry) {
			continue
		}
		if !isTransientDownloadError(errors.New(entry.LastError)) {
			continue
		}
		ref := entry.UpdatedAt
		if entry.LastErrorTime != nil {
			ref = *entry.LastErrorTime
		}
		backoffSec := float64(minRetry) * math.Pow(2, float64(entry.ErrorCount))
		if backoffSec > float64(maxRetry) {
			backoffSec = float64(maxRetry)
		}
		if now.Sub(ref) < time.Duration(backoffSec)*time.Second {
			continue
		}
		m.logger.Info().
			Str("hash", entry.InfoHash).
			Str("name", entry.Name).
			Msg("Auto-retrying transient local download failure")
		if err := m.retryLocalPull(entry); err != nil {
			m.logger.Warn().Err(err).Str("hash", entry.InfoHash).Msg("Auto-retry local pull failed")
		}
	}
}
