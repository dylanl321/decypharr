package manager

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

const pendingReasonAccepted = "accepted"

// acceptTorrentImport persists a pending queue row and returns immediately (no debrid I/O).
func (m *Manager) acceptTorrentImport(importReq *ImportRequest) (string, error) {
	if importReq == nil || importReq.Magnet == nil {
		return "", fmt.Errorf("magnet is required")
	}
	hash := strings.ToLower(importReq.Magnet.InfoHash)
	if existing, _ := m.queue.GetTorrent(hash); existing != nil {
		if existing.State == storage.EntryStatePending || existing.State == storage.EntryStateDownloading {
			return hash, nil
		}
	}
	if err := m.queue.UpsertPending(importReq, pendingReasonAccepted); err != nil {
		return "", err
	}
	return hash, nil
}

// acceptNZBImport persists a pending NZB row for async debrid submit.
func (m *Manager) acceptNZBImport(req *ImportRequest) (string, error) {
	if req == nil || len(req.NZBContent) == 0 {
		return "", fmt.Errorf("NZB content is required")
	}
	hash := nzbImportHash(req.NZBContent)
	if existing, _ := m.queue.GetTorrent(hash); existing != nil {
		if existing.State == storage.EntryStatePending || existing.State == storage.EntryStateDownloading {
			return hash, nil
		}
	}
	if err := m.queue.UpsertPending(req, pendingReasonAccepted); err != nil {
		return "", err
	}
	return hash, nil
}

func nzbImportHash(content []byte) string {
	sum := md5.Sum(content)
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

// SchedulePendingSubmit kicks off an immediate background debrid submit (deduped per hash).
func (m *Manager) SchedulePendingSubmit(hash string) {
	m.schedulePendingSubmit(hash)
}

func (m *Manager) schedulePendingSubmit(hash string) {
	hash = strings.ToLower(hash)
	if _, loaded := m.pendingSubmits.LoadOrStore(hash, struct{}{}); loaded {
		return
	}
	go func() {
		defer m.pendingSubmits.Delete(hash)
		m.submitPendingByHash(hash)
	}()
}

// CancelPendingEntry removes a pending queue row without debrid cleanup so *arr can re-grab.
func (m *Manager) CancelPendingEntry(hash string) error {
	hash = strings.ToLower(hash)
	entry, err := m.queue.GetTorrent(hash)
	if err != nil || entry == nil {
		return fmt.Errorf("queue item not found")
	}
	if entry.State != storage.EntryStatePending {
		return fmt.Errorf("only pending entries can be cancelled")
	}
	entry.AppendEvent(storage.TimelineRemoved, "", "Cancelled from pending queue")
	_ = m.queue.Update(entry)
	return m.queue.Delete(hash, nil)
}

func (m *Manager) submitPendingByHash(hash string) {
	entry, err := m.queue.GetTorrent(hash)
	if err != nil || entry == nil || entry.State != storage.EntryStatePending {
		return
	}
	m.submitPendingEntry(m.ctx, entry)
}

// submitTorrentImport runs debrid submit for torrents (sync API path or pending promotion).
func (m *Manager) submitTorrentImport(ctx context.Context, importReq *ImportRequest) error {
	if importReq == nil || importReq.Magnet == nil {
		return fmt.Errorf("magnet is required")
	}
	hash := strings.ToLower(importReq.Magnet.InfoHash)
	existing, _ := m.queue.GetTorrent(hash)
	if existing != nil && len(existing.BlockedProviders) > 0 {
		importReq.BlockedProviders = append([]string(nil), existing.BlockedProviders...)
	}

	debridTorrent, err := m.SendToDebrid(ctx, importReq)
	if err != nil {
		return m.handleTorrentSubmitError(err, importReq, existing)
	}

	if existing != nil && existing.State == storage.EntryStatePending {
		m.promotePendingTorrent(existing, importReq, debridTorrent)
		return nil
	}

	return m.createDownloadingTorrentEntry(importReq, debridTorrent)
}

func (m *Manager) handleTorrentSubmitError(err error, importReq *ImportRequest, existing *storage.Entry) error {
	reason, shouldPend := m.classifySubmitError(err, importReq)
	if shouldPend {
		if existing != nil && existing.State == storage.EntryStatePending {
			m.refreshPendingAfterFailedSubmit(existing, importReq, reason)
			return nil
		}
		m.logger.Warn().Msgf("Submit failed, accepting as pending: %s - %s", importReq.Magnet.Name, reason)
		if addErr := m.queue.AddPending(importReq, reason); addErr != nil {
			return fmt.Errorf("failed to add pending entry: %w", addErr)
		}
		return nil
	}

	if existing != nil && existing.State == storage.EntryStatePending {
		existing.MarkAsError(fmt.Errorf("permanent failure: %w", err))
		_ = m.queue.Update(existing)
		return nil
	}
	return fmt.Errorf("failed to submit torrent to debrid: %w", err)
}

func (m *Manager) refreshPendingAfterFailedSubmit(entry *storage.Entry, importReq *ImportRequest, reason string) {
	entry.PendingReason = reason
	entry.UpdatedAt = time.Now()
	entry.AppendEvent(storage.TimelinePendingRetryFailed, "", fmt.Sprintf("Retry #%d failed: %s", entry.PendingAttempts, reason))
	m.appendSubmitAttemptsToTimeline(entry, importReq.SubmitAttempts)
	m.mergeBlockedProviders(entry, importReq.SubmitAttempts)
	_ = m.queue.Update(entry)
}

// appendSubmitAttemptsToTimeline records per-provider fallback failures on the entry timeline.
func (m *Manager) appendSubmitAttemptsToTimeline(entry *storage.Entry, attempts []SubmitAttempt) {
	for _, attempt := range attempts {
		kind, msg := submitAttemptEvent(attempt)
		entry.AppendEvent(kind, attempt.Provider, msg)
	}
}

func (m *Manager) mergeBlockedProviders(entry *storage.Entry, attempts []SubmitAttempt) {
	for _, attempt := range attempts {
		if attempt.Code != "content_blocked" && attempt.Code != "dmca_blocked" {
			continue
		}
		found := false
		for _, blocked := range entry.BlockedProviders {
			if blocked == attempt.Provider {
				found = true
				break
			}
		}
		if !found {
			entry.BlockedProviders = append(entry.BlockedProviders, attempt.Provider)
			entry.AppendEvent(storage.TimelineProviderBlocked, attempt.Provider, attempt.Message)
		}
	}
}

func (m *Manager) promotePendingTorrent(entry *storage.Entry, importReq *ImportRequest, debridTorrent *debridTypes.Torrent) {
	m.logger.Info().
		Str("hash", entry.InfoHash).
		Str("provider", debridTorrent.Debrid).
		Msg("Pending torrent promoted to downloading")

	entry.State = storage.EntryStateDownloading
	entry.Phase = storage.DownloadPhaseDebridFetching
	entry.Status = debridTorrent.Status
	entry.PendingReason = ""
	entry.DownloadUncached = debridTorrent.DownloadUncached
	entry.UpdatedAt = time.Now()
	if importReq.QueuedAt != nil {
		waited := time.Since(*importReq.QueuedAt).Round(time.Second)
		entry.AppendEvent(storage.TimelineQueued, "", fmt.Sprintf("Waited %s for free debrid slot", waited))
	}
	for _, attempt := range importReq.SubmitAttempts {
		kind, msg := submitAttemptEvent(attempt)
		entry.AppendEvent(kind, attempt.Provider, msg)
	}
	entry.AppendEvent(storage.TimelinePendingPromoted, debridTorrent.Debrid, fmt.Sprintf("Provider available after %d attempts", entry.PendingAttempts))
	entry.AppendEvent(storage.TimelineDebridSubmitted, debridTorrent.Debrid, "")
	_ = m.queue.Update(entry)
	go m.processNewTorrent(entry, debridTorrent)
}

func (m *Manager) createDownloadingTorrentEntry(importReq *ImportRequest, debridTorrent *debridTypes.Torrent) error {
	torrent := &storage.Entry{
		InfoHash:         importReq.Magnet.InfoHash,
		Name:             importReq.Magnet.Name,
		OriginalFilename: importReq.Magnet.Name,
		Protocol:         config.ProtocolTorrent,
		Size:             importReq.Magnet.Size,
		Bytes:            importReq.Magnet.Size,
		Magnet:           importReq.Magnet.Link,
		Category:         importReq.Arr.Name,
		SavePath:         config.ResolveCategoryPath(importReq.Arr.Name, importReq.DownloadFolder, importReq.Arr.Name),
		Status:           debridTypes.TorrentStatusDownloading,
		State:            storage.EntryStateDownloading,
		Phase:            storage.DownloadPhaseDebridFetching,
		Progress:         0,
		Action:           importReq.Action,
		DownloadUncached: debridTorrent.DownloadUncached,
		CallbackURL:      importReq.CallBackUrl,
		SkipMultiSeason:  importReq.SkipMultiSeason,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		AddedOn:          time.Now(),
		Providers:        make(map[string]*storage.ProviderEntry),
		Files:            make(map[string]*storage.File),
		Tags:             []string{},
	}
	torrent.ContentPath = torrent.DownloadPath()
	torrent.AppendEvent(storage.TimelineAdded, "", "Added via "+importReq.Arr.Name)
	if importReq.QueuedAt != nil {
		waited := time.Since(*importReq.QueuedAt).Round(time.Second)
		torrent.AppendEvent(storage.TimelineQueued, "", fmt.Sprintf("Waited %s for free debrid slot", waited))
	}
	for _, attempt := range importReq.SubmitAttempts {
		kind, msg := submitAttemptEvent(attempt)
		torrent.AppendEvent(kind, attempt.Provider, msg)
	}
	torrent.AppendEvent(storage.TimelineDebridSubmitted, debridTorrent.Debrid, "")

	if err := m.queue.Add(torrent); err != nil {
		return fmt.Errorf("failed to add torrent to queue: %w", err)
	}
	go m.processNewTorrent(torrent, debridTorrent)
	return nil
}

// submitNZBDebridImport submits an NZB to debrid, promoting a pending row when present.
func (m *Manager) submitNZBDebridImport(ctx context.Context, req *ImportRequest, entry *storage.Entry) (string, error) {
	if req == nil || len(req.NZBContent) == 0 {
		return "", fmt.Errorf("NZB content is required")
	}
	if entry == nil {
		hash := nzbImportHash(req.NZBContent)
		entry, _ = m.queue.GetTorrent(hash)
	}
	if entry != nil && len(entry.BlockedProviders) > 0 {
		req.BlockedProviders = append([]string(nil), entry.BlockedProviders...)
	}

	usenetDownload, err := m.SendToNZBDebrid(ctx, req)
	if err != nil {
		if handleErr := m.handleNZBSubmitError(err, req, entry); handleErr != nil {
			return "", handleErr
		}
		if entry != nil {
			return entry.InfoHash, nil
		}
		return nzbImportHash(req.NZBContent), nil
	}

	if entry != nil && entry.State == storage.EntryStatePending {
		m.promotePendingNZB(entry, req, usenetDownload)
		return entry.InfoHash, nil
	}

	return m.createDownloadingNZBEntry(req, usenetDownload)
}

func (m *Manager) handleNZBSubmitError(err error, req *ImportRequest, existing *storage.Entry) error {
	reason, shouldPend := m.classifyNZBSubmitError(err, req)
	if shouldPend {
		if existing != nil && existing.State == storage.EntryStatePending {
			m.refreshPendingAfterFailedSubmit(existing, req, reason)
			return nil
		}
		m.logger.Warn().Msgf("NZB submit failed, accepting as pending: %s - %s", req.Name, reason)
		if addErr := m.queue.AddPending(req, reason); addErr != nil {
			return fmt.Errorf("failed to add pending nzb entry: %w", addErr)
		}
		return nil
	}

	if existing != nil && existing.State == storage.EntryStatePending {
		existing.MarkAsError(fmt.Errorf("permanent failure: %w", err))
		_ = m.queue.Update(existing)
		return nil
	}
	return fmt.Errorf("failed to submit nzb to debrid: %w", err)
}

func (m *Manager) promotePendingNZB(entry *storage.Entry, req *ImportRequest, usenetDownload *debridTypes.UsenetDownload) {
	m.logger.Info().
		Str("hash", entry.InfoHash).
		Str("provider", usenetDownload.Debrid).
		Msg("Pending NZB promoted to downloading")

	entry.State = storage.EntryStateDownloading
	entry.Phase = storage.DownloadPhaseDebridFetching
	entry.Status = usenetDownload.Status
	entry.PendingReason = ""
	entry.Name = cmpOr(usenetDownload.Name, entry.Name)
	entry.DownloadUncached = usenetDownload.DownloadUncached
	entry.UpdatedAt = time.Now()
	if req.QueuedAt != nil {
		waited := time.Since(*req.QueuedAt).Round(time.Second)
		entry.AppendEvent(storage.TimelineQueued, "", fmt.Sprintf("Waited %s for free debrid slot", waited))
	}
	for _, attempt := range req.SubmitAttempts {
		kind, msg := submitAttemptEvent(attempt)
		entry.AppendEvent(kind, attempt.Provider, msg)
	}
	entry.AppendEvent(storage.TimelinePendingPromoted, usenetDownload.Debrid, fmt.Sprintf("Provider available after %d attempts", entry.PendingAttempts))
	entry.AppendEvent(storage.TimelineDebridSubmitted, usenetDownload.Debrid, "")
	backfillEntryFromDebrid(entry, usenetDownload.AsTorrent())
	entry.DebridProgress = usenetDownload.Progress / 100.0
	entry.Progress = entry.DebridProgress
	_ = m.queue.Update(entry)
	go m.processNewNZBDebrid(entry, usenetDownload)
}

func (m *Manager) createDownloadingNZBEntry(req *ImportRequest, usenetDownload *debridTypes.UsenetDownload) (string, error) {
	entry := &storage.Entry{
		InfoHash:         usenetDownload.Hash,
		Name:             cmpOr(usenetDownload.Name, req.Name),
		OriginalFilename: cmpOr(usenetDownload.OriginalFilename, req.Name),
		Size:             usenetDownload.GetSize(),
		Protocol:         config.ProtocolNZB,
		Bytes:            usenetDownload.GetSize(),
		Category:         req.Arr.Name,
		SavePath:         config.ResolveCategoryPath(req.Arr.Name, req.DownloadFolder, req.Arr.Name),
		Status:           debridTypes.TorrentStatusDownloading,
		State:            storage.EntryStateDownloading,
		Phase:            storage.DownloadPhaseDebridFetching,
		Progress:         0,
		Action:           req.Action,
		DownloadUncached: usenetDownload.DownloadUncached,
		CallbackURL:      req.CallBackUrl,
		SkipMultiSeason:  req.SkipMultiSeason,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		AddedOn:          time.Now(),
		Providers:        make(map[string]*storage.ProviderEntry),
		Files:            make(map[string]*storage.File),
		Tags:             []string{},
	}
	entry.ContentPath = entry.DownloadPath()
	backfillEntryFromDebrid(entry, usenetDownload.AsTorrent())
	entry.DebridProgress = usenetDownload.Progress / 100.0
	entry.Progress = entry.DebridProgress
	entry.AppendEvent(storage.TimelineAdded, "", "Added via "+req.Arr.Name)
	if req.QueuedAt != nil {
		waited := time.Since(*req.QueuedAt).Round(time.Second)
		entry.AppendEvent(storage.TimelineQueued, "", fmt.Sprintf("Waited %s for free debrid slot", waited))
	}
	for _, attempt := range req.SubmitAttempts {
		kind, msg := submitAttemptEvent(attempt)
		entry.AppendEvent(kind, attempt.Provider, msg)
	}
	entry.AppendEvent(storage.TimelineDebridSubmitted, usenetDownload.Debrid, "")

	if err := m.queue.Add(entry); err != nil {
		return "", fmt.Errorf("failed to add nzb to queue: %w", err)
	}
	go m.processNewNZBDebrid(entry, usenetDownload)
	return entry.InfoHash, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (m *Manager) importReqFromPendingEntry(entry *storage.Entry) *ImportRequest {
	if entry == nil {
		return nil
	}
	if entry.IsTorrent() {
		magnet, err := utils.GetMagnetInfo(entry.Magnet, m.config.AlwaysRmTrackerUrls)
		if err != nil {
			magnet = utils.ConstructMagnet(entry.InfoHash, entry.Name)
		}
		arr := m.arr.GetOrCreate(entry.Category)
		uncached := entry.DownloadUncached
		req := NewTorrentRequest(
			"",
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
	if entry.IsNZB() {
		if len(entry.NZBContent) == 0 {
			m.logger.Error().Str("hash", entry.InfoHash).Msg("Cannot retry NZB pending entry: NZB content not stored")
			entry.MarkAsError(fmt.Errorf("cannot retry: NZB content not available"))
			_ = m.queue.Update(entry)
			return nil
		}
		arr := m.arr.GetOrCreate(entry.Category)
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

func (m *Manager) submitPendingEntry(ctx context.Context, entry *storage.Entry) {
	importReq := m.importReqFromPendingEntry(entry)
	if importReq == nil {
		return
	}

	entry.PendingAttempts++
	now := time.Now()
	entry.LastAttemptAt = &now
	entry.UpdatedAt = now
	_ = m.queue.Update(entry)

	var err error
	if entry.IsNZB() {
		_, err = m.submitNZBDebridImport(ctx, importReq, entry)
	} else {
		err = m.submitTorrentImport(ctx, importReq)
	}
	if err != nil {
		m.logger.Error().Err(err).Str("hash", entry.InfoHash).Msg("Pending submit failed")
	}
}
