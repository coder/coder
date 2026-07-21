package workspacestats

import (
	"maps"
	"slices"
	"strings"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/idemetadata"
)

// SessionCountsFromProto returns an agent's per-app session counts,
// normalized and capped, with non-positive counts dropped.
func SessionCountsFromProto(st *agentproto.Stats) map[string]int64 {
	return capSessionCounts(normalizedSessionCounts(st))
}

// normalizedSessionCounts is SessionCountsFromProto without the entry cap,
// so callers can observe overflow. Deprecated fixed fields are converted
// for agents predating the session_counts map (API < 2.11).
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

// capSessionCounts keeps at most idemetadata.MaxSessionCountEntries named
// entries, well-known names first then lexicographic for determinism; the
// rest sums into AppNameUnknown, which can add one entry past the cap.
func capSessionCounts(counts map[string]int64) map[string]int64 {
	if len(counts) <= idemetadata.MaxSessionCountEntries {
		return counts
	}
	names := slices.Collect(maps.Keys(counts))
	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[name] = idemetadata.Family(name) != idemetadata.AppNameUnknown
	}
	slices.SortFunc(names, func(a, b string) int {
		if known[a] != known[b] {
			if known[a] {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	capped := make(map[string]int64, idemetadata.MaxSessionCountEntries+1)
	for _, name := range names[:idemetadata.MaxSessionCountEntries] {
		capped[name] = counts[name]
	}
	var overflow int64
	for _, name := range names[idemetadata.MaxSessionCountEntries:] {
		overflow += counts[name]
	}
	if overflow > 0 {
		capped[idemetadata.AppNameUnknown] += overflow
	}
	return capped
}

// HasSessionCounts reports whether the stats contain any active session,
// including the deprecated fixed fields sent by older agents.
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
// the deprecated fixed fields sent by older agents.
func ClearSessionCounts(st *agentproto.Stats) {
	st.SessionCounts = nil
	//nolint:staticcheck // Deprecated fields are cleared for old-agent compatibility.
	st.SessionCountSsh, st.SessionCountJetbrains, st.SessionCountVscode, st.SessionCountReconnectingPty = 0, 0, 0, 0
}
