package manager

import (
	"testing"

	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
)

func TestDebridTorrentReadyForLocalPull(t *testing.T) {
	if debridTorrentReadyForLocalPull(nil) {
		t.Fatal("nil torrent should not be ready")
	}
	downloading := &debridTypes.Torrent{Status: debridTypes.TorrentStatusDownloading}
	if debridTorrentReadyForLocalPull(downloading) {
		t.Fatal("downloading should not be ready")
	}
	cachedNoFiles := &debridTypes.Torrent{Status: debridTypes.TorrentStatusDownloaded, Files: map[string]debridTypes.File{}}
	if debridTorrentReadyForLocalPull(cachedNoFiles) {
		t.Fatal("downloaded with empty files should not be ready")
	}
	ready := &debridTypes.Torrent{
		Status: debridTypes.TorrentStatusDownloaded,
		Files:  map[string]debridTypes.File{"a.mkv": {Name: "a.mkv"}},
	}
	if !debridTorrentReadyForLocalPull(ready) {
		t.Fatal("downloaded with files should be ready")
	}
}
