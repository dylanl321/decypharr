package config

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
)

type Debrid struct {
	Provider                     string   `json:"provider,omitempty"` // realdebrid, alldebrid, debridlink, torbox, premiumize
	Name                         string   `json:"name,omitempty"`
	APIKey                       string   `json:"api_key,omitempty"`
	DownloadAPIKeys              []string `json:"download_api_keys,omitempty"`
	DownloadUncached             bool     `json:"download_uncached,omitempty"`
	RateLimit                    string   `json:"rate_limit,omitempty"` // 200/minute or 10/second
	RepairRateLimit              string   `json:"repair_rate_limit,omitempty"`
	DownloadRateLimit            string   `json:"download_rate_limit,omitempty"`
	Proxy                        string   `json:"proxy,omitempty"`
	UnpackRar                    bool     `json:"unpack_rar,omitempty"`
	MinimumFreeSlot              int      `json:"minimum_free_slot,omitempty"` // Minimum active pots to use this debrid
	Limit                        int      `json:"limit,omitempty"`             // Maximum number of total torrents
	TorrentsRefreshInterval      string   `json:"torrents_refresh_interval,omitempty"`
	DownloadLinksRefreshInterval string   `json:"download_links_refresh_interval,omitempty"`
	Workers                      int      `json:"workers,omitempty"`
	AutoExpireLinksAfter         string   `json:"auto_expire_links_after,omitempty"`
	UserAgent                    string   `json:"user_agent,omitempty"`

	// Routing capabilities. Pointers so an unset value can fall back to provider-specific defaults.
	AllowTorrents      *bool `json:"allow_torrents,omitempty"`       // default true
	AllowNZBs          *bool `json:"allow_nzbs,omitempty"`           // default true for NZB-capable providers (torbox), false otherwise
	PreferTorrents     *bool `json:"prefer_torrents,omitempty"`      // try this provider before other torrent-eligible debrids
	PreferNZBs         *bool `json:"prefer_nzbs,omitempty"`          // try this provider before other NZB-eligible debrids
	MaxActiveDownloads int   `json:"max_active_downloads,omitempty"` // optional cap; 0 = use provider plan limit

	// Post-completion cleanup
	RemoveOnComplete *bool `json:"remove_on_complete,omitempty"` // delete from this provider after local completion to free slots / stop seeding

	// Local download throttling. Empty/zero disables throttling for this
	// provider. Values support standard suffixes parsed by ParseBandwidth
	// ("10MB/s", "1.5MiB/s", "500KB/s", or raw bytes/sec). The effective
	// rate is the more restrictive of this and Config.Download.BandwidthLimit.
	BandwidthLimit string `json:"bandwidth_limit,omitempty"`

	// Folder
	Folder        string `json:"folder,omitempty"`          // Deprecated. Use Mount MountPath instead.
	FolderNaming  string `json:"folder_naming,omitempty"`   // Deprecated. Use global setting instead.
	RcUrl         string `json:"rc_url,omitempty"`          // Deprecated. Use global setting instead.
	RcUser        string `json:"rc_user,omitempty"`         // Deprecated. Use global setting instead.
	RcPass        string `json:"rc_pass,omitempty"`         // Deprecated. Use global setting instead.
	RcRefreshDirs string `json:"rc_refresh_dirs,omitempty"` // Deprecated. Use global setting instead.

	// Directories
	Directories map[string]WebdavDirectories `json:"directories,omitempty"` // Deprecated. Use global setting instead.
}

func (c *Config) updateDebrid(d Debrid) Debrid {
	workers := runtime.NumCPU() * 50
	perDebrid := workers / len(c.Debrids)

	if d.Provider == "" {
		d.Provider = d.Name
	}

	var downloadKeys []string

	if len(d.DownloadAPIKeys) > 0 {
		downloadKeys = d.DownloadAPIKeys
	} else {
		// If no download API keys are specified, use the main API key
		downloadKeys = []string{d.APIKey}
	}
	d.DownloadAPIKeys = downloadKeys

	if d.TorrentsRefreshInterval == "" {
		d.TorrentsRefreshInterval = DefaultTorrentsRefreshInterval
	}
	if d.DownloadLinksRefreshInterval == "" {
		d.DownloadLinksRefreshInterval = DefaultDownloadsRefreshInterval
	}
	if d.Workers == 0 {
		d.Workers = perDebrid
	}
	if d.AutoExpireLinksAfter == "" {
		d.AutoExpireLinksAfter = DefaultAutoExpireLinksAfter
	}

	if d.AllowTorrents == nil {
		t := true
		d.AllowTorrents = &t
	}
	if d.AllowNZBs == nil {
		// NZB capability follows provider type today: only Torbox supports debrid-backed NZBs.
		nzb := d.Provider == "torbox"
		d.AllowNZBs = &nzb
	}

	return d
}

// PrefersTorrents returns true when this debrid should be tried before other torrent-eligible providers.
func (d Debrid) PrefersTorrents() bool {
	return d.PreferTorrents != nil && *d.PreferTorrents
}

// PrefersNZBs returns true when this debrid should be tried before other NZB-eligible providers.
func (d Debrid) PrefersNZBs() bool {
	return d.PreferNZBs != nil && *d.PreferNZBs
}

// AllowsTorrents returns true if this debrid is eligible for torrent submissions.
func (d Debrid) AllowsTorrents() bool {
	if d.AllowTorrents == nil {
		return true
	}
	return *d.AllowTorrents
}

// AllowsNZBs returns true if this debrid is eligible for NZB submissions.
// Defaults to true for torbox (currently the only NZB-capable provider) and false otherwise.
func (d Debrid) AllowsNZBs() bool {
	if d.AllowNZBs == nil {
		return d.Provider == "torbox"
	}
	return *d.AllowNZBs
}

// RemovesOnComplete returns true if this debrid should auto-delete its placement after local completion.
func (d Debrid) RemovesOnComplete() bool {
	if d.RemoveOnComplete == nil {
		return false
	}
	return *d.RemoveOnComplete
}

func validateDebrids(debrids []Debrid) error {
	if len(debrids) == 0 {
		return nil
	}

	for _, debrid := range debrids {
		// Basic field validation
		if debrid.APIKey == "" {
			return errors.New("debrid api key is required")
		}
	}

	return nil
}

func (c *Config) applyDebridEnvVars() {
	// Debrid providers array
	for i := 0; i < 10; i++ { // Support up to 10 debrid providers
		prefix := fmt.Sprintf("DEBRIDS__%d__", i)
		if val := getEnv(prefix + "NAME"); val != "" {
			// Ensure array is large enough
			if i >= len(c.Debrids) {
				c.Debrids = append(c.Debrids, make([]Debrid, i-len(c.Debrids)+1)...)
			}
			c.Debrids[i].Name = val

			// Set other debrid fields
			if apiKey := getEnv(prefix + "API_KEY"); apiKey != "" {
				c.Debrids[i].APIKey = apiKey
			}
			if folder := getEnv(prefix + "FOLDER"); folder != "" {
				c.Debrids[i].Folder = folder
			}
			if provider := getEnv(prefix + "PROVIDER"); provider != "" {
				c.Debrids[i].Provider = provider
			}
			if proxy := getEnv(prefix + "PROXY"); proxy != "" {
				c.Debrids[i].Proxy = proxy
			}
			if val := getEnv(prefix + "ALLOW_TORRENTS"); val != "" {
				b := parseBool(val)
				c.Debrids[i].AllowTorrents = &b
			}
			if val := getEnv(prefix + "ALLOW_NZBS"); val != "" {
				b := parseBool(val)
				c.Debrids[i].AllowNZBs = &b
			}
			if val := getEnv(prefix + "PREFER_TORRENTS"); val != "" {
				b := parseBool(val)
				c.Debrids[i].PreferTorrents = &b
			}
			if val := getEnv(prefix + "PREFER_NZBS"); val != "" {
				b := parseBool(val)
				c.Debrids[i].PreferNZBs = &b
			}
			if val := getEnv(prefix + "MAX_ACTIVE_DOWNLOADS"); val != "" {
				if n, err := strconv.Atoi(val); err == nil {
					c.Debrids[i].MaxActiveDownloads = n
				}
			}
			if val := getEnv(prefix + "MINIMUM_FREE_SLOT"); val != "" {
				if n, err := strconv.Atoi(val); err == nil {
					c.Debrids[i].MinimumFreeSlot = n
				}
			}
			if val := getEnv(prefix + "REMOVE_ON_COMPLETE"); val != "" {
				b := parseBool(val)
				c.Debrids[i].RemoveOnComplete = &b
			}
		}
	}
}
