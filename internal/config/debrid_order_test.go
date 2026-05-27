package config

import "testing"

func TestOrderDebridsForTorrents_PreferCheckboxes(t *testing.T) {
	t1 := true
	debrids := []Debrid{
		{Name: "torbox"},
		{Name: "realdebrid", PreferTorrents: &t1},
	}
	ordered := OrderDebridsForTorrents(debrids, nil)
	if len(ordered) != 2 || ordered[0].Name != "realdebrid" {
		t.Fatalf("expected realdebrid first, got %+v", ordered)
	}
}

func TestOrderDebridsForTorrents_ExplicitOrderWins(t *testing.T) {
	t1 := true
	debrids := []Debrid{
		{Name: "torbox", PreferTorrents: &t1},
		{Name: "realdebrid"},
	}
	ordered := OrderDebridsForTorrents(debrids, []string{"torbox", "realdebrid"})
	if ordered[0].Name != "torbox" {
		t.Fatalf("explicit order should win, got %s", ordered[0].Name)
	}
}
