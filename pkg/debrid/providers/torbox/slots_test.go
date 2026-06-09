package torbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/debrid/account"
	"github.com/sirrobot01/decypharr/pkg/debrid/types"
)

func TestGetAvailableSlots_SubtractsActiveCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/torrents/mylist"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": 1, "active": true, "download_finished": false},
					{"id": 2, "active": false, "download_finished": true},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/api/usenet/mylist"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": 10, "active": true, "download_finished": false},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(config.Debrid{Name: "torbox", Provider: "torbox", APIKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	client.Host = server.URL
	client.Profile = &types.Profile{Type: "pro"}

	slots, err := client.GetAvailableSlots()
	if err != nil {
		t.Fatalf("GetAvailableSlots: %v", err)
	}
	// pro plan = 10 slots; 1 active torrent + 1 active usenet = 2 used; 10 - 2 = 8 free
	if slots != 8 {
		t.Fatalf("slots = %d, want 8", slots)
	}
}

func TestGetAvailableSlots_ExcludesFailedItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/torrents/mylist"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": 1, "active": true, "download_finished": false, "download_state": "downloading"},
					{"id": 2, "active": false, "download_finished": false, "download_state": "failed (timeout)"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/api/usenet/mylist"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{
					{"id": 10, "active": false, "download_finished": false, "download_state": "failed (Aborted, cannot be completed - https://sabnzbd.org/not-complete)"},
					{"id": 11, "active": false, "download_finished": false, "download_state": "failed (Aborted, cannot be completed - https://sabnzbd.org/not-complete)"},
					{"id": 12, "active": true, "download_finished": false, "download_state": "downloading"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(config.Debrid{Name: "torbox", Provider: "torbox", APIKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	client.Host = server.URL
	client.Profile = &types.Profile{Type: "pro"}

	slots, err := client.GetAvailableSlots()
	if err != nil {
		t.Fatalf("GetAvailableSlots: %v", err)
	}
	// pro plan = 10 slots; 1 active torrent (failed one excluded) + 1 active usenet (2 failed excluded) = 2 used; 10 - 2 = 8 free
	if slots != 8 {
		t.Fatalf("slots = %d, want 8 (failed items should not count as active)", slots)
	}
}

func TestGetAvailableSlots_RespectsMaxActiveDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    []map[string]any{},
		})
	}))
	defer server.Close()

	client, err := New(config.Debrid{Name: "torbox", Provider: "torbox", APIKey: "test-key", MaxActiveDownloads: 4}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	client.Host = server.URL
	client.Profile = &types.Profile{Type: "pro"}

	slots, err := client.GetAvailableSlots()
	if err != nil {
		t.Fatalf("GetAvailableSlots: %v", err)
	}
	// pro = 10 slots, capped to 4 by config; no active downloads → 4 free
	if slots != 4 {
		t.Fatalf("slots = %d, want 4", slots)
	}
}

func TestFetchDownloadLink_SetsSkipValidation(t *testing.T) {
	client, err := New(config.Debrid{Name: "torbox", Provider: "torbox", APIKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dl, err := client.fetchDownloadLink(&account.Account{Token: "tok"}, "123", &types.File{Id: "7", Name: "movie.mkv", Link: "torbox://1"})
	if err != nil {
		t.Fatalf("fetchDownloadLink: %v", err)
	}
	if !dl.SkipValidation {
		t.Fatal("expected SkipValidation=true on torrent permalink to avoid HEAD-validation 429s")
	}
}
