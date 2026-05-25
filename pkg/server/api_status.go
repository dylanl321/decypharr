package server

import (
	"net/http"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	debrid "github.com/sirrobot01/decypharr/pkg/debrid/common"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// DebridProviderStatus represents the status of a single debrid provider
type DebridProviderStatus struct {
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Status    string     `json:"status"` // "active", "inactive", "error"
	Premium   bool       `json:"premium"`
	Expiry    *time.Time `json:"expiry,omitempty"`
	Username  string     `json:"username,omitempty"`
	Email     string     `json:"email,omitempty"`
	Points    *int       `json:"points,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// DebridStatusResponse is the response for the debrid status endpoint
type DebridStatusResponse struct {
	Providers []DebridProviderStatus `json:"providers"`
}

func (s *Server) handleDebridStatus(w http.ResponseWriter, r *http.Request) {
	var providers []DebridProviderStatus

	s.manager.Clients().Range(func(name string, client debrid.Client) bool {
		status := DebridProviderStatus{
			Name: name,
			Type: string(client.Config().Provider),
		}

		profile, err := client.GetProfile()
		if err != nil {
			status.Status = "error"
			status.Error = err.Error()
		} else {
			status.Status = "active"
			if profile != nil {
				status.Premium = profile.Premium > 0
				status.Username = profile.Username
				status.Email = profile.Email
				if profile.Points > 0 {
					status.Points = &profile.Points
				}
				if !profile.Expiration.IsZero() {
					status.Expiry = &profile.Expiration
				}
			}
		}

		providers = append(providers, status)
		return true
	})

	response := DebridStatusResponse{
		Providers: providers,
	}

	utils.JSONResponse(w, response, http.StatusOK)
}

// QueueSummaryResponse is the response for the queue summary endpoint
type QueueSummaryResponse struct {
	Total      int                      `json:"total"`
	ByState    map[string]int           `json:"by_state"`
	ByCategory map[string]int           `json:"by_category"`
	ByProtocol map[string]int           `json:"by_protocol"`
	Errors     []QueueErrorItem         `json:"errors"`
}

type QueueErrorItem struct {
	Hash    string    `json:"hash"`
	Name    string    `json:"name"`
	Error   string    `json:"error"`
	AddedOn time.Time `json:"added_on"`
}

func (s *Server) handleQueueSummary(w http.ResponseWriter, r *http.Request) {
	// Get all torrents from queue
	allTorrents := s.manager.Queue().ListFilter("", config.ProtocolAll, "", nil, "added_on", false)

	response := QueueSummaryResponse{
		Total:      len(allTorrents),
		ByState:    make(map[string]int),
		ByCategory: make(map[string]int),
		ByProtocol: make(map[string]int),
		Errors:     make([]QueueErrorItem, 0),
	}

	for _, t := range allTorrents {
		// Count by state
		response.ByState[string(t.State)]++

		// Count by category
		if t.Category != "" {
			response.ByCategory[t.Category]++
		} else {
			response.ByCategory["uncategorized"]++
		}

		// Count by protocol
		response.ByProtocol[string(t.Protocol)]++

		// Collect errors
		if t.State == storage.EntryStateError && t.LastError != "" {
			response.Errors = append(response.Errors, QueueErrorItem{
				Hash:    t.InfoHash,
				Name:    t.Name,
				Error:   t.LastError,
				AddedOn: t.AddedOn,
			})
		}
	}

	utils.JSONResponse(w, response, http.StatusOK)
}
