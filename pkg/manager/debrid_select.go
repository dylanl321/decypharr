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
