package manager

import (
	"testing"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func TestCanRetryLocalPull(t *testing.T) {
	m := &Manager{}
	entry := &storage.Entry{
		ActiveProvider: "torbox",
		DebridProgress: 1.0,
		Status:         types.TorrentStatusDownloaded,
		Files: map[string]*storage.File{
			"movie.mkv": {Name: "movie.mkv", Size: 1000},
		},
		Timeline: []storage.TimelineEvent{
			{At: time.Now(), Kind: storage.TimelineLocalDownloadStart, Provider: "torbox"},
		},
	}
	if !m.canRetryLocalPull(entry) {
		t.Fatal("expected local pull retry when debrid complete with files")
	}

	entry.Files = nil
	if m.canRetryLocalPull(entry) {
		t.Fatal("expected false without files")
	}
}

func TestCanRetryLocalPull_NoProvider(t *testing.T) {
	m := &Manager{}
	if m.canRetryLocalPull(&storage.Entry{DebridProgress: 1.0}) {
		t.Fatal("expected false without provider")
	}
}

func TestImportReqFromEntry_UsesEntryAction(t *testing.T) {
	m := testManagerWithQueue(t)
	entry := &storage.Entry{
		InfoHash:       "abc",
		Name:           "Test",
		Magnet:         "magnet:?xt=urn:btih:abc",
		Category:       "tv",
		ActiveProvider: "torbox",
		Action:         config.DownloadActionSymlink,
		SavePath:       "/downloads/tv",
		Protocol:       config.ProtocolTorrent,
	}
	req := m.importReqFromEntry(entry)
	if req == nil {
		t.Fatal("expected import request")
	}
	if req.Action != config.DownloadActionSymlink {
		t.Fatalf("action: got %q", req.Action)
	}
	if req.SelectedDebrid != "torbox" {
		t.Fatalf("debrid: got %q", req.SelectedDebrid)
	}
}
