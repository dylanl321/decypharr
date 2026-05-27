package config

// ApplyDebridNameOrder puts names in preferred first (listed order), then remaining debrids in config order.
func ApplyDebridNameOrder(debrids []Debrid, preferred []string) []Debrid {
	if len(preferred) == 0 || len(debrids) == 0 {
		return debrids
	}
	byName := make(map[string]Debrid, len(debrids))
	for _, d := range debrids {
		byName[d.Name] = d
	}
	seen := make(map[string]bool, len(debrids))
	out := make([]Debrid, 0, len(debrids))
	for _, name := range preferred {
		if d, ok := byName[name]; ok && !seen[name] {
			out = append(out, d)
			seen[name] = true
		}
	}
	for _, d := range debrids {
		if !seen[d.Name] {
			out = append(out, d)
			seen[d.Name] = true
		}
	}
	return out
}

// ApplyPreferFirstOrder moves debrids with prefer==true to the front (config order within each group).
func ApplyPreferFirstOrder(debrids []Debrid, prefer func(Debrid) bool) []Debrid {
	if len(debrids) == 0 || prefer == nil {
		return debrids
	}
	first := make([]Debrid, 0, len(debrids))
	rest := make([]Debrid, 0, len(debrids))
	for _, d := range debrids {
		if prefer(d) {
			first = append(first, d)
		} else {
			rest = append(rest, d)
		}
	}
	if len(first) == 0 {
		return debrids
	}
	return append(first, rest...)
}

func anyPrefers(debrids []Debrid, prefer func(Debrid) bool) bool {
	for _, d := range debrids {
		if prefer(d) {
			return true
		}
	}
	return false
}

// OrderDebridsForTorrents returns debrids in submit try-order for torrents.
// Explicit torrent_debrid_order wins; otherwise prefer_torrents checkboxes; else config order.
func OrderDebridsForTorrents(debrids []Debrid, nameOrder []string) []Debrid {
	if len(nameOrder) > 0 {
		return ApplyDebridNameOrder(debrids, nameOrder)
	}
	if anyPrefers(debrids, func(d Debrid) bool { return d.PrefersTorrents() }) {
		return ApplyPreferFirstOrder(debrids, func(d Debrid) bool { return d.PrefersTorrents() })
	}
	return debrids
}

// OrderDebridsForNZBs is the NZB analogue of OrderDebridsForTorrents.
func OrderDebridsForNZBs(debrids []Debrid, nameOrder []string) []Debrid {
	if len(nameOrder) > 0 {
		return ApplyDebridNameOrder(debrids, nameOrder)
	}
	if anyPrefers(debrids, func(d Debrid) bool { return d.PrefersNZBs() }) {
		return ApplyPreferFirstOrder(debrids, func(d Debrid) bool { return d.PrefersNZBs() })
	}
	return debrids
}
