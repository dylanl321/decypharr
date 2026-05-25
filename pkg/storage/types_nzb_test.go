package storage_test

import (
	"testing"

	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/storage"
)

func TestIsNNTPNZB(t *testing.T) {
	nntp := &storage.Entry{Protocol: config.ProtocolNZB, ActiveProvider: "usenet"}
	if !nntp.IsNNTPNZB() {
		t.Fatal("expected NNTP NZB")
	}

	debrid := &storage.Entry{Protocol: config.ProtocolNZB, ActiveProvider: "torbox"}
	if debrid.IsNNTPNZB() {
		t.Fatal("expected debrid-backed NZB")
	}
}
