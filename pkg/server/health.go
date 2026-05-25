package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

type healthResponse struct {
	Status string                 `json:"status"`
	Checks map[string]interface{} `json:"checks"`
}

func (s *Server) handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	checks := make(map[string]interface{})
	overall := "healthy"
	degraded := false
	unhealthy := false

	debridChecks := s.checkDebrids(ctx)
	checks["debrids"] = debridChecks
	for _, st := range debridChecks {
		if st == "error" || st == "timeout" {
			degraded = true
		}
	}
	if len(debridChecks) > 0 {
		allFailed := true
		for _, st := range debridChecks {
			if st == "ok" {
				allFailed = false
				break
			}
		}
		if allFailed {
			unhealthy = true
		}
	}

	arrChecks := s.checkArrs(ctx)
	checks["arrs"] = arrChecks
	for _, st := range arrChecks {
		if st != "ok" && st != "not_found" {
			degraded = true
		}
	}

	diskChecks := s.checkDiskWritable()
	checks["disk"] = diskChecks
	for _, st := range diskChecks {
		if st != "writable" {
			unhealthy = true
		}
	}

	stuck := s.checkStuckQueue(30 * time.Minute)
	checks["queue"] = stuck
	if stuck["stuck_count"].(int) > 0 {
		degraded = true
	}

	switch {
	case unhealthy:
		overall = "unhealthy"
	case degraded:
		overall = "degraded"
	default:
		overall = "healthy"
	}

	utils.JSONResponse(w, healthResponse{Status: overall, Checks: checks}, http.StatusOK)
}

func (s *Server) checkDebrids(ctx context.Context) map[string]string {
	out := make(map[string]string)
	cfg := config.Get()
	if cfg == nil {
		return out
	}
	for _, dc := range cfg.Debrids {
		client := s.manager.ProviderClient(dc.Name)
		if client == nil {
			out[dc.Name] = "unconfigured"
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := client.GetProfile()
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(cctx.Err(), context.DeadlineExceeded) {
				out[dc.Name] = "timeout"
			} else {
				out[dc.Name] = "error"
			}
			continue
		}
		out[dc.Name] = "ok"
	}
	return out
}

func (s *Server) checkArrs(ctx context.Context) map[string]string {
	out := make(map[string]string)
	cfg := config.Get()
	if cfg == nil {
		return out
	}
	for _, a := range cfg.Arrs {
		if a.Host == "" || a.Token == "" {
			out[a.Name] = "unconfigured"
			continue
		}
		arr := s.manager.Arr().GetOrCreate(a.Name)
		if arr == nil {
			out[a.Name] = "error"
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := arr.ValidateCtx(cctx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(cctx.Err(), context.DeadlineExceeded) {
				out[a.Name] = "timeout"
			} else {
				out[a.Name] = "error"
			}
			continue
		}
		out[a.Name] = "ok"
	}
	return out
}

func (s *Server) checkDiskWritable() map[string]string {
	out := make(map[string]string)
	cfg := config.Get()
	if cfg == nil {
		return out
	}
	paths := make(map[string]string)
	if len(cfg.CategoryPaths) > 0 {
		for cat, p := range cfg.CategoryPaths {
			paths[cat] = p
		}
	} else if cfg.DownloadFolder != "" {
		for _, cat := range cfg.Categories {
			paths[cat] = config.ResolveCategoryPath(cat, cfg.DownloadFolder, cat)
		}
	}
	for name, dir := range paths {
		out[name] = diskWritable(dir)
	}
	if cfg.DownloadFolder != "" {
		out["download_folder"] = diskWritable(cfg.DownloadFolder)
	}
	return out
}

func diskWritable(dir string) string {
	if dir == "" {
		return "missing"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "not_writable"
	}
	test := filepath.Join(dir, ".decypharr-health-"+time.Now().Format("150405"))
	if err := os.WriteFile(test, []byte("ok"), 0644); err != nil {
		return "not_writable"
	}
	_ = os.Remove(test)
	return "writable"
}

func (s *Server) checkStuckQueue(threshold time.Duration) map[string]interface{} {
	cutoff := time.Now().Add(-threshold)
	var stuck []string
	entries := s.manager.Queue().ListFilter("", config.ProtocolAll, storage.EntryStateDownloading, nil, "", true)
	for _, e := range entries {
		if e == nil {
			continue
		}
		debridDone := e.Status == debridTypes.TorrentStatusDownloaded || e.DebridProgress >= 1.0
		if debridDone && e.State == storage.EntryStateDownloading && e.UpdatedAt.Before(cutoff) {
			stuck = append(stuck, e.InfoHash)
		}
	}
	return map[string]interface{}{
		"stuck_count": len(stuck),
		"stuck_items": stuck,
	}
}
