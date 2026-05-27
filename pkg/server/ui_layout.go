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
		{Page: "overview", Label: "Overview", Href: urlBase + "overview", Icon: "bi-speedometer2"},
		{Page: "index", Label: "Queue", Href: urlBase, Icon: "bi-list-ul"},
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

// pageIconFor returns a Bootstrap Icon class used in the topbar breadcrumb for the given page.
func pageIconFor(page string) string {
	switch page {
	case "overview":
		return "bi-speedometer2"
	case "index":
		return "bi-list-ul"
	case "download":
		return "bi-cloud-download"
	case "browse":
		return "bi-folder2-open"
	case "repair":
		return "bi-wrench-adjustable"
	case "health":
		return "bi-heart-pulse"
	case "stats":
		return "bi-graph-up"
	case "config":
		return "bi-gear"
	case "login", "register":
		return "bi-shield-lock"
	case "setup":
		return "bi-magic"
	default:
		return "bi-grid-3x3-gap"
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

func contentWideFor(page string) bool {
	switch page {
	case "overview", "index", "browse", "stats":
		return true
	default:
		return false
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
		"PageIcon":     pageIconFor(page),
		"Title":        title,
		"NavItems":     primaryNavItems(cfg.URLBase),
		"UtilityNav":   utilityNavItems(cfg.URLBase),
		"ShowNav":      showAppNav(page),
		"Username":     s.sessionUsername(r),
		"ShowLogout":   s.showLogout(r),
		"ShowAuthPage": page == "login" || page == "register",
		"ContentWide":  contentWideFor(page),
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}
