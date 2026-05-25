package config

import (
	"path/filepath"
	"strings"
)

// ResolveCategoryPath returns the save path for a category using category_paths when configured.
func ResolveCategoryPath(category, downloadFolder, arrName string) string {
	cfg := Get()
	if cfg != nil && len(cfg.CategoryPaths) > 0 {
		key := strings.ToLower(strings.TrimSpace(category))
		for k, v := range cfg.CategoryPaths {
			if strings.EqualFold(k, category) || strings.ToLower(k) == key {
				if v != "" {
					return v
				}
			}
		}
	}
	if downloadFolder == "" {
		return arrName
	}
	return filepath.Join(downloadFolder, arrName)
}
