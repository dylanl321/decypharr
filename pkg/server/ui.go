package server

import (
	"net/http"

	json "github.com/bytedance/sonic"

	"github.com/sirrobot01/decypharr/internal/config"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) LoginHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if cfg.NeedsAuth() {
		http.Redirect(w, r, cfg.URLBase+"register", http.StatusSeeOther)
		return
	}
	if r.Method == "GET" {
		data := s.layoutData(r, "login", "Login", nil)
		err := s.templates.ExecuteTemplate(w, "layout", data)
		if err != nil {
			s.logger.Warn().Err(err).Msg("error rendering /login template")
		}
		return
	}

	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.ConfigDefault.NewDecoder(r.Body).Decode(&credentials); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if s.verifyAuth(credentials.Username, credentials.Password) {
		session, _ := s.cookie.Get(r, "auth-session")
		session.Values["authenticated"] = true
		session.Values["username"] = credentials.Username
		if err := session.Save(r, w); err != nil {
			http.Error(w, "Error saving session", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, cfg.URLBase, http.StatusSeeOther)
		return
	}

	http.Error(w, "Invalid credentials", http.StatusUnauthorized)
}

func (s *Server) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	session, _ := s.cookie.Get(r, "auth-session")
	session.Values["authenticated"] = false
	session.Options.MaxAge = -1
	err := session.Save(r, w)
	if err != nil {
		return
	}
	http.Redirect(w, r, cfg.URLBase+"login", http.StatusSeeOther)
}

func (s *Server) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	authCfg := cfg.GetAuth()

	if r.Method == "GET" {
		data := s.layoutData(r, "register", "Register", nil)
		err := s.templates.ExecuteTemplate(w, "layout", data)
		if err != nil {
			s.logger.Warn().Err(err).Msg("error rendering /register template")
		}
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")

	if password != confirmPassword {
		http.Error(w, "Passwords do not match", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Error processing password", http.StatusInternalServerError)
		return
	}

	authCfg.Username = username
	authCfg.Password = string(hashedPassword)

	if err := cfg.SaveAuth(authCfg); err != nil {
		http.Error(w, "Error saving credentials", http.StatusInternalServerError)
		return
	}

	session, _ := s.cookie.Get(r, "auth-session")
	session.Values["authenticated"] = true
	session.Values["username"] = username
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Error saving session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, cfg.URLBase, http.StatusSeeOther)
}

func (s *Server) IndexHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	data := s.layoutData(r, "index", "Queues", map[string]interface{}{
		"SetupError":              cfg.SetupError(),
		"SetupAlertMessage":       "Your configuration is incomplete (" + cfg.SetupError() + "). Please complete the setup in the",
		"SetupAlertLink":          cfg.URLBase + "settings",
		"SetupAlertLinkText":      "Settings page",
	})
	err := s.templates.ExecuteTemplate(w, "layout", data)
	if err != nil {
		s.logger.Warn().Err(err).Msg("error rendering /index template")
	}
}

func (s *Server) DownloadHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	debrids := make([]string, 0)
	for _, d := range cfg.Debrids {
		debrids = append(debrids, d.Name)
	}
	data := s.layoutData(r, "download", "Download", map[string]interface{}{
		"Debrids":                 debrids,
		"HasMultiDebrid":          len(debrids) > 1,
		"downloadFolder":          cfg.DownloadFolder,
		"alwaysRemoveTrackerURLS": cfg.AlwaysRmTrackerUrls,
		"SetupError":              cfg.SetupError(),
		"SetupAlertMessage":       "Your configuration is incomplete (" + cfg.SetupError() + "). Please complete the setup in the",
		"SetupAlertLink":          cfg.URLBase + "settings",
		"SetupAlertLinkText":      "Settings page",
	})
	err := s.templates.ExecuteTemplate(w, "layout", data)
	if err != nil {
		s.logger.Warn().Err(err).Msg("error rendering /download template")
	}
}

func (s *Server) RepairHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	data := s.layoutData(r, "repair", "Repair", map[string]interface{}{
		"SetupError":        cfg.SetupError(),
		"SetupAlertMessage": "Your configuration is incomplete (" + cfg.SetupError() + "). Please complete the setup in the",
		"SetupAlertLink":    cfg.URLBase + "settings",
		"SetupAlertLinkText": "Settings page",
	})
	err := s.templates.ExecuteTemplate(w, "layout", data)
	if err != nil {
		s.logger.Warn().Err(err).Msg("error rendering /repair template")
	}
}

func (s *Server) ConfigHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	data := s.layoutData(r, "config", "Config", map[string]interface{}{
		"SetupError": cfg.SetupError(),
	})
	err := s.templates.ExecuteTemplate(w, "layout", data)
	if err != nil {
		s.logger.Warn().Err(err).Msg("error rendering /config template")
	}
}

func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	data := s.layoutData(r, "stats", "Statistics", nil)
	err := s.templates.ExecuteTemplate(w, "layout", data)
	if err != nil {
		s.logger.Warn().Err(err).Msg("error rendering /stats template")
	}
}

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	data := s.layoutData(r, "health", "System Health", map[string]interface{}{
		"SetupError": cfg.SetupError(),
	})
	err := s.templates.ExecuteTemplate(w, "layout", data)
	if err != nil {
		s.logger.Warn().Err(err).Msg("error rendering /health template")
	}
}

func (s *Server) BrowseHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	data := s.layoutData(r, "browse", "Browse Torrents", map[string]interface{}{
		"SetupError":        cfg.SetupError(),
		"SetupAlertMessage": "Your configuration is incomplete (" + cfg.SetupError() + "). Please complete the setup in the",
		"SetupAlertLink":    cfg.URLBase + "settings",
		"SetupAlertLinkText": "Settings page",
	})
	err := s.templates.ExecuteTemplate(w, "layout", data)
	if err != nil {
		s.logger.Warn().Err(err).Msg("error rendering /browse template")
	}
}
