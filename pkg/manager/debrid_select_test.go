package manager

import "testing"

func TestIsProviderBlocked(t *testing.T) {
	if !isProviderBlocked("RealDebrid", []string{"realdebrid"}) {
		t.Fatal("expected realdebrid to be blocked")
	}
	if isProviderBlocked("torbox", []string{"realdebrid"}) {
		t.Fatal("expected torbox to remain eligible")
	}
}

func TestFilterBlockedNZBClients(t *testing.T) {
	items := []namedNZBClient{
		{name: "realdebrid"},
		{name: "torbox"},
	}
	filtered := filterBlockedNZBClients(items, []string{"RealDebrid"})
	if len(filtered) != 1 || filtered[0].name != "torbox" {
		t.Fatalf("unexpected filter result: %+v", filtered)
	}
}
