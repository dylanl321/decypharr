package server

import (
	"net/http"
	"strconv"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/version"
)

// handleGetMountStatus returns mount status and info
func (s *Server) handleGetMountStatus(w http.ResponseWriter, r *http.Request) {
	mountMgr := s.manager.MountManager()
	if mountMgr == nil {
		utils.JSONResponse(w, map[string]interface{}{
			"enabled": false,
			"ready":   false,
			"type":    "none",
		}, http.StatusOK)
		return
	}

	stats := mountMgr.Stats()
	utils.JSONResponse(w, map[string]interface{}{
		"enabled": true,
		"ready":   mountMgr.IsReady(),
		"type":    mountMgr.Type(),
		"stats":   stats,
	}, http.StatusOK)
}

// handleRefreshMount triggers a mount refresh/rescan
func (s *Server) handleRefreshMount(w http.ResponseWriter, r *http.Request) {
	mountMgr := s.manager.MountManager()
	if mountMgr == nil {
		http.Error(w, "Mount manager not configured", http.StatusServiceUnavailable)
		return
	}

	// Get dirs from query param or use default
	dirsParam := r.URL.Query().Get("dirs")
	var dirs []string
	if dirsParam != "" {
		dirs = strings.Split(dirsParam, ",")
	} else {
		dirs = []string{"__all__"}
	}

	if err := mountMgr.Refresh(dirs); err != nil {
		s.logger.Error().Err(err).Msg("Failed to refresh mount")
		http.Error(w, "Failed to refresh mount: "+err.Error(), http.StatusInternalServerError)
		return
	}

	utils.JSONResponse(w, map[string]string{"status": "refreshed"}, http.StatusOK)
}

// handleGetNotificationConfig returns notification configuration
func (s *Server) handleGetNotificationConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	utils.JSONResponse(w, cfg.Notifications, http.StatusOK)
}

// handleUpdateNotificationConfig updates notification settings
func (s *Server) handleUpdateNotificationConfig(w http.ResponseWriter, r *http.Request) {
	var req config.Notifications
	if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	cfg := config.Get()
	cfg.Notifications = req

	if err := cfg.Save(); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save config")
		http.Error(w, "Failed to save config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reinitialize notifications service if needed
	if s.manager.Notifications != nil {
		// The notifications service should pick up new config automatically
		s.logger.Info().Msg("Notification configuration updated")
	}

	utils.JSONResponse(w, cfg.Notifications, http.StatusOK)
}

// handleGetLogs returns recent log entries
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	level := strings.TrimSpace(r.URL.Query().Get("level"))

	limit := 100
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Cap limit at 1000
	if limit > 1000 {
		limit = 1000
	}

	// Get logs from buffer
	var logs []LogEntry
	if s.logBuffer != nil {
		if level != "" {
			logs = s.logBuffer.FilterByLevel(level, limit)
		} else {
			logs = s.logBuffer.GetRecent(limit)
		}
	} else {
		logs = []LogEntry{}
	}

	utils.JSONResponse(w, map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
		"limit": limit,
	}, http.StatusOK)
}

// handleRestart triggers a graceful restart
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	utils.JSONResponse(w, map[string]string{"status": "restarting"}, http.StatusOK)

	// Trigger restart asynchronously
	go s.Restart()
}

// handleGetAPIVersion returns version info under /api/version
func (s *Server) handleGetAPIVersion(w http.ResponseWriter, r *http.Request) {
	v := version.GetInfo()
	utils.JSONResponse(w, v, http.StatusOK)
}

// handleGetDebridProviders lists configured debrid providers
func (s *Server) handleGetDebridProviders(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	providers := make([]map[string]interface{}, 0, len(cfg.Debrids))

	for _, d := range cfg.Debrids {
		providers = append(providers, map[string]interface{}{
			"name":    d.Name,
			"type":    d.Provider,
			"enabled": d.APIKey != "",
		})
	}

	utils.JSONResponse(w, providers, http.StatusOK)
}

// handleTestDebridProvider tests connectivity to a specific debrid provider
func (s *Server) handleTestDebridProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "No provider name provided", http.StatusBadRequest)
		return
	}

	// Get the debrid client
	client := s.manager.ProviderClient(name)
	if client == nil {
		http.Error(w, "Debrid provider not found", http.StatusNotFound)
		return
	}

	// Test by getting profile
	profile, err := client.GetProfile()
	if err != nil {
		utils.JSONResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, http.StatusOK)
		return
	}

	utils.JSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Connection successful",
		"profile": profile,
	}, http.StatusOK)
}
