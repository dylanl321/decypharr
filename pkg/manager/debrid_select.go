package manager

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	debrid "github.com/sirrobot01/decypharr/pkg/debrid/common"
)

const cacheCheckTimeout = 5 * time.Second

// orderedDebridClients returns configured debrid clients in config order, optionally filtered by name.
func (m *Manager) orderedDebridClients(selectedDebrid string) []debrid.Client {
	cfg := config.Get()
	out := make([]debrid.Client, 0, len(cfg.Debrids))
	for _, dc := range cfg.Debrids {
		if selectedDebrid != "" && dc.Name != selectedDebrid {
			continue
		}
		client := m.ProviderClient(dc.Name)
		if client != nil {
			out = append(out, client)
		}
	}
	return out
}

// applyDebridOrder reorders a list of debrid configs so that names appearing
// in `preferred` come first (in the listed order), followed by any remaining
// configs in their original order. Names in `preferred` that don't exist in
// the source list are silently ignored.
func applyDebridOrder(debrids []config.Debrid, preferred []string) []config.Debrid {
	if len(preferred) == 0 || len(debrids) == 0 {
		return debrids
	}
	byName := make(map[string]config.Debrid, len(debrids))
	for _, d := range debrids {
		byName[d.Name] = d
	}
	seen := make(map[string]bool, len(debrids))
	out := make([]config.Debrid, 0, len(debrids))
	for _, name := range preferred {
		if d, ok := byName[name]; ok && !seen[name] {
			out = append(out, d)
			seen[name] = true
		}
	}
	for _, d := range debrids {
		if !seen[d.Name] {
			out = append(out, d)
			seen[d.Name] = true
		}
	}
	return out
}

// orderedTorrentDebridClients returns torrent-eligible clients in preferred order.
// Honors the per-debrid `allow_torrents` flag, the global `torrent_debrid` pin,
// and the optional `torrent_debrid_order` preference list.
func (m *Manager) orderedTorrentDebridClients(selectedDebrid string) []debrid.Client {
	cfg := config.Get()
	pin := selectedDebrid
	if pin == "" {
		pin = cfg.TorrentDebrid
	}
	debrids := applyDebridOrder(cfg.Debrids, cfg.TorrentDebridOrder)
	out := make([]debrid.Client, 0, len(debrids))
	for _, dc := range debrids {
		if pin != "" && dc.Name != pin {
			continue
		}
		if !dc.AllowsTorrents() {
			continue
		}
		client := m.ProviderClient(dc.Name)
		if client != nil {
			out = append(out, client)
		}
	}
	return out
}

// filterClientsBySlots returns only clients whose providers report enough free slots.
// Providers with errors are skipped. Honors `minimum_free_slot` per debrid.
func (m *Manager) filterClientsBySlots(clients []debrid.Client) []debrid.Client {
	if len(clients) == 0 {
		return clients
	}
	out := make([]debrid.Client, 0, len(clients))
	for _, c := range clients {
		dc := c.Config()
		slots, err := c.GetAvailableSlots()
		if err != nil {
			m.logger.Debug().
				Err(err).
				Str("provider", dc.Name).
				Msg("Could not determine available slots; skipping for selection")
			continue
		}
		if slots <= dc.MinimumFreeSlot {
			m.logger.Info().
				Str("provider", dc.Name).
				Int("slots", slots).
				Int("minimum_free_slot", dc.MinimumFreeSlot).
				Msg("Provider slot-exhausted; skipping for selection")
			continue
		}
		out = append(out, c)
	}
	return out
}

// filterNZBClientsBySlots applies the same slot logic to the NZB-named client list,
// using the underlying provider's GetAvailableSlots when available.
func (m *Manager) filterNZBClientsBySlots(items []namedNZBClient) []namedNZBClient {
	if len(items) == 0 {
		return items
	}
	out := make([]namedNZBClient, 0, len(items))
	for _, item := range items {
		client := m.ProviderClient(item.name)
		if client == nil {
			continue
		}
		dc := client.Config()
		slots, err := client.GetAvailableSlots()
		if err != nil {
			m.logger.Debug().
				Err(err).
				Str("provider", item.name).
				Msg("Could not determine available NZB slots; skipping for selection")
			continue
		}
		if slots <= dc.MinimumFreeSlot {
			m.logger.Info().
				Str("provider", item.name).
				Int("slots", slots).
				Int("minimum_free_slot", dc.MinimumFreeSlot).
				Msg("Provider slot-exhausted; skipping for NZB selection")
			continue
		}
		out = append(out, item)
	}
	return out
}

type cacheProbeResult struct {
	client   debrid.Client
	cached   bool
	probeErr error
}

// selectCachedProvider checks all providers in parallel and returns the first cached match in config order.
func (m *Manager) selectCachedProvider(ctx context.Context, infoHash string, clients []debrid.Client) (debrid.Client, bool) {
	if len(clients) == 0 || infoHash == "" {
		return nil, false
	}
	hash := strings.ToUpper(infoHash)

	ctx, cancel := context.WithTimeout(ctx, cacheCheckTimeout)
	defer cancel()

	results := make([]cacheProbeResult, len(clients))
	var wg sync.WaitGroup
	wg.Add(len(clients))
	for i, client := range clients {
		i, client := i, client
		go func() {
			defer wg.Done()
			results[i].client = client
			// AllDebrid returns empty availability; skip false negatives.
			if client.Config().Provider == "alldebrid" {
				return
			}
			avail := client.IsAvailable([]string{hash})
			if avail != nil && avail[hash] {
				results[i].cached = true
			}
		}()
	}
	wg.Wait()

	for _, client := range clients {
		for _, r := range results {
			if r.client != nil && r.client.Config().Name == client.Config().Name && r.cached {
				m.logger.Info().
					Str("provider", client.Config().Name).
					Str("hash", hash).
					Msg("Pre-flight cache check: torrent cached on provider")
				return client, true
			}
		}
	}
	return nil, false
}
