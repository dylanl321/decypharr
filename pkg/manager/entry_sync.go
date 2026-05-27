package manager

import (
	"time"

	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

// debridTorrentReadyForLocalPull is true when the provider reports complete and at least one
// file is known. Torbox (and others) may return status "cached"/downloaded before the file
// list is populated; starting the local pull then yields "0 files" / no download links.
func debridTorrentReadyForLocalPull(t *debridTypes.Torrent) bool {
	if t == nil || t.Status != debridTypes.TorrentStatusDownloaded {
		return false
	}
	for _, f := range t.Files {
		if f.Name != "" && !f.Deleted {
			return true
		}
	}
	return false
}

// backfillEntryFromDebrid merges provider status and file list from a remote debrid torrent
// into the queue entry. Required before HTTP pull when files were not populated at submit time.
func backfillEntryFromDebrid(entry *storage.Entry, debridTorrent *debridTypes.Torrent) {
	if debridTorrent == nil {
		return
	}
	if entry.Files == nil {
		entry.Files = make(map[string]*storage.File)
	}
	_ = entry.AddTorrentProvider(debridTorrent)
	entry.ActiveProvider = debridTorrent.Debrid
	entry.Status = debridTorrent.Status
	entry.Bytes = debridTorrent.GetSize()
	if debridTorrent.GetSize() > 0 {
		entry.Size = debridTorrent.GetSize()
	}
	if debridTorrent.Name != "" {
		entry.Name = debridTorrent.Name
	}
	if debridTorrent.OriginalFilename != "" {
		entry.OriginalFilename = debridTorrent.OriginalFilename
	}
	for _, file := range debridTorrent.Files {
		if file.Name == "" {
			continue
		}
		entry.Files[file.Name] = &storage.File{
			Name:      file.Name,
			Size:      file.Size,
			ByteRange: file.ByteRange,
			Deleted:   file.Deleted,
			InfoHash:  entry.InfoHash,
			AddedOn:   entry.AddedOn,
		}
	}
	if placement := entry.GetActiveProvider(); placement != nil {
		placement.Progress = entry.Progress
		if debridTorrent.Status == debridTypes.TorrentStatusDownloaded {
			now := time.Now()
			placement.DownloadedAt = &now
			placement.Progress = 1.0
		}
		touchProviderProgress(placement, entry.Progress)
	}
}

// touchProviderProgress updates LastProgressAt when progress advances on a placement.
func touchProviderProgress(placement *storage.ProviderEntry, progress float64) {
	if placement == nil {
		return
	}
	if progress > placement.LastProgressValue+0.001 || placement.LastProgressAt.IsZero() {
		placement.LastProgressAt = time.Now()
		placement.LastProgressValue = progress
	}
	placement.Progress = progress
}
