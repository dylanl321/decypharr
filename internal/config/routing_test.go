package config_test

import (
	"testing"

	"github.com/sirrobot01/decypharr/internal/config"
)

func TestDebrid_AllowsTorrents_DefaultTrue(t *testing.T) {
	d := config.Debrid{Provider: "realdebrid"}
	if !d.AllowsTorrents() {
		t.Fatal("AllowsTorrents should default to true when AllowTorrents is unset")
	}
}

func TestDebrid_AllowsTorrents_ExplicitFalse(t *testing.T) {
	f := false
	d := config.Debrid{Provider: "torbox", AllowTorrents: &f}
	if d.AllowsTorrents() {
		t.Fatal("AllowsTorrents should respect explicit false")
	}
}

func TestDebrid_AllowsNZBs_DefaultsByProvider(t *testing.T) {
	cases := map[string]bool{
		"torbox":     true,
		"realdebrid": false,
		"alldebrid":  false,
		"debridlink": false,
	}
	for provider, want := range cases {
		got := config.Debrid{Provider: provider}.AllowsNZBs()
		if got != want {
			t.Fatalf("AllowsNZBs default for %s: want %v, got %v", provider, want, got)
		}
	}
}

func TestDebrid_AllowsNZBs_ExplicitOverride(t *testing.T) {
	tr := true
	d := config.Debrid{Provider: "realdebrid", AllowNZBs: &tr}
	if !d.AllowsNZBs() {
		t.Fatal("explicit AllowNZBs=true should override provider default")
	}
}

func TestDebrid_RemovesOnComplete_DefaultsFalse(t *testing.T) {
	d := config.Debrid{Provider: "torbox"}
	if d.RemovesOnComplete() {
		t.Fatal("RemovesOnComplete should default to false")
	}
}

func TestCleanupOnComplete_EnabledForAction(t *testing.T) {
	tr := true
	c := config.CleanupOnComplete{RemoveFromProvider: &tr}
	if !c.EnabledForAction(config.DownloadActionDownload) {
		t.Fatal("download action should be enabled by default")
	}
	if c.EnabledForAction(config.DownloadActionSymlink) {
		t.Fatal("symlink action should be excluded by default to keep mounts alive")
	}
}

func TestCleanupOnComplete_NoFlagsDisabled(t *testing.T) {
	c := config.CleanupOnComplete{}
	if c.EnabledForAction(config.DownloadActionDownload) {
		t.Fatal("cleanup should be disabled when no flags are set")
	}
}

func TestCleanupOnComplete_CustomActions(t *testing.T) {
	tr := true
	c := config.CleanupOnComplete{
		RemoveFromQueue: &tr,
		Actions:         []config.DownloadAction{config.DownloadActionSymlink},
	}
	if !c.EnabledForAction(config.DownloadActionSymlink) {
		t.Fatal("explicit symlink action should be honored")
	}
	if c.EnabledForAction(config.DownloadActionDownload) {
		t.Fatal("download action should not be enabled when not in custom list")
	}
}
