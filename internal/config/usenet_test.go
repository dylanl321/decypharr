package config_test

import (
	"testing"

	"github.com/sirrobot01/decypharr/internal/config"
)

func TestValidateUsenetConfig_DebridBackend(t *testing.T) {
	usenet := config.Usenet{
		Backend: config.UsenetBackendDebrid,
		Debrid:  "torbox",
	}
	debrids := []config.Debrid{
		{Name: "torbox", Provider: "torbox", APIKey: "key"},
	}
	if err := config.ValidateUsenetConfig(usenet, debrids); err != nil {
		t.Fatalf("expected valid debrid backend config, got %v", err)
	}
}

func TestValidateUsenetConfig_DebridRequiresTorbox(t *testing.T) {
	usenet := config.Usenet{
		Backend: config.UsenetBackendDebrid,
		Debrid:  "rd",
	}
	debrids := []config.Debrid{
		{Name: "rd", Provider: "realdebrid", APIKey: "key"},
	}
	if err := config.ValidateUsenetConfig(usenet, debrids); err == nil {
		t.Fatal("expected error for non-NZB-capable debrid")
	}
}

func TestValidateUsenetConfig_NNTPWithProviders(t *testing.T) {
	usenet := config.Usenet{
		Backend: config.UsenetBackendNNTP,
		Providers: []config.UsenetProvider{
			{Host: "news.example.com", Username: "user", Password: "pass"},
		},
	}
	if err := config.ValidateUsenetConfig(usenet, nil); err != nil {
		t.Fatalf("expected valid nntp config, got %v", err)
	}
}

func TestValidateUsenetConfig_TorrentOnlyNoProviders(t *testing.T) {
	usenet := config.Usenet{Backend: config.UsenetBackendNNTP}
	debrids := []config.Debrid{{Name: "torbox", Provider: "torbox", APIKey: "key"}}
	if err := config.ValidateUsenetConfig(usenet, debrids); err != nil {
		t.Fatalf("torrent-only install should remain valid, got %v", err)
	}
}
