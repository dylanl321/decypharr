package manager

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/customerror"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/notifications"
	"github.com/sirrobot01/decypharr/pkg/debrid/common"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
	"github.com/sirrobot01/decypharr/pkg/usenet"
)

// AddNewTorrent creates a torrent from import request and processes it
func (m *Manager) AddNewTorrent(ctx context.Context, importReq *ImportRequest) error {
	var (
		debridTorrent *debridTypes.Torrent
		err           error
	)

	debridTorrent, err = m.SendToDebrid(ctx, importReq)
	if err != nil {
		// Check if too many active downloads
		var customErr *customerror.Error
		if errors.As(err, &customErr) && customErr.Code == "too_many_active_downloads" {
			m.logger.Warn().Msgf("Too many active downloads, marking as queued: %s", importReq.Magnet.Name)
			if err := m.queue.ReQueue(importReq); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("failed to submit torrent to debrid: %w", err)
	}

	// Create managed torrent with InfoHash as primary key
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

	// Add to queue
	if err := m.queue.Add(torrent); err != nil {
		return fmt.Errorf("failed to add torrent to queue: %w", err)
	}

	// Parse in background
	go m.processNewTorrent(torrent, debridTorrent)

	return nil
}

func (m *Manager) processQueuedEntries() {
	queueEntries := m.queue.ListFilter("", config.ProtocolAll, storage.EntryStateDownloading, nil, "", true)
	if len(queueEntries) == 0 {
		return
	}
	for _, entry := range queueEntries {
		// Parse only active downloading torrents
		if entry.State != storage.EntryStateDownloading {
			continue
		}
		// Skip entries that are actively being downloading
		if entry.IsDownloading {
			continue
		}
		// Skip if a previous tick's goroutine hasn't finished yet for this hash.
		if _, loaded := m.processingEntries.LoadOrStore(entry.InfoHash, struct{}{}); loaded {
			continue
		}
		if entry.IsTorrent() {
			if entry.ActiveProvider != "" {
				go m.processQueuedTorrent(entry)
			} else {
				m.processingEntries.Delete(entry.InfoHash)
			}
		} else if entry.IsNZB() {
			go m.processQueuedNZB(entry)
		} else {
			m.processingEntries.Delete(entry.InfoHash)
		}
	}
}

func (m *Manager) processQueuedNZB(entry *storage.Entry) {
	defer m.processingEntries.Delete(entry.InfoHash)
	if !entry.IsNNTPNZB() {
		m.processQueuedNZBDebrid(entry)
		return
	}
	if m.usenet == nil {
		m.logger.Error().Str("name", entry.Name).Msg("Usenet client not configured for NNTP NZB")
		entry.MarkAsError(fmt.Errorf("usenet client not configured"))
		_ = m.queue.Update(entry)
		return
	}
	// Check if the nzb is already processed
	metadata, err := m.usenet.GetNZB(entry.InfoHash)
	if err != nil {
		m.logger.Error().Err(err).Str("name", entry.Name).Msg("Error getting NZB metadata")
		entry.MarkAsError(err)
		_ = m.queue.Update(entry)
		return
	}
	if metadata == nil {
		m.logger.Error().Str("name", entry.Name).Msg("NZB metadata not found")
		entry.MarkAsError(fmt.Errorf("nzb metadata not found"))
		_ = m.queue.Update(entry)
		return
	}
	switch metadata.Status {
	case usenet.NZBStatusFailed:
		m.logger.Error().Str("name", entry.Name).Msg("NZB processing failed")
		entry.MarkAsError(fmt.Errorf("nzb processing failed"))
		_ = m.queue.Update(entry)
		return
	case usenet.NZBStatusParsing, usenet.NZBStatusDownloading:
		// Still processing, skip for now
		return
	case usenet.NZBStatusCompleted:
		if err := m.processNZB(context.Background(), entry, metadata); err != nil {
			m.logger.Error().Err(err).Str("name", entry.Name).Msg("Error processing queued NZB")
			entry.MarkAsError(err)
			_ = m.queue.Update(entry)
			return
		}
	default:
		m.logger.Error().Str("name", entry.Name).Msgf("Unknown NZB status: %s", metadata.Status)
		entry.MarkAsError(fmt.Errorf("unknown nzb status: %s", metadata.Status))
		_ = m.queue.Update(entry)
		return
	}
}

func (m *Manager) processQueuedTorrent(entry *storage.Entry) {
	defer m.processingEntries.Delete(entry.InfoHash)
	placement := entry.GetActiveProvider()
	if placement == nil {
		m.logger.Error().Str("name", entry.Name).Msg("No active placement found for queued entry")
		entry.MarkAsError(fmt.Errorf("no active placement found"))
		_ = m.queue.Update(entry)
		return
	}

	client := m.ProviderClient(entry.ActiveProvider)
	if client == nil {
		m.logger.Error().Str("debrid", entry.ActiveProvider).Msg("Provider client not found")
		entry.MarkAsError(fmt.Errorf("debrid client not found: %s", entry.ActiveProvider))
		_ = m.queue.Update(entry)
		return
	}

	magnet, err := utils.GetMagnetInfo(entry.Magnet, m.config.AlwaysRmTrackerUrls)
	if err != nil {
		magnet = utils.ConstructMagnet(entry.InfoHash, entry.Name)
	}

	arr := m.arr.GetOrCreate(entry.Category)

	debridTorrent := &debridTypes.Torrent{
		Id:               placement.ID,
		InfoHash:         entry.InfoHash,
		Magnet:           magnet,
		Name:             magnet.Name,
		Arr:              arr,
		Size:             entry.Size,
		Files:            make(map[string]debridTypes.File),
		DownloadUncached: entry.DownloadUncached,
	}

	dbT, err := client.CheckStatus(debridTorrent)
	if err != nil {
		m.logger.Error().Err(err).Str("name", entry.Name).Msg("Error checking status")
		entry.MarkAsError(err)
		_ = m.queue.Update(entry)

		// Delete from debrid on error
		go func() {
			if dbT != nil && dbT.Id != "" {
				_ = client.DeleteTorrent(dbT.Id)
			}
		}()
		return
	}

	debridTorrent = dbT

	if debridTorrent == nil {
		m.logger.Error().Str("name", entry.Name).Msg("Provider entry not found")
		entry.MarkAsError(fmt.Errorf("debrid entry not found"))
		_ = m.queue.Update(entry)
		return
	}

	if debridTorrent.Status == debridTypes.TorrentStatusError {
		m.logger.Error().
			Str("debrid", debridTorrent.Debrid).
			Str("name", debridTorrent.Name).
			Str("status", string(debridTorrent.Status)).
			Msg("Entry in error state")
		entry.MarkAsError(fmt.Errorf("entry in error state on debrid: %s", debridTorrent.Debrid))
		_ = m.queue.Update(entry)
		return
	}

	// Update entry progress
	entry.DebridProgress = debridTorrent.Progress / 100.0
	entry.Progress = entry.DebridProgress
	entry.Speed = debridTorrent.Speed
	entry.Size = debridTorrent.GetSize()
	entry.Seeders = debridTorrent.Seeders
	entry.UpdatedAt = time.Now()
	if entry.Phase == "" || entry.Phase == storage.DownloadPhaseQueued {
		entry.Phase = storage.DownloadPhaseDebridFetching
	}

	// Update placement progress
	if placement := entry.GetActiveProvider(); placement != nil {
		touchProviderProgress(placement, entry.DebridProgress)
	}

	_ = m.queue.Update(entry)

	debridComplete := debridTorrent.Status == debridTypes.TorrentStatusDownloaded ||
		(entry.DebridProgress >= 1.0 && len(debridTorrent.Files) > 0 && debridTorrent.Status != debridTypes.TorrentStatusError)

	if debridComplete {
		backfillEntryFromDebrid(entry, debridTorrent)
		_ = m.queue.Update(entry)
		m.processAction(entry)
	}
}

func (m *Manager) processAction(entry *storage.Entry) {
	entry.Status = debridTypes.TorrentStatusDownloaded
	entry.Phase = storage.DownloadPhaseDownloading
	entry.UpdatedAt = time.Now()
	entry.AppendEvent(storage.TimelineDebridReady, entry.ActiveProvider, "")
	_ = m.queue.Update(entry)
	m.logger.Info().
		Str("name", entry.Name).
		Str("action", string(entry.Action)).
		Msg("Download completed, processing action")

	m.notifyDebridReady(entry)

	// Merge with existing entry if same infohash already exists (e.g., same
	// torrent on a different provider). The queue entry only knows about the
	// provider it was queued for, so we need to preserve other placements.
	if existing, err := m.storage.Get(entry.InfoHash); err == nil && existing != nil {
		entry = storage.HandleExistingEntryMerge(existing, entry)
	}

	// Now add entry to the main storage
	if err := m.AddOrUpdate(entry, func(t *storage.Entry) {
		m.RefreshEntries(true)
	}); err != nil {
		return
	}
	err := m.downloader.download(entry)
	if err != nil {
		m.logger.Error().
			Err(err).
			Str("name", entry.Name).
			Msg("Error running post-download action")
		m.downloader.markAsError(entry, err)
		return
	}
}

func (m *Manager) notifyDebridReady(entry *storage.Entry) {
	msg := fmt.Sprintf("Debrid ready, starting local pull: %s [%s]", entry.Name, entry.Category)
	m.Notifications.Notify(notifications.Event{
		Type:    config.EventDebridReady,
		Status:  "info",
		Entry:   entry,
		Message: msg,
	})
}

// processTorrent handles the complete torrent lifecycle
func (m *Manager) processNewTorrent(torrent *storage.Entry, debridTorrent *debridTypes.Torrent) {
	// Update status to submitting
	torrent.UpdatedAt = time.Now()
	_ = m.queue.Update(torrent)

	backfillEntryFromDebrid(torrent, debridTorrent)
	torrent.Phase = storage.DownloadPhaseDebridFetching
	torrent.DebridProgress = debridTorrent.Progress / 100.0
	torrent.Progress = torrent.DebridProgress
	_ = m.queue.Update(torrent)

	if debridTorrent.Status != debridTypes.TorrentStatusDownloaded {
		m.logger.Info().
			Str("debrid", debridTorrent.Debrid).
			Str("name", debridTorrent.Name).
			Msg("Started downloading torrent")
		return
	}

	m.processAction(torrent)
}

// SendToDebrid submits a magnet to debrid service(s) - replaces debrid.Parse
func (m *Manager) SendToDebrid(ctx context.Context, importRequest *ImportRequest) (*debridTypes.Torrent, error) {
	debridTorrent := &debridTypes.Torrent{
		InfoHash: importRequest.Magnet.InfoHash,
		Magnet:   importRequest.Magnet,
		Name:     importRequest.Magnet.Name,
		Arr:      importRequest.Arr,
		Size:     importRequest.Magnet.Size,
		Files:    make(map[string]debridTypes.File),
	}

	clients := m.orderedTorrentDebridClients(importRequest.SelectedDebrid)

	if len(clients) == 0 {
		return nil, fmt.Errorf("no debrid clients available")
	}

	eligible := m.filterClientsBySlots(clients)
	if len(eligible) == 0 {
		// All eligible providers are slot-exhausted — surface as retryable so the queue can backoff.
		return nil, customerror.TooManyActiveDownloadsError
	}
	clients = eligible

	if config.Get().PreferCached() {
		if cached, ok := m.selectCachedProvider(ctx, importRequest.Magnet.InfoHash, clients); ok {
			// Promote the cached provider to be tried first, but keep the
			// remaining providers as fallback so a provider-specific failure
			// (e.g. 451 DMCA block) still falls through to others.
			reordered := make([]common.Client, 0, len(clients))
			reordered = append(reordered, cached)
			for _, c := range clients {
				if c.Config().Name != cached.Config().Name {
					reordered = append(reordered, c)
				}
			}
			clients = reordered
		}
	}

	errs := make([]error, 0, len(clients))

	for _, db := range clients {
		overrideDownloadUncached := false

		if importRequest.DownloadUncached != nil {
			overrideDownloadUncached = *importRequest.DownloadUncached
		} else {
			overrideDownloadUncached = db.Config().DownloadUncached
		}
		debridTorrent.DownloadUncached = overrideDownloadUncached
		_logger := db.Logger()
		_logger.Info().
			Str("Provider", db.Config().Name).
			Str("Arr", importRequest.Arr.Name).
			Str("Hash", debridTorrent.InfoHash).
			Str("Name", debridTorrent.Name).
			Str("Action", string(importRequest.Action)).
			Msg("Processing torrent")

		dbt, err := db.SubmitMagnet(debridTorrent)
		if err != nil || dbt == nil || dbt.Id == "" {
			if err != nil {
				_logger.Warn().
					Err(err).
					Str("provider", db.Config().Name).
					Str("hash", debridTorrent.InfoHash).
					Msg("Submit failed; trying next provider if available")
				attempt := SubmitAttempt{Provider: db.Config().Name, Message: err.Error()}
				var customErr *customerror.Error
				if errors.As(err, &customErr) {
					attempt.Code = customErr.Code
				}
				importRequest.SubmitAttempts = append(importRequest.SubmitAttempts, attempt)
			}
			errs = append(errs, err)
			continue
		}
		dbt.Arr = importRequest.Arr
		_logger.Info().Str("id", dbt.Id).Msgf("Entry: %s submitted to %s", dbt.Name, db.Config().Name)

		torrent, err := db.CheckStatus(dbt)
		if err != nil && torrent != nil && torrent.Id != "" {
			// Delete the torrent if it was not downloaded
			go func(id string) {
				_ = db.DeleteTorrent(id)
			}(torrent.Id)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if torrent == nil {
			errs = append(errs, fmt.Errorf("torrent %s returned nil after checking status", dbt.Name))
			continue
		}
		return torrent, nil
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("failed to process torrent: no clients available")
	}
	joinedErrors := errors.Join(errs...)
	return nil, fmt.Errorf("failed to process torrent: %w", joinedErrors)
}

// submitAttemptEvent maps a recorded SubmitAttempt to a timeline kind and
// human-readable message. Provider-blocked submissions (HTTP 451 / DMCA) get a
// dedicated kind so the UI can render them distinctly from generic errors.
func submitAttemptEvent(a SubmitAttempt) (storage.TimelineEventKind, string) {
	switch a.Code {
	case "content_blocked":
		return storage.TimelineProviderBlocked, "Blocked by provider (DMCA / 451) — falling back"
	default:
		msg := a.Message
		if msg == "" {
			msg = "Submit failed — falling back"
		} else {
			msg = "Submit failed — falling back: " + msg
		}
		return storage.TimelineProviderSkipped, msg
	}
}

func (m *Manager) processNewNZBDebrid(entry *storage.Entry, usenetDownload *debridTypes.UsenetDownload) {
	entry.UpdatedAt = time.Now()
	_ = m.queue.Update(entry)

	backfillEntryFromDebrid(entry, usenetDownload.AsTorrent())
	entry.Phase = storage.DownloadPhaseDebridFetching
	entry.DebridProgress = usenetDownload.Progress / 100.0
	entry.Progress = entry.DebridProgress
	_ = m.queue.Update(entry)

	if usenetDownload.Status != debridTypes.TorrentStatusDownloaded {
		m.logger.Info().
			Str("debrid", usenetDownload.Debrid).
			Str("name", usenetDownload.Name).
			Msg("Started downloading NZB via debrid")
		return
	}

	m.processAction(entry)
}

func (m *Manager) processQueuedNZBDebrid(entry *storage.Entry) {
	placement := entry.GetActiveProvider()
	if placement == nil {
		m.logger.Error().Str("name", entry.Name).Msg("No active placement found for queued NZB")
		entry.MarkAsError(fmt.Errorf("no active placement found"))
		_ = m.queue.Update(entry)
		return
	}

	nzbClient, err := m.NZBProviderClient(entry.ActiveProvider)
	if err != nil {
		m.logger.Error().Err(err).Str("debrid", entry.ActiveProvider).Msg("NZB provider client not found")
		entry.MarkAsError(err)
		_ = m.queue.Update(entry)
		return
	}

	arr := m.arr.GetOrCreate(entry.Category)
	usenetDownload := &debridTypes.UsenetDownload{
		Id:               placement.ID,
		Hash:             entry.InfoHash,
		Name:             entry.Name,
		Arr:              arr,
		Size:             entry.Size,
		Files:            make(map[string]debridTypes.File),
		DownloadUncached: entry.DownloadUncached,
	}

	updated, err := nzbClient.CheckNZBStatus(context.Background(), usenetDownload)
	if err != nil {
		m.logger.Error().Err(err).Str("name", entry.Name).Msg("Error checking NZB debrid status")
		entry.MarkAsError(err)
		_ = m.queue.Update(entry)
		if updated != nil && updated.Id != "" {
			go func(id string) {
				_ = nzbClient.DeleteNZB(id)
			}(updated.Id)
		}
		return
	}
	usenetDownload = updated

	entry.DebridProgress = usenetDownload.Progress / 100.0
	entry.Progress = entry.DebridProgress
	entry.Speed = usenetDownload.Speed
	entry.Size = usenetDownload.GetSize()
	entry.UpdatedAt = time.Now()
	if entry.Phase == "" || entry.Phase == storage.DownloadPhaseQueued {
		entry.Phase = storage.DownloadPhaseDebridFetching
	}
	if placement := entry.GetActiveProvider(); placement != nil {
		touchProviderProgress(placement, entry.DebridProgress)
	}
	_ = m.queue.Update(entry)

	debridComplete := usenetDownload.Status == debridTypes.TorrentStatusDownloaded ||
		(entry.DebridProgress >= 1.0 && len(usenetDownload.Files) > 0 && usenetDownload.Status != debridTypes.TorrentStatusError)

	if debridComplete {
		backfillEntryFromDebrid(entry, usenetDownload.AsTorrent())
		_ = m.queue.Update(entry)
		m.processAction(entry)
	}
}

func nzbContentHash(content []byte) string {
	sum := md5.Sum(content)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// SendToNZBDebrid submits an NZB to a debrid provider that supports usenet downloads.
func (m *Manager) SendToNZBDebrid(ctx context.Context, importRequest *ImportRequest) (*debridTypes.UsenetDownload, error) {
	hash := nzbContentHash(importRequest.NZBContent)
	usenetDownload := &debridTypes.UsenetDownload{
		Hash:       hash,
		Name:       importRequest.Name,
		Filename:   filepath.Base(importRequest.Name),
		Arr:        importRequest.Arr,
		NZBContent: importRequest.NZBContent,
		Files:      make(map[string]debridTypes.File),
	}

	clients := m.orderedNamedNZBClients(importRequest.SelectedDebrid)
	if len(clients) == 0 {
		return nil, fmt.Errorf("no NZB-capable debrid clients available")
	}

	eligible := m.filterNZBClientsBySlots(clients)
	if len(eligible) == 0 {
		return nil, customerror.TooManyActiveDownloadsError
	}
	clients = eligible

	if config.Get().PreferCached() {
		if cached, ok := m.selectCachedNZBProvider(ctx, hash, importRequest.SelectedDebrid); ok {
			// Promote the cached provider to be tried first while keeping
			// remaining providers as fallback (mirrors SendToDebrid behaviour
			// for symmetric provider-specific failure handling).
			reordered := make([]namedNZBClient, 0, len(clients))
			reordered = append(reordered, cached)
			for _, c := range clients {
				if c.name != cached.name {
					reordered = append(reordered, c)
				}
			}
			clients = reordered
		}
	}

	errs := make([]error, 0, len(clients))
	for _, item := range clients {
		client := item.client
		overrideDownloadUncached := false
		if importRequest.DownloadUncached != nil {
			overrideDownloadUncached = *importRequest.DownloadUncached
		} else if importRequest.Arr != nil && importRequest.Arr.DownloadUncached != nil {
			overrideDownloadUncached = *importRequest.Arr.DownloadUncached
		} else if dc := m.debridConfigByName(item.name); dc != nil {
			overrideDownloadUncached = dc.DownloadUncached
		}
		usenetDownload.DownloadUncached = overrideDownloadUncached

		dl := *usenetDownload
		submitted, err := client.SubmitNZB(ctx, &dl)
		if err != nil || submitted == nil || submitted.Id == "" {
			if err != nil {
				errs = append(errs, err)
				attempt := SubmitAttempt{Provider: item.name, Message: err.Error()}
				var customErr *customerror.Error
				if errors.As(err, &customErr) {
					attempt.Code = customErr.Code
				}
				importRequest.SubmitAttempts = append(importRequest.SubmitAttempts, attempt)
			} else {
				errs = append(errs, fmt.Errorf("failed to submit nzb to %s", item.name))
			}
			continue
		}
		submitted.Debrid = item.name
		submitted.Arr = importRequest.Arr
		submitted.DownloadUncached = overrideDownloadUncached

		updated, err := client.CheckNZBStatus(ctx, submitted)
		if err != nil && updated != nil && updated.Id != "" {
			go func(id string, c common.NZBClient) {
				_ = c.DeleteNZB(id)
			}(updated.Id, client)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if updated == nil {
			errs = append(errs, fmt.Errorf("nzb %s returned nil after checking status", submitted.Name))
			continue
		}
		updated.Debrid = item.name
		return updated, nil
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("failed to process nzb: no clients available")
	}
	return nil, fmt.Errorf("failed to process nzb: %w", errors.Join(errs...))
}

type namedNZBClient struct {
	name   string
	client common.NZBClient
}

func (m *Manager) orderedNamedNZBClients(selectedDebrid string) []namedNZBClient {
	cfg := config.Get()
	debrids := applyDebridOrder(cfg.Debrids, cfg.NZBDebridOrder)
	out := make([]namedNZBClient, 0, len(debrids))
	for _, dc := range debrids {
		if selectedDebrid != "" && dc.Name != selectedDebrid {
			continue
		}
		if !dc.AllowsNZBs() {
			continue
		}
		client, err := m.NZBProviderClient(dc.Name)
		if err == nil && client != nil {
			out = append(out, namedNZBClient{name: dc.Name, client: client})
		}
	}
	return out
}

func (m *Manager) debridConfigByName(name string) *config.Debrid {
	for _, dc := range config.Get().Debrids {
		if dc.Name == name {
			copy := dc
			return &copy
		}
	}
	return nil
}

func (m *Manager) selectCachedNZBProvider(ctx context.Context, hash string, selectedDebrid string) (namedNZBClient, bool) {
	clients := m.orderedNamedNZBClients(selectedDebrid)
	if len(clients) == 0 || hash == "" {
		return namedNZBClient{}, false
	}
	hash = strings.ToUpper(hash)

	ctx, cancel := context.WithTimeout(ctx, cacheCheckTimeout)
	defer cancel()

	type probeResult struct {
		item   namedNZBClient
		cached bool
	}
	results := make([]probeResult, len(clients))
	var wg sync.WaitGroup
	wg.Add(len(clients))
	for i, item := range clients {
		i, item := i, item
		go func() {
			defer wg.Done()
			results[i].item = item
			avail := item.client.IsNZBAvailable([]string{hash})
			if avail != nil && avail[hash] {
				results[i].cached = true
			}
		}()
	}
	wg.Wait()

	for _, item := range clients {
		for _, r := range results {
			if r.cached && r.item.name == item.name {
				m.logger.Info().
					Str("provider", item.name).
					Str("hash", hash).
					Msg("Pre-flight cache check: NZB cached on provider")
				return item, true
			}
		}
	}
	return namedNZBClient{}, false
}
