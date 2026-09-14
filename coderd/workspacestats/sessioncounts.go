package workspacestats

import (
	"cmp"
	"maps"
	"slices"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
)

// deprecatedSessionCounts returns the fixed session_count_* fields keyed by
// normalized app name. Agents predating API v2.11 set these instead of
// session_counts, never both.
func deprecatedSessionCounts(st *agentproto.Stats) map[string]int64 {
	return map[string]int64{
		string(codersdk.AppFamilyVSCode):          st.SessionCountVscode,
		string(codersdk.AppFamilyJetBrains):       st.SessionCountJetbrains,
		string(codersdk.AppFamilyReconnectingPTY): st.SessionCountReconnectingPty,
		string(codersdk.AppFamilySSH):             st.SessionCountSsh,
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
			counts[codersdk.NormalizeAppName(app)] += count
		}
	}
	return counts
}

// maxSessionCountEntries bounds distinct app names per stats report. Overflow
// aggregates under AppFamilyUnknown.
const maxSessionCountEntries = 64

// capSessionCounts keeps the busiest maxSessionCountEntries names, preferring
// known apps, and sums the rest into AppFamilyUnknown, so the result can hold
// one name past the cap.
func capSessionCounts(counts map[string]int64) map[string]int64 {
	if len(counts) <= maxSessionCountEntries {
		return counts
	}
	// Known apps rank 0, unknown 1, so known apps win the cap.
	rank := func(name string) int {
		if codersdk.AppNameFamily(name) == codersdk.AppFamilyUnknown {
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
	capped := make(map[string]int64, maxSessionCountEntries+1)
	for _, name := range ranked[:maxSessionCountEntries] {
		capped[name] = counts[name]
	}
	for _, name := range ranked[maxSessionCountEntries:] {
		capped[string(codersdk.AppFamilyUnknown)] += counts[name]
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
	st.SessionCountVscode = 0
	st.SessionCountJetbrains = 0
	st.SessionCountReconnectingPty = 0
	st.SessionCountSsh = 0
}
