package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/manager"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// handleGetQueueItem returns a single queue item by info hash
func (s *Server) handleGetQueueItem(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "No hash provided", http.StatusBadRequest)
		return
	}

	torrent, err := s.manager.Queue().GetTorrent(hash)
	if err != nil {
		http.Error(w, "Queue item not found", http.StatusNotFound)
		return
	}

	torrent.Sanitize()
	utils.JSONResponse(w, torrent, http.StatusOK)
}

// handleGetQueueTimeline returns the lifecycle event log for a queue entry.
// Falls back to a synthesized timeline if no events have been persisted yet
// (e.g. the entry predates timeline support) so the UI is never empty.
func (s *Server) handleGetQueueTimeline(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "No hash provided", http.StatusBadRequest)
		return
	}

	torrent, err := s.manager.Queue().GetTorrent(hash)
	if err != nil || torrent == nil {
		http.Error(w, "Queue item not found", http.StatusNotFound)
		return
	}

	events := torrent.Timeline
	if len(events) == 0 {
		events = synthesizeTimeline(torrent)
	}
	utils.JSONResponse(w, map[string]interface{}{
		"hash":     torrent.InfoHash,
		"name":     torrent.Name,
		"provider": torrent.ActiveProvider,
		"timeline": events,
	}, http.StatusOK)
}

// synthesizeTimeline reconstructs a coarse event log from existing timestamp
// fields on the entry. Used for entries created before the feature shipped.
func synthesizeTimeline(t *storage.Entry) []storage.TimelineEvent {
	out := make([]storage.TimelineEvent, 0, 4)
	if !t.AddedOn.IsZero() {
		out = append(out, storage.TimelineEvent{At: t.AddedOn, Kind: storage.TimelineAdded})
	}
	if t.DebridProgress >= 1.0 && !t.UpdatedAt.IsZero() {
		out = append(out, storage.TimelineEvent{
			At: t.UpdatedAt, Kind: storage.TimelineDebridReady, Provider: t.ActiveProvider,
		})
	}
	if t.CompletedAt != nil {
		out = append(out, storage.TimelineEvent{
			At: *t.CompletedAt, Kind: storage.TimelineLocalDownloadDone, Provider: t.ActiveProvider,
		})
	}
	if t.LastErrorTime != nil && t.LastError != "" {
		out = append(out, storage.TimelineEvent{
			At: *t.LastErrorTime, Kind: storage.TimelineError, Message: t.LastError,
		})
	}
	if t.ImportedAt != nil {
		out = append(out, storage.TimelineEvent{At: *t.ImportedAt, Kind: storage.TimelineImported})
	}
	return out
}

// handleRetryQueueItem retries/requeues a failed item
func (s *Server) handleRetryQueueItem(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "No hash provided", http.StatusBadRequest)
		return
	}

	torrent, err := s.manager.Queue().GetTorrent(hash)
	if err != nil {
		http.Error(w, "Queue item not found", http.StatusNotFound)
		return
	}

	// Convert storage.Entry back to ImportRequest for requeueing
	// We need the original magnet and arr info
	if torrent.InfoHash == "" {
		http.Error(w, "Cannot retry: missing info hash", http.StatusBadRequest)
		return
	}

	// Get the arr from storage
	arr := s.manager.Arr().Get(torrent.Category)
	if arr == nil {
		http.Error(w, "Cannot retry: arr not found", http.StatusBadRequest)
		return
	}

	// Reconstruct magnet from stored data
	magnet := &utils.Magnet{
		InfoHash: torrent.InfoHash,
		Name:     torrent.Name,
	}

	// Create a new import request
	importReq := &manager.ImportRequest{
		Magnet:         magnet,
		Arr:            arr,
		SelectedDebrid: torrent.ActiveProvider,
		DownloadFolder: config.Get().DownloadFolder,
		Action:         config.Get().DefaultDownloadAction,
	}

	if err := s.manager.Queue().ReQueue(importReq); err != nil {
		s.logger.Error().Err(err).Str("hash", hash).Msg("Failed to retry queue item")
		http.Error(w, "Failed to retry: "+err.Error(), http.StatusInternalServerError)
		return
	}

	utils.JSONResponse(w, map[string]string{"status": "queued"}, http.StatusOK)
}

// handlePauseQueueItem pauses a downloading item
func (s *Server) handlePauseQueueItem(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "No hash provided", http.StatusBadRequest)
		return
	}

	err := s.manager.Queue().UpdateWhere(
		func(t *storage.Entry) bool {
			return t.InfoHash == hash
		},
		func(t *storage.Entry) bool {
			if t.State == storage.EntryStateDownloading {
				t.State = storage.EntryStatePausedDL
				return true
			}
			return false
		},
	)

	if err != nil {
		s.logger.Error().Err(err).Str("hash", hash).Msg("Failed to pause queue item")
		http.Error(w, "Failed to pause", http.StatusInternalServerError)
		return
	}

	utils.JSONResponse(w, map[string]string{"status": "paused"}, http.StatusOK)
}

// handleResumeQueueItem resumes a paused item
func (s *Server) handleResumeQueueItem(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "No hash provided", http.StatusBadRequest)
		return
	}

	err := s.manager.Queue().UpdateWhere(
		func(t *storage.Entry) bool {
			return t.InfoHash == hash
		},
		func(t *storage.Entry) bool {
			if t.State == storage.EntryStatePausedDL || t.State == storage.EntryStatePausedUP {
				t.State = storage.EntryStateDownloading
				return true
			}
			return false
		},
	)

	if err != nil {
		s.logger.Error().Err(err).Str("hash", hash).Msg("Failed to resume queue item")
		http.Error(w, "Failed to resume", http.StatusInternalServerError)
		return
	}

	utils.JSONResponse(w, map[string]string{"status": "downloading"}, http.StatusOK)
}

// handleDeleteCompleted removes all completed items from queue
func (s *Server) handleDeleteCompleted(w http.ResponseWriter, r *http.Request) {
	err := s.manager.Queue().DeleteWhere(
		"",
		config.ProtocolAll,
		storage.EntryStatePausedUP,
		nil,
		nil,
	)

	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to delete completed items")
		http.Error(w, "Failed to delete completed items", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleDeleteErrors removes all errored items from queue
func (s *Server) handleDeleteErrors(w http.ResponseWriter, r *http.Request) {
	err := s.manager.Queue().DeleteWhere(
		"",
		config.ProtocolAll,
		storage.EntryStateError,
		nil,
		nil,
	)

	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to delete error items")
		http.Error(w, "Failed to delete error items", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleRetryAllErrors retries all items in error state
func (s *Server) handleRetryAllErrors(w http.ResponseWriter, r *http.Request) {
	// Get all errored items
	erroredItems := s.manager.Queue().ListFilter("", config.ProtocolAll, storage.EntryStateError, nil, "", false)

	var successCount, failCount int

	for _, torrent := range erroredItems {
		// Get the arr
		arr := s.manager.Arr().Get(torrent.Category)
		if arr == nil {
			failCount++
			continue
		}

		// Reconstruct magnet
		magnet := &utils.Magnet{
			InfoHash: torrent.InfoHash,
			Name:     torrent.Name,
		}

		// Create import request
		importReq := &manager.ImportRequest{
			Magnet:         magnet,
			Arr:            arr,
			SelectedDebrid: torrent.ActiveProvider,
			DownloadFolder: config.Get().DownloadFolder,
			Action:         config.Get().DefaultDownloadAction,
		}

		if err := s.manager.Queue().ReQueue(importReq); err != nil {
			s.logger.Error().Err(err).Str("hash", torrent.InfoHash).Msg("Failed to retry queue item")
			failCount++
		} else {
			successCount++
		}
	}

	utils.JSONResponse(w, map[string]interface{}{
		"retried": successCount,
		"failed":  failCount,
		"total":   len(erroredItems),
	}, http.StatusOK)
}
