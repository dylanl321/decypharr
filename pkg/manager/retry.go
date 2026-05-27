package manager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// RetryQueueEntry re-attempts a failed or pending queue item without requiring a new *arr grab.
func (m *Manager) RetryQueueEntry(ctx context.Context, hash string) error {
	hash = strings.ToLower(hash)
	entry, err := m.queue.GetTorrent(hash)
	if err != nil || entry == nil {
		return fmt.Errorf("queue item not found")
	}

	switch entry.State {
	case storage.EntryStatePending:
		if err := m.queue.UpdateWhere(
			func(t *storage.Entry) bool { return t.InfoHash == hash },
			func(t *storage.Entry) bool {
				t.LastAttemptAt = nil
				t.PendingAttempts = 0
				t.UpdatedAt = time.Now()
				return true
			},
		); err != nil {
			return err
		}
		m.SchedulePendingSubmit(hash)
		return nil

	case storage.EntryStateError:
		if m.canRetryLocalPull(entry) {
			return m.retryLocalPull(entry)
		}
		return m.retryDebridSubmit(ctx, entry)

	case storage.EntryStateDownloading:
		if entry.LastError != "" && !entry.IsDownloading && m.canRetryLocalPull(entry) {
			return m.retryLocalPull(entry)
		}
		return fmt.Errorf("entry is already downloading")

	default:
		return fmt.Errorf("cannot retry entry in state %q", entry.State)
	}
}

// canRetryLocalPull is true when debrid already finished and we only need to re-pull files locally.
func (m *Manager) canRetryLocalPull(entry *storage.Entry) bool {
	if entry == nil || entry.ActiveProvider == "" {
		return false
	}
	if len(entry.GetActiveFiles()) == 0 {
		return false
	}
	if entry.DebridProgress >= 1.0 || entry.Status == types.TorrentStatusDownloaded {
		return true
	}
	if entry.Phase == storage.DownloadPhaseDownloading || entry.LocalProgress > 0 {
		return true
	}
	for _, ev := range entry.Timeline {
		switch ev.Kind {
		case storage.TimelineDebridReady, storage.TimelineLocalDownloadStart:
			return true
		}
	}
	return false
}

func (m *Manager) retryLocalPull(entry *storage.Entry) error {
	if entry.HasPartialFileFailures() {
		entry.RetryFailedFilesOnly = true
	}
	m.linkService.InvalidateEntryValidation(context.Background(), entry)

	entry.State = storage.EntryStateDownloading
	entry.IsDownloading = false
	entry.LastError = ""
	entry.LastErrorTime = nil
	entry.Phase = storage.DownloadPhaseDownloading
	entry.LocalProgress = 0
	entry.SizeDownloaded = 0
	if entry.Action == config.DownloadActionDownload {
		entry.Progress = entry.DebridProgress*0.5 + entry.LocalProgress*0.5
	} else {
		entry.Progress = entry.DebridProgress
	}
	entry.UpdatedAt = time.Now()
	entry.AppendEvent(storage.TimelineLocalDownloadStart, entry.ActiveProvider, "Retrying local download")
	if err := m.queue.Update(entry); err != nil {
		return err
	}

	m.logger.Info().
		Str("hash", entry.InfoHash).
		Str("name", entry.Name).
		Str("provider", entry.ActiveProvider).
		Msg("Retrying local file pull")

	go func(e *storage.Entry) {
		if err := m.downloader.download(e); err != nil {
			m.downloader.markAsError(e, err)
		}
	}(entry)
	return nil
}

func (m *Manager) retryDebridSubmit(ctx context.Context, entry *storage.Entry) error {
	importReq := m.importReqFromEntry(entry)
	if importReq == nil {
		return fmt.Errorf("cannot reconstruct import request for retry")
	}

	entry.State = storage.EntryStateDownloading
	entry.IsDownloading = false
	entry.LastError = ""
	entry.LastErrorTime = nil
	entry.PendingReason = ""
	entry.UpdatedAt = time.Now()
	entry.AppendEvent(storage.TimelineDebridSubmitted, entry.ActiveProvider, "Retrying debrid submit")
	if err := m.queue.Update(entry); err != nil {
		return err
	}

	go func() {
		if entry.IsNZB() {
			if _, err := m.submitNZBDebridImport(ctx, importReq, entry); err != nil {
				m.logger.Error().Err(err).Str("hash", entry.InfoHash).Msg("NZB debrid retry failed")
			}
			return
		}
		if err := m.submitTorrentImport(ctx, importReq); err != nil {
			m.logger.Error().Err(err).Str("hash", entry.InfoHash).Msg("Torrent debrid retry failed")
		}
	}()
	return nil
}

func (m *Manager) importReqFromEntry(entry *storage.Entry) *ImportRequest {
	if entry == nil {
		return nil
	}
	arr := m.arr.GetOrCreate(entry.Category)
	if entry.IsTorrent() {
		magnet, err := utils.GetMagnetInfo(entry.Magnet, m.config.AlwaysRmTrackerUrls)
		if err != nil {
			magnet = utils.ConstructMagnet(entry.InfoHash, entry.Name)
		}
		uncached := entry.DownloadUncached
		req := NewTorrentRequest(
			entry.ActiveProvider,
			entry.SavePath,
			magnet,
			arr,
			entry.Action,
			&uncached,
			entry.CallbackURL,
			ImportTypeAPI,
			entry.SkipMultiSeason,
		)
		req.BlockedProviders = append([]string(nil), entry.BlockedProviders...)
		return req
	}
	if entry.IsNZB() && len(entry.NZBContent) > 0 {
		req := NewNZBRequest(
			entry.Name,
			entry.SavePath,
			entry.NZBContent,
			arr,
			entry.Action,
			entry.CallbackURL,
			ImportTypeAPI,
			entry.SkipMultiSeason,
		)
		req.BlockedProviders = append([]string(nil), entry.BlockedProviders...)
		return req
	}
	return nil
}
