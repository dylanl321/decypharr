package config

import (
	"fmt"
	"strconv"
	"strings"
)

// DownloadConfig holds settings that influence how local downloads are
// performed (separate from per-debrid behavior). Today this only covers
// bandwidth shaping but is grouped together so future tunables (concurrency
// caps, retry windows, etc.) have a natural home.
type DownloadConfig struct {
	// BandwidthLimit caps the aggregate write rate across all active local
	// downloads, regardless of which provider they came from. Empty/zero
	// disables the cap. See ParseBandwidth for the accepted suffixes.
	BandwidthLimit string `json:"bandwidth_limit,omitempty"`
}

// IsZero is used by encoding/json's omitzero to drop empty Download blocks
// from on-disk config.
func (d DownloadConfig) IsZero() bool { return d.BandwidthLimit == "" }

// BandwidthLimitBytes returns the configured bandwidth cap in bytes/sec or
// 0 when the cap is unset/invalid. Errors are intentionally swallowed so a
// malformed value never blocks startup; validation happens via ValidateBandwidth.
func (d DownloadConfig) BandwidthLimitBytes() int64 {
	v, _ := ParseBandwidth(d.BandwidthLimit)
	return v
}

// ParseBandwidth converts a human-friendly bandwidth string into bytes/sec.
// Accepts:
//   - "10MB/s" / "10MB" / "10mbps" (decimal, MB = 1_000_000)
//   - "10MiB/s"                    (binary, MiB = 1_048_576)
//   - "1.5GB/s"
//   - raw integer strings (interpreted as bytes/sec)
//   - "" or "0" -> (0, nil) meaning "no limit"
//
// The parse is case-insensitive and tolerates the trailing "/s" or "ps".
func ParseBandwidth(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	raw := strings.ToLower(s)
	// Strip trailing per-second suffixes: "/s", "ps", "/sec".
	for _, suffix := range []string{"/sec", "/s", "ps"} {
		if strings.HasSuffix(raw, suffix) {
			raw = strings.TrimSuffix(raw, suffix)
			break
		}
	}
	raw = strings.TrimSpace(raw)

	// Find split between numeric prefix and unit suffix.
	splitAt := len(raw)
	for i, ch := range raw {
		if (ch >= '0' && ch <= '9') || ch == '.' {
			continue
		}
		splitAt = i
		break
	}
	numPart := strings.TrimSpace(raw[:splitAt])
	unit := strings.TrimSpace(raw[splitAt:])
	if numPart == "" {
		return 0, fmt.Errorf("bandwidth %q: missing numeric value", s)
	}
	val, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, fmt.Errorf("bandwidth %q: %w", s, err)
	}
	if val < 0 {
		return 0, fmt.Errorf("bandwidth %q: must be non-negative", s)
	}

	mult := float64(1)
	switch unit {
	case "", "b":
		mult = 1
	case "k", "kb":
		mult = 1_000
	case "ki", "kib":
		mult = 1 << 10
	case "m", "mb":
		mult = 1_000_000
	case "mi", "mib":
		mult = 1 << 20
	case "g", "gb":
		mult = 1_000_000_000
	case "gi", "gib":
		mult = 1 << 30
	case "t", "tb":
		mult = 1_000_000_000_000
	case "ti", "tib":
		mult = 1 << 40
	default:
		return 0, fmt.Errorf("bandwidth %q: unknown unit %q", s, unit)
	}
	return int64(val * mult), nil
}

// ValidateBandwidth returns an error when a bandwidth string is non-empty
// and unparseable. Used by config validation passes.
func ValidateBandwidth(field, value string) error {
	if value == "" {
		return nil
	}
	if _, err := ParseBandwidth(value); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}
