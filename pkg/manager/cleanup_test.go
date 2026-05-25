package manager

import (
	"testing"

	"github.com/sirrobot01/decypharr/internal/config"
)

func TestActionAllowedForCleanup_DefaultDownloadOnly(t *testing.T) {
	c := config.CleanupOnComplete{}
	if !actionAllowedForCleanup(c, config.DownloadActionDownload) {
		t.Fatal("default cleanup actions should include `download`")
	}
	for _, a := range []config.DownloadAction{config.DownloadActionSymlink, config.DownloadActionStrm, config.DownloadActionNone} {
		if actionAllowedForCleanup(c, a) {
			t.Fatalf("default cleanup actions should exclude %q to preserve mount/stream copies", a)
		}
	}
}

func TestActionAllowedForCleanup_ExplicitList(t *testing.T) {
	c := config.CleanupOnComplete{Actions: []config.DownloadAction{config.DownloadActionSymlink}}
	if !actionAllowedForCleanup(c, config.DownloadActionSymlink) {
		t.Fatal("explicit symlink action should be honored")
	}
	if actionAllowedForCleanup(c, config.DownloadActionDownload) {
		t.Fatal("download action should be excluded when not in explicit list")
	}
}

func TestFindDebridConfig(t *testing.T) {
	cfg := &config.Config{Debrids: []config.Debrid{
		{Name: "rd", Provider: "realdebrid"},
		{Name: "tb", Provider: "torbox"},
	}}
	if got := findDebridConfig(cfg, "tb"); got == nil || got.Provider != "torbox" {
		t.Fatalf("findDebridConfig: expected torbox config, got %+v", got)
	}
	if got := findDebridConfig(cfg, "missing"); got != nil {
		t.Fatalf("findDebridConfig: expected nil for unknown name, got %+v", got)
	}
}
