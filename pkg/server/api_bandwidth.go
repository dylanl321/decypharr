package server

import (
	"encoding/json"
	"net/http"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
)

// BandwidthSnapshot is the JSON shape returned by /api/bandwidth. Values are
// reported in bytes per second; the UI is responsible for human-friendly
// formatting (KB/s, MB/s, ...).
type BandwidthSnapshot struct {
	Global      int64            `json:"global_bytes_per_sec"`
	GlobalRaw   string           `json:"global_raw,omitempty"` // original config string
	PerProvider map[string]int64 `json:"per_provider_bytes_per_sec"`
	ProviderRaw map[string]string `json:"provider_raw,omitempty"`
	TotalBytes  int64            `json:"total_bytes_shaped"`
}

func (s *Server) handleBandwidth(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	global, perProv, total := s.manager.BandwidthSnapshot()

	provRaw := map[string]string{}
	for _, d := range cfg.Debrids {
		if d.BandwidthLimit != "" {
			provRaw[d.Name] = d.BandwidthLimit
		}
	}

	utils.JSONResponse(w, BandwidthSnapshot{
		Global:      global,
		GlobalRaw:   cfg.Download.BandwidthLimit,
		PerProvider: perProv,
		ProviderRaw: provRaw,
		TotalBytes:  total,
	}, http.StatusOK)
}

// BandwidthUpdate is the request payload for PUT /api/bandwidth. All fields
// are optional; absent fields preserve the existing value. Send an empty
// string to clear an existing cap.
type BandwidthUpdate struct {
	Global      *string            `json:"global,omitempty"`
	PerProvider map[string]*string `json:"per_provider,omitempty"`
}

func (s *Server) handleUpdateBandwidth(w http.ResponseWriter, r *http.Request) {
	var req BandwidthUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Global != nil {
		if err := config.ValidateBandwidth("download.bandwidth_limit", *req.Global); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	for name, v := range req.PerProvider {
		if v == nil {
			continue
		}
		if err := config.ValidateBandwidth("debrids."+name+".bandwidth_limit", *v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	cfg := config.Get()
	if req.Global != nil {
		cfg.Download.BandwidthLimit = *req.Global
	}
	if len(req.PerProvider) > 0 {
		for i, d := range cfg.Debrids {
			if v, ok := req.PerProvider[d.Name]; ok && v != nil {
				cfg.Debrids[i].BandwidthLimit = *v
			}
		}
	}
	if err := cfg.Save(); err != nil {
		http.Error(w, "failed to persist config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.manager.ReloadBandwidth()
	s.handleBandwidth(w, r)
}
