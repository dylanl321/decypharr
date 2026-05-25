package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirrobot01/decypharr/internal/utils"
	debrid "github.com/sirrobot01/decypharr/pkg/debrid/common"
	"github.com/sirrobot01/decypharr/pkg/version"
)

// HealthCheck holds the status of an individual component
type HealthCheck struct {
	Status    string  `json:"status"` // "up", "down", "degraded"
	LatencyMs *int64  `json:"latency_ms,omitempty"`
	Error     string  `json:"error,omitempty"`
	Detail    any     `json:"detail,omitempty"`
}

// HealthResponse is the response for the health endpoint
type HealthResponse struct {
	Status        string                    `json:"status"` // "healthy", "degraded", "unhealthy"
	UptimeSeconds int64                     `json:"uptime_seconds"`
	Version       string                    `json:"version"`
	Checks        map[string]any            `json:"checks"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	checks := make(map[string]any)

	// Check qBit API
	checks["qbit_api"] = s.checkQbitAPI()

	// Check WebDAV
	checks["webdav"] = s.checkWebDAV()

	// Check storage
	checks["storage"] = s.checkStorage()

	// Check mount
	checks["mount"] = s.checkMount()

	// Check debrid providers
	checks["debrid_providers"] = s.checkDebridProviders(ctx)

	// Check arr connections
	checks["arr_connections"] = s.checkArrConnections(ctx)

	// Determine overall status
	overallStatus := s.determineOverallStatus(checks)

	response := HealthResponse{
		Status:        overallStatus,
		UptimeSeconds: int64(s.manager.Uptime().Seconds()),
		Version:       version.GetInfo().String(),
		Checks:        checks,
	}

	statusCode := http.StatusOK
	if overallStatus == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	utils.JSONResponse(w, response, statusCode)
}

func (s *Server) checkQbitAPI() HealthCheck {
	start := time.Now()
	// Simple check - if we can access the manager, qBit API is up
	if s.manager == nil {
		return HealthCheck{Status: "down", Error: "manager not initialized"}
	}

	latency := time.Since(start).Milliseconds()
	return HealthCheck{Status: "up", LatencyMs: &latency}
}

func (s *Server) checkWebDAV() HealthCheck {
	start := time.Now()
	// WebDAV is always available in the server
	latency := time.Since(start).Milliseconds()
	return HealthCheck{Status: "up", LatencyMs: &latency}
}

func (s *Server) checkStorage() HealthCheck {
	start := time.Now()
	storage := s.manager.Storage()
	if storage == nil {
		return HealthCheck{Status: "down", Error: "storage not initialized"}
	}

	// Try a simple operation
	_, err := storage.Count()
	if err != nil {
		return HealthCheck{Status: "down", Error: fmt.Sprintf("storage error: %v", err)}
	}

	latency := time.Since(start).Milliseconds()
	return HealthCheck{Status: "up", LatencyMs: &latency}
}

func (s *Server) checkMount() HealthCheck {
	mountMgr := s.manager.MountManager()
	if mountMgr == nil {
		return HealthCheck{Status: "down", Error: "mount manager not configured"}
	}

	if !mountMgr.IsReady() {
		return HealthCheck{Status: "down", Error: "mount not ready"}
	}

	return HealthCheck{
		Status: "up",
		Detail: map[string]string{
			"type": mountMgr.Type(),
		},
	}
}

func (s *Server) checkDebridProviders(ctx context.Context) []map[string]any {
	var providers []map[string]any

	s.manager.Clients().Range(func(name string, client debrid.Client) bool {
		start := time.Now()
		check := map[string]any{
			"name": name,
		}

		// Try to get profile to verify connectivity
		profile, err := client.GetProfile()
		latency := time.Since(start).Milliseconds()
		check["latency_ms"] = latency

		if err != nil {
			check["status"] = "down"
			check["error"] = err.Error()
		} else {
			check["status"] = "up"
			if profile != nil {
				if !profile.Expiration.IsZero() {
					check["account_expiry"] = profile.Expiration.Format(time.RFC3339)
				}
				check["username"] = profile.Username
				check["premium"] = profile.Premium > 0
			}
		}

		providers = append(providers, check)
		return true
	})

	return providers
}

func (s *Server) checkArrConnections(ctx context.Context) []map[string]any {
	var connections []map[string]any

	arrs := s.manager.Arr().GetAll()
	for _, arr := range arrs {
		start := time.Now()
		check := map[string]any{
			"name": arr.Name,
		}

		// Try to validate the arr connection
		err := arr.Validate()
		latency := time.Since(start).Milliseconds()
		check["latency_ms"] = latency

		if err != nil {
			check["status"] = "down"
			check["error"] = err.Error()
		} else {
			check["status"] = "up"
		}

		connections = append(connections, check)
	}

	return connections
}

func (s *Server) determineOverallStatus(checks map[string]any) string {
	// Critical services: storage, mount, debrid_providers
	// Non-critical: arr_connections, webdav, qbit_api

	criticalDown := false
	nonCriticalDown := false

	// Check storage
	if storage, ok := checks["storage"].(HealthCheck); ok && storage.Status != "up" {
		criticalDown = true
	}

	// Check mount
	if mount, ok := checks["mount"].(HealthCheck); ok && mount.Status != "up" {
		criticalDown = true
	}

	// Check debrid providers - at least one must be up
	if providers, ok := checks["debrid_providers"].([]map[string]any); ok {
		allDown := true
		for _, p := range providers {
			if status, ok := p["status"].(string); ok && status == "up" {
				allDown = false
				break
			}
		}
		if allDown && len(providers) > 0 {
			criticalDown = true
		}
	}

	// Check non-critical services
	if qbit, ok := checks["qbit_api"].(HealthCheck); ok && qbit.Status != "up" {
		nonCriticalDown = true
	}

	if webdav, ok := checks["webdav"].(HealthCheck); ok && webdav.Status != "up" {
		nonCriticalDown = true
	}

	if arrs, ok := checks["arr_connections"].([]map[string]any); ok {
		for _, a := range arrs {
			if status, ok := a["status"].(string); ok && status != "up" {
				nonCriticalDown = true
				break
			}
		}
	}

	if criticalDown {
		return "unhealthy"
	}
	if nonCriticalDown {
		return "degraded"
	}
	return "healthy"
}
