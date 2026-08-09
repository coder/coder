package workspacestats

import (
	"cmp"
	"maps"
	"slices"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/idemetadata"
)

// normalizedSessionCounts returns an agent's per-app session counts,
// normalized, with non-positive counts dropped. One agent fills either the map
// (API >= 2.11) or the deprecated fields, so those are a fallback.
func normalizedSessionCounts(st *agentproto.Stats) map[string]int64 {
	counts := make(map[string]int64, len(st.GetSessionCounts()))
	for app, count := range st.GetSessionCounts() {
		if count > 0 {
			counts[idemetadata.Normalize(app)] += count
		}
	}
	if len(counts) == 0 {
		//nolint:staticcheck // Deprecated fields are read for old-agent compatibility.
		for _, d := range [...]struct {
			app   string
			count int64
		}{
			{idemetadata.AppNameVSCode, st.SessionCountVscode},
			{idemetadata.AppNameJetBrains, st.SessionCountJetbrains},
			{idemetadata.AppNameReconnectingPTY, st.SessionCountReconnectingPty},
			{idemetadata.AppNameSSH, st.SessionCountSsh},
		} {
			if d.count > 0 {
				counts[d.app] = d.count
			}
		}
	}
	return counts
}

// capSessionCounts keeps at most idemetadata.MaxSessionCountEntries entries and
// sums the rest into AppNameUnknown, which can add one past the cap. Known
// families always survive; the rest fill the room busiest first.
func capSessionCounts(counts map[string]int64) map[string]int64 {
	if len(counts) <= idemetadata.MaxSessionCountEntries {
		return counts
	}
	capped := make(map[string]int64, idemetadata.MaxSessionCountEntries+1)
	for name, count := range counts {
		if idemetadata.Family(name) != idemetadata.AppNameUnknown {
			capped[name] = count
		}
	}
	byCount := slices.SortedFunc(maps.Keys(counts), func(a, b string) int {
		return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b))
	})
	var overflow int64
	for _, name := range byCount {
		if _, kept := capped[name]; kept {
			continue
		}
		if len(capped) < idemetadata.MaxSessionCountEntries {
			capped[name] = counts[name]
			continue
		}
		overflow += counts[name]
	}
	if overflow > 0 {
		capped[idemetadata.AppNameUnknown] += overflow
	}
	return capped
}

// HasSessionCounts reports whether the stats contain any active session,
// including the deprecated fields. Normalizing cannot change a count's sign.
func HasSessionCounts(st *agentproto.Stats) bool {
	for _, count := range st.GetSessionCounts() {
		if count > 0 {
			return true
		}
	}
	//nolint:staticcheck // Deprecated fields are read for old-agent compatibility.
	return st.SessionCountVscode > 0 || st.SessionCountJetbrains > 0 ||
		st.SessionCountReconnectingPty > 0 || st.SessionCountSsh > 0
}

// ClearSessionCounts zeroes all session counts on the given stats, including
// the deprecated fields sent by older agents.
func ClearSessionCounts(st *agentproto.Stats) {
	st.SessionCounts = nil
	//nolint:staticcheck // Deprecated fields are cleared for old-agent compatibility.
	st.SessionCountSsh, st.SessionCountJetbrains, st.SessionCountVscode, st.SessionCountReconnectingPty = 0, 0, 0, 0
}
