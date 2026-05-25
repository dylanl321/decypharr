package torbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/debrid/types"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "decypharr-torbox-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	cfgPath := filepath.Join(dir, "config.json")
	cfg := map[string]any{
		"download_folder": dir,
		"debrids": []map[string]any{
			{"name": "torbox", "provider": "torbox", "api_key": "test"},
		},
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		panic(err)
	}
	config.SetConfigPath(dir)
	os.Exit(m.Run())
}

func TestGetNZBMapsDownloadedFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/usenet/mylist" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":                42,
				"hash":              "ABC123",
				"name":              "Release.Name",
				"size":              1000,
				"progress":          1.0,
				"download_state":    "downloaded",
				"download_finished": true,
				"created_at":        time.Now().Format(time.RFC3339),
				"files": []map[string]any{
					{"id": 7, "name": "folder/file.mkv", "size": 1000, "absolute_path": "folder/file.mkv"},
				},
			},
		})
	}))
	defer server.Close()

	client, err := New(config.Debrid{Name: "torbox", Provider: "torbox", APIKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tb := client
	tb.Host = server.URL

	dl, err := tb.GetNZB("42")
	if err != nil {
		t.Fatalf("GetNZB: %v", err)
	}
	if dl.Status != types.TorrentStatusDownloaded {
		t.Fatalf("status = %q, want downloaded", dl.Status)
	}
	file, ok := dl.Files["file.mkv"]
	if !ok {
		t.Fatal("expected file.mkv in files map")
	}
	if file.Link != "torbox-usenet://42/7" {
		t.Fatalf("link = %q", file.Link)
	}
}

func TestIsNZBAvailableCached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"HASH1": map[string]any{"size": 1, "name": "cached"},
			},
		})
	}))
	defer server.Close()

	client, err := New(config.Debrid{Name: "torbox", Provider: "torbox", APIKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tb := client
	tb.Host = server.URL

	avail := tb.IsNZBAvailable([]string{"HASH1"})
	if !avail["HASH1"] {
		t.Fatal("expected HASH1 to be cached")
	}
}

func TestSupportsNZB(t *testing.T) {
	client, err := New(config.Debrid{Name: "torbox", Provider: "torbox", APIKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !client.SupportsNZB() {
		t.Fatal("expected torbox client to support NZB")
	}
}
