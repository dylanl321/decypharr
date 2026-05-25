package manager

import (
	debrid "github.com/sirrobot01/decypharr/pkg/debrid/common"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func (m *Manager) RemoveFromProvider(entry *storage.Entry, providerEntry *storage.ProviderEntry) error {
	if providerEntry == nil {
		return nil
	}
	if providerEntry.Provider == "usenet" {
		if m.usenet != nil {
			return m.usenet.Delete(providerEntry.ID)
		}
		return nil
	}

	client := m.ProviderClient(providerEntry.Provider)
	if client == nil {
		return nil
	}
	if entry != nil && entry.IsNZB() && !entry.IsNNTPNZB() {
		if capable, ok := client.(debrid.NZBCapable); ok && capable.SupportsNZB() {
			return capable.AsNZBClient().DeleteNZB(providerEntry.ID)
		}
	}
	return client.DeleteTorrent(providerEntry.ID)
}

func (m *Manager) RemoveTorrentPlacements(t *storage.Entry) {
	for _, placement := range t.Providers {
		_ = m.RemoveFromProvider(t, placement)
	}
}