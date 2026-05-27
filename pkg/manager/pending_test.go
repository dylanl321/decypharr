package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/internal/utils"
	"github.com/sirrobot01/decypharr/pkg/arr"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "decypharr-manager-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	_ = os.Chdir(dir)
	config.SetConfigPath(dir)
	os.Exit(m.Run())
}

func testManagerWithQueue(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	strg, err := storage.NewStorage(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	ctx := context.Background()
	return &Manager{
		storage:        strg,
		queue:          newQueue(ctx, strg, 100, ""),
		ctx:            ctx,
		pendingSubmits: xsync.NewMap[string, struct{}](),
		arr:            arr.NewStorage(),
		config:         &config.Config{},
	}
}

func TestNzbImportHash(t *testing.T) {
	content := []byte("nzb test payload")
	h1 := nzbImportHash(content)
	h2 := nzbImportHash(content)
	if h1 != h2 {
		t.Fatalf("expected stable hash, got %q and %q", h1, h2)
	}
	for _, c := range h1 {
		if c >= 'A' && c <= 'F' {
			t.Fatalf("hash should be lowercase, got %q", h1)
		}
	}
}

func TestUpsertPending_Idempotent(t *testing.T) {
	dir := t.TempDir()
	strg, err := storage.NewStorage(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	q := newQueue(context.Background(), strg, 100, "")
	a := arr.New("tv", "", "", false, false, nil, "", "")
	req := &ImportRequest{
		Magnet: &utils.Magnet{
			InfoHash: "8a19577fb5f690970ca43a57ff1011ae202244b8",
			Name:     "Test Release",
			Link:     "magnet:?xt=urn:btih:8a19577fb5f690970ca43a57ff1011ae202244b8",
		},
		Arr: a,
	}
	if err := q.UpsertPending(req, pendingReasonAccepted); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := q.UpsertPending(req, pendingReasonAccepted); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	hash := "8a19577fb5f690970ca43a57ff1011ae202244b8"
	entry, err := q.GetTorrent(hash)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if entry.State != storage.EntryStatePending {
		t.Fatalf("state: got %q", entry.State)
	}
	if entry.PendingReason != pendingReasonAccepted {
		t.Fatalf("reason: got %q", entry.PendingReason)
	}
	if entry.LastAttemptAt != nil {
		t.Fatal("fresh accept should not set LastAttemptAt")
	}
}

func TestAddNewTorrent_QBitAcceptsWithoutDebrid(t *testing.T) {
	m := testManagerWithQueue(t)
	a := arr.New("tv", "", "", false, false, nil, "", "")
	magnet := &utils.Magnet{
		InfoHash: "8a19577fb5f690970ca43a57ff1011ae202244b8",
		Name:     "Test Release",
		Link:     "magnet:?xt=urn:btih:8a19577fb5f690970ca43a57ff1011ae202244b8",
	}
	req := NewTorrentRequest("", "/downloads", magnet, a, config.DownloadActionSymlink, nil, "", ImportTypeQBit, false)

	if err := m.AddNewTorrent(context.Background(), req); err != nil {
		t.Fatalf("AddNewTorrent: %v", err)
	}

	entry, err := m.queue.GetTorrent(magnet.InfoHash)
	if err != nil {
		t.Fatalf("get queue entry: %v", err)
	}
	if entry.State != storage.EntryStatePending {
		t.Fatalf("expected pending, got %q", entry.State)
	}
	if entry.PendingReason != pendingReasonAccepted {
		t.Fatalf("expected accepted reason, got %q", entry.PendingReason)
	}
}

func TestAcceptTorrentImport_Idempotent(t *testing.T) {
	m := testManagerWithQueue(t)
	a := arr.New("tv", "", "", false, false, nil, "", "")
	magnet := &utils.Magnet{
		InfoHash: "8a19577fb5f690970ca43a57ff1011ae202244b8",
		Name:     "Test Release",
		Link:     "magnet:?xt=urn:btih:8a19577fb5f690970ca43a57ff1011ae202244b8",
	}
	req := NewTorrentRequest("", "/downloads", magnet, a, config.DownloadActionSymlink, nil, "", ImportTypeQBit, false)

	h1, err := m.acceptTorrentImport(req)
	if err != nil {
		t.Fatalf("first accept: %v", err)
	}
	h2, err := m.acceptTorrentImport(req)
	if err != nil {
		t.Fatalf("second accept: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hash mismatch: %q vs %q", h1, h2)
	}
}
