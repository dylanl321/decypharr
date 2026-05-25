package server

import (
	"net/http"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
)

type debridRateLimitInfo struct {
	Main     string `json:"main"`
	Repair   string `json:"repair"`
	Download string `json:"download"`
	Provider string `json:"provider"`
}

func (s *Server) handleDebridRateLimits(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	out := make(map[string]debridRateLimitInfo)
	if cfg == nil {
		utils.JSONResponse(w, out, http.StatusOK)
		return
	}
	for _, dc := range cfg.Debrids {
		out[dc.Name] = debridRateLimitInfo{
			Main:     dc.RateLimit,
			Repair:   dc.RepairRateLimit,
			Download: dc.DownloadRateLimit,
			Provider: dc.Provider,
		}
	}
	utils.JSONResponse(w, out, http.StatusOK)
}
