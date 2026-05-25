package arr

import "testing"

func TestStorage_Get_CaseInsensitive(t *testing.T) {
	s := NewStorage()
	s.AddOrUpdate(New("Sonarr", "http://sonarr.local", "token", false, false, nil, "", "config"))

	if got := s.Get("sonarr"); got == nil {
		t.Fatal("expected Get(sonarr) to match Sonarr")
	}
	if got := s.Get("SONARR"); got == nil {
		t.Fatal("expected Get(SONARR) to match Sonarr")
	}
	if got := s.Get("Sonarr"); got == nil {
		t.Fatal("expected exact Get(Sonarr) to match")
	}
	if got := s.Get("radarr"); got != nil {
		t.Fatal("expected Get(radarr) to miss when only Sonarr configured")
	}
}

func TestStorage_GetOrCreate_CaseInsensitive(t *testing.T) {
	s := NewStorage()
	s.AddOrUpdate(New("Radarr", "http://radarr.local", "token", false, false, nil, "", "config"))

	a := s.GetOrCreate("radarr")
	if a == nil || a.Name != "Radarr" {
		t.Fatalf("GetOrCreate(radarr) = %+v, want Radarr entry", a)
	}
}
