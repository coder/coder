package workspacestats

import (
	"cmp"
	"maps"
	"slices"

	"github.com/google/uuid"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
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

// activitySources returns active apps, busiest first, or the reporting connection.
func activitySources(st *agentproto.Stats, agentID uuid.UUID) []database.WorkspaceBuildDeadlineSource {
	counts := normalizedSessionCounts(st)
	codersdk.FoldUnregisteredSessionApps(counts)
	if len(counts) == 0 {
		return []database.WorkspaceBuildDeadlineSource{{Type: codersdk.WorkspaceBuildDeadlineSourceTypeConnection, AgentID: agentID}}
	}
	names := slices.SortedFunc(maps.Keys(counts), func(a, b string) int {
		return cmp.Or(
			cmp.Compare(counts[b], counts[a]),
			cmp.Compare(a, b),
		)
	})
	sources := make([]database.WorkspaceBuildDeadlineSource, 0, len(names))
	for _, name := range names {
		sources = append(sources, database.WorkspaceBuildDeadlineSource{
			Type:    codersdk.WorkspaceBuildDeadlineSourceTypeSessionApp,
			AppName: name,
			AgentID: agentID,
		})
	}
	return sources
}

// ClearSessionCounts zeroes every session count on the given stats.
func ClearSessionCounts(st *agentproto.Stats) {
	st.SessionCounts = nil
	st.SessionCountVscode = 0
	st.SessionCountJetbrains = 0
	st.SessionCountReconnectingPty = 0
	st.SessionCountSsh = 0
}
