package server

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) WebRoutes() http.Handler {
	r := chi.NewRouter()

	// Apply setup redirect middleware globally
	r.Use(s.setupRedirectMiddleware)

	// Static assets - always public
	staticFS, _ := fs.Sub(assetsEmbed, "assets/build")
	imagesFS, _ := fs.Sub(imagesEmbed, "assets/images")
	r.Handle("/assets/*", http.StripPrefix(s.urlBase+"assets/", http.FileServer(http.FS(staticFS))))
	r.Handle("/images/*", http.StripPrefix(s.urlBase+"images/", http.FileServer(http.FS(imagesFS))))

	// Public routes - no auth needed
	r.Get("/version", s.handleGetVersion)
	r.Get("/login", s.LoginHandler)
	r.Post("/login", s.LoginHandler)
	r.Get("/register", s.RegisterHandler)
	r.Post("/register", s.RegisterHandler)
	r.Post("/skip-auth", s.skipAuthHandler)

	// Setup wizard - public, no auth required
	r.Get("/setup", s.SetupHandler)
	r.Post("/api/setup/complete", s.setupCompleteHandler)
	r.Get("/api/health", s.handleAPIHealth)

	// Protected routes - require auth
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)
		// Web pages
		r.Get("/", s.IndexHandler)
		r.Get("/overview", s.OverviewHandler)
		r.Get("/browse", s.BrowseHandler)
		r.Get("/download", s.DownloadHandler)
		r.Get("/repair", s.RepairHandler)
		r.Get("/stats", s.StatsHandler)
		r.Get("/health", s.HealthHandler)
		r.Get("/settings", s.ConfigHandler)
		r.Get("/logout", s.LogoutHandler)

		// API routes
		r.Route("/api", func(r chi.Router) {
			// Health and Status (public GET /api/health uses handleAPIHealth for Docker/reliability)
			r.Get("/health/components", s.handleHealth)
			r.Get("/stats", s.stats.Handler())
			r.Get("/debrid/status", s.handleDebridStatus)
			r.Get("/queue/summary", s.handleQueueSummary)
			r.Get("/bandwidth", s.handleBandwidth)
			r.Put("/bandwidth", s.handleUpdateBandwidth)
			r.Get("/version", s.handleGetAPIVersion)

			// Arr management (full CRUD)
			r.Get("/arrs", s.handleGetArrs)
			r.Post("/arrs", s.handleAddArr)
			r.Put("/arrs/{name}", s.handleUpdateArr)
			r.Delete("/arrs/{name}", s.handleDeleteArr)
			r.Post("/arrs/{name}/test", s.handleTestArr)
			r.Get("/debrid/rate-limits", s.handleDebridRateLimits)
			r.Post("/add", s.handleAddContent)

			// Queue management (CRUD + Actions)
			r.Get("/queue/{hash}", s.handleGetQueueItem)
			r.Get("/queue/{hash}/timeline", s.handleGetQueueTimeline)
			r.Post("/queue/{hash}/retry", s.handleRetryQueueItem)
			r.Post("/queue/{hash}/pause", s.handlePauseQueueItem)
			r.Post("/queue/{hash}/resume", s.handleResumeQueueItem)
			r.Delete("/queue/completed", s.handleDeleteCompleted)
			r.Delete("/queue/errors", s.handleDeleteErrors)
			r.Post("/queue/retry-all-errors", s.handleRetryAllErrors)
			r.Post("/queue/cleanup", s.handleCleanupCompleted)

			// Debrid Provider Management
			r.Get("/debrid/providers", s.handleGetDebridProviders)
			r.Post("/debrid/providers/{name}/test", s.handleTestDebridProvider)

			// Notification Settings
			r.Get("/notifications/config", s.handleGetNotificationConfig)
			r.Put("/notifications/config", s.handleUpdateNotificationConfig)

			// Mount Status/Info
			r.Get("/mount/status", s.handleGetMountStatus)
			r.Post("/mount/refresh", s.handleRefreshMount)

			// Log Viewing
			r.Get("/logs", s.handleGetLogs)

			// System/Service Control
			r.Post("/restart", s.handleRestart)

			// Repair / health-checker operations
			r.Get("/repair/config", s.handleGetRepairConfig)
			r.Put("/repair/config", s.handleUpdateRepairConfig)
			r.Get("/repair/status", s.handleRepairStatus)
			r.Post("/repair/run", s.handleRunRepair)
			r.Post("/repair/stop", s.handleStopRepair)
			r.Post("/repair/recheck/media", s.handleRecheckMedia)
			r.Post("/repair/fix", s.handleFixBroken)
			r.Post("/repair/clear", s.handleClearBroken)
			r.Post("/repair/clear-state", s.handleClearRepairState)
			r.Get("/repair/runs", s.handleListRepairRuns)
			r.Get("/repair/runs/{id}", s.handleGetRepairRun)
			r.Delete("/repair/runs", s.handleClearRepairRuns)
			r.Get("/repair/health", s.handleListEntryHealth)
			r.Get("/repair/health/{name}", s.handleGetEntryHealth)
			r.Post("/repair/health/{name}/check", s.handleRecheckEntry)

			// Torrent management
			r.Get("/torrents", s.handleGetTorrents)
			r.Delete("/torrents/{category}/{hash}", s.handleDeleteTorrent)
			r.Delete("/torrents", s.handleDeleteTorrents) // Fixed trailing slash

			// Browse - WebDAV-style hierarchical file browser
			r.Route("/browse", func(r chi.Router) {
				// Hierarchical browse endpoints
				r.Get("/", s.handleBrowseMount)                                    // Mount: groups (__all__, __bad__, etc.)
				r.Get("/{group}", s.handleBrowseGroup)                             // Group: torrents
				r.Get("/{group}/{subgroup}/{torrent}", s.handleBrowseTorrentFiles) // Torrent files (with subgroup)
				r.Get("/{group}/{torrent}", s.handleBrowseTorrentFiles)            // Torrent files (without subgroup) - This route needs to come after the subgroup route

				// Torrent operations
				r.Delete("/torrents/{id}", s.handleDeleteBrowseTorrent)
				r.Delete("/torrents/batch", s.handleBatchDeleteBrowseTorrents)

				// File download
				r.Get("/download/{torrent}/{file}", s.handleDownloadFile)
			})

			// Config/Auth
			r.Get("/config", s.handleGetConfig)
			r.Post("/config", s.handleUpdateConfig)
			r.Post("/refresh-token", s.handleRefreshAPIToken)
			r.Post("/update-auth", s.handleUpdateAuth)
		})
	})

	return r
}
