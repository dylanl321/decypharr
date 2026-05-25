package manager

import (
	"strings"
	"testing"

	"github.com/sirrobot01/decypharr/pkg/storage"
)

func TestCategoryFilter_UsesEqualFold(t *testing.T) {
	entry := &storage.Entry{Category: "Sonarr", InfoHash: "abc"}
	category := "sonarr"
	filter := func(t *storage.Entry) bool {
		if category != "" && !strings.EqualFold(t.Category, category) {
			return false
		}
		return true
	}
	if !filter(entry) {
		t.Fatal("Sonarr entry should match sonarr category filter")
	}
	if filter(&storage.Entry{Category: "radarr"}) {
		t.Fatal("radarr entry should not match sonarr category filter")
	}
}
