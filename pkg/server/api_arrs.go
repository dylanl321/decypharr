package server

import (
	"net/http"

	json "github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/arr"
)

// handleAddArr adds a new arr connection
func (s *Server) handleAddArr(w http.ResponseWriter, r *http.Request) {
	var req config.Arr
	if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.Host == "" {
		http.Error(w, "Host is required", http.StatusBadRequest)
		return
	}
	if req.Token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	// Check if arr already exists
	if s.manager.Arr().Get(req.Name) != nil {
		http.Error(w, "Arr with this name already exists", http.StatusConflict)
		return
	}

	// Create new arr
	newArr := arr.New(req.Name, req.Host, req.Token, req.Cleanup, req.SkipRepair, req.DownloadUncached, req.SelectedDebrid, req.Source)

	// Validate the arr connection
	if err := newArr.Validate(); err != nil {
		http.Error(w, "Failed to validate arr connection: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Add to storage
	s.manager.Arr().AddOrUpdate(newArr)

	// Sync to config and save
	cfg := config.Get()
	cfg.Arrs = s.manager.Arr().SyncToConfig()
	if err := cfg.Save(); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save config")
		http.Error(w, "Failed to save config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	utils.JSONResponse(w, newArr, http.StatusCreated)
}

// handleUpdateArr updates an existing arr
func (s *Server) handleUpdateArr(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "No arr name provided", http.StatusBadRequest)
		return
	}

	var req config.Arr
	if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Check if arr exists
	existingArr := s.manager.Arr().Get(name)
	if existingArr == nil {
		http.Error(w, "Arr not found", http.StatusNotFound)
		return
	}

	// Update arr fields (keep name the same)
	if req.Host != "" {
		existingArr.Host = req.Host
	}
	if req.Token != "" {
		existingArr.Token = req.Token
	}
	existingArr.Cleanup = req.Cleanup
	existingArr.SkipRepair = req.SkipRepair
	existingArr.DownloadUncached = req.DownloadUncached
	if req.SelectedDebrid != "" {
		existingArr.SelectedDebrid = req.SelectedDebrid
	}
	if req.Source != "" {
		existingArr.Source = arr.Source(req.Source)
	}

	// Validate the updated arr
	if err := existingArr.Validate(); err != nil {
		http.Error(w, "Failed to validate arr connection: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Update in storage
	s.manager.Arr().AddOrUpdate(existingArr)

	// Sync to config and save
	cfg := config.Get()
	cfg.Arrs = s.manager.Arr().SyncToConfig()
	if err := cfg.Save(); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save config")
		http.Error(w, "Failed to save config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	utils.JSONResponse(w, existingArr, http.StatusOK)
}

// handleDeleteArr removes an arr
func (s *Server) handleDeleteArr(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "No arr name provided", http.StatusBadRequest)
		return
	}

	// Check if arr exists
	if s.manager.Arr().Get(name) == nil {
		http.Error(w, "Arr not found", http.StatusNotFound)
		return
	}

	// Remove from storage by syncing a filtered config
	cfg := config.Get()
	filteredArrs := make([]config.Arr, 0)
	for _, a := range cfg.Arrs {
		if a.Name != name {
			filteredArrs = append(filteredArrs, a)
		}
	}
	cfg.Arrs = filteredArrs

	// Sync from config (this will remove the arr from memory)
	s.manager.Arr().SyncFromConfig(cfg.Arrs)

	// Save config
	if err := cfg.Save(); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save config")
		http.Error(w, "Failed to save config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleTestArr tests connectivity to an arr
func (s *Server) handleTestArr(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "No arr name provided", http.StatusBadRequest)
		return
	}

	// Get the arr
	arrInstance := s.manager.Arr().Get(name)
	if arrInstance == nil {
		http.Error(w, "Arr not found", http.StatusNotFound)
		return
	}

	// Test connectivity
	if err := arrInstance.Validate(); err != nil {
		utils.JSONResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, http.StatusOK)
		return
	}

	utils.JSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Connection successful",
	}, http.StatusOK)
}
