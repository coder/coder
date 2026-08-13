package workspacestats

import (
	"cmp"
	"maps"
	"slices"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/idemetadata"
)

// deprecatedSessionCounts returns the fixed session_count_* fields keyed by
// canonical app name. Agents predating API v2.11 set these instead of
// session_counts, never both.
func deprecatedSessionCounts(st *agentproto.Stats) map[string]int64 {
	return map[string]int64{
		idemetadata.AppNameVSCode:          st.SessionCountVscode,
		idemetadata.AppNameJetBrains:       st.SessionCountJetbrains,
		idemetadata.AppNameReconnectingPTY: st.SessionCountReconnectingPty,
		idemetadata.AppNameSSH:             st.SessionCountSsh,
	}
}

// normalizedSessionCounts returns per-app session counts, names normalized and
// non-positive counts dropped.
func normalizedSessionCounts(st *agentproto.Stats) map[string]int64 {
	reported := st.GetSessionCounts()
	if len(reported) == 0 {
		reported = deprecatedSessionCounts(st)
	}
	counts := make(map[string]int64, len(reported))
	for app, count := range reported {
		if count > 0 {
			counts[idemetadata.Normalize(app)] += count
		}
	}
	return counts
}

// capSessionCounts keeps the busiest MaxSessionCountEntries names, preferring
// known apps, and sums the rest into AppNameUnknown, so the result can hold one
// name past the cap.
func capSessionCounts(counts map[string]int64) map[string]int64 {
	if len(counts) <= idemetadata.MaxSessionCountEntries {
		return counts
	}
	// Known apps rank 0, unknown 1, so known apps win the cap.
	rank := func(name string) int {
		if idemetadata.Family(name) == idemetadata.AppNameUnknown {
			return 1
		}
		return 0
	}
	ranked := slices.SortedFunc(maps.Keys(counts), func(a, b string) int {
		return cmp.Or(
			cmp.Compare(rank(a), rank(b)),     // known apps first
			cmp.Compare(counts[b], counts[a]), // then busiest
			cmp.Compare(a, b),                 // then name, so the cut is stable
		)
	})
	capped := make(map[string]int64, idemetadata.MaxSessionCountEntries+1)
	for _, name := range ranked[:idemetadata.MaxSessionCountEntries] {
		capped[name] = counts[name]
	}
	for _, name := range ranked[idemetadata.MaxSessionCountEntries:] {
		capped[idemetadata.AppNameUnknown] += counts[name]
	}
	return capped
}

// HasSessionCounts reports whether the stats contain any active session.
func HasSessionCounts(st *agentproto.Stats) bool {
	return len(normalizedSessionCounts(st)) > 0
}

// ClearSessionCounts zeroes every session count on the given stats.
func ClearSessionCounts(st *agentproto.Stats) {
	st.SessionCounts = nil
	st.SessionCountVscode, st.SessionCountJetbrains = 0, 0
	st.SessionCountReconnectingPty, st.SessionCountSsh = 0, 0
}
