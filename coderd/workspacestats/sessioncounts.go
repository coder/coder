package workspacestats

import (
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
		if count <= 0 {
			continue
		}
		name := codersdk.NormalizeAppName(app)
		// Reserved for codersdk.FoldUnregisteredSessionApps.
		if name == codersdk.AppNameOverflow {
			name = string(codersdk.AppFamilyUnknown)
		}
		counts[name] += count
	}
	return counts
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
