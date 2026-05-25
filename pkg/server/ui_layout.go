package server

import (
	"net/http"

	"github.com/sirrobot01/decypharr/internal/config"
)

// NavItem describes a primary navigation link in the app shell.
type NavItem struct {
	Page     string
	Label    string
	Href     string
	Icon     string
	External bool
}

func primaryNavItems(urlBase string) []NavItem {
	return []NavItem{
		{Page: "index", Label: "Dashboard", Href: urlBase, Icon: "bi-grid-3x3-gap"},
		{Page: "download", Label: "Download", Href: urlBase + "download", Icon: "bi-cloud-download"},
		{Page: "browse", Label: "Browse", Href: urlBase + "browse", Icon: "bi-folder2-open"},
		{Page: "repair", Label: "Repair", Href: urlBase + "repair", Icon: "bi-wrench-adjustable"},
		{Page: "health", Label: "Health", Href: urlBase + "health", Icon: "bi-heart-pulse"},
		{Page: "stats", Label: "Stats", Href: urlBase + "stats", Icon: "bi-graph-up"},
		{Page: "config", Label: "Settings", Href: urlBase + "settings", Icon: "bi-gear"},
	}
}

func utilityNavItems(urlBase string) []NavItem {
	return []NavItem{
		{Label: "Logs", Href: urlBase + "debug/logs", Icon: "bi-journal-text", External: true},
	}
}

// showAppNav returns false on auth pages where the main queue nav is distracting.
func showAppNav(page string) bool {
	switch page {
	case "login", "register":
		return false
	default:
		return true
	}
}

func (s *Server) sessionUsername(r *http.Request) string {
	if r == nil {
		return ""
	}
	session, err := s.cookie.Get(r, "auth-session")
	if err != nil {
		return ""
	}
	if u, ok := session.Values["username"].(string); ok {
		return u
	}
	return ""
}

func (s *Server) showLogout(r *http.Request) bool {
	cfg := config.Get()
	if cfg == nil || !cfg.UseAuth {
		return false
	}
	session, err := s.cookie.Get(r, "auth-session")
	if err != nil {
		return false
	}
	auth, ok := session.Values["authenticated"].(bool)
	return ok && auth
}

func (s *Server) layoutData(r *http.Request, page, title string, extra map[string]interface{}) map[string]interface{} {
	cfg := config.Get()
	data := map[string]interface{}{
		"URLBase":      cfg.URLBase,
		"Page":         page,
		"Title":        title,
		"NavItems":     primaryNavItems(cfg.URLBase),
		"UtilityNav":   utilityNavItems(cfg.URLBase),
		"ShowNav":      showAppNav(page),
		"Username":     s.sessionUsername(r),
		"ShowLogout":   s.showLogout(r),
		"ShowAuthPage": page == "login" || page == "register",
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}
