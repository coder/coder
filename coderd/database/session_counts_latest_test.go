package database_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

// GetWorkspaceAgentStats and GetWorkspaceAgentStatsAndLabels report the
// sessions of an agent's latest row only, while their byte and latency
// aggregates cover every row in the window. These tests pin that split: the
// per-app object must equal the latest row's session_counts exactly, with no
// contribution from older rows and no row multiplication in the aggregates
// beside it.

// sessionCountsByApp decodes the per-app session count object a session count
// query returns. Comparing the decoded map rather than the raw bytes keeps the
// assertion about the counts instead of about jsonb key ordering.
func sessionCountsByApp(t *testing.T, data json.RawMessage) map[string]int64 {
	t.Helper()

	counts := map[string]int64{}
	if len(data) > 0 {
		require.NoError(t, json.Unmarshal(data, &counts))
	}
	return counts
}

func TestGetWorkspaceAgentStatsLatestRowSessions(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()
	now := dbtime.Now()

	// The query groups by the owner columns, so every row of an agent must
	// share them.
	newAgent := func() database.WorkspaceAgentStat {
		return database.WorkspaceAgentStat{
			UserID:      uuid.New(),
			AgentID:     uuid.New(),
			WorkspaceID: uuid.New(),
			TemplateID:  uuid.New(),
		}
	}
	insert := func(owner database.WorkspaceAgentStat, createdAt time.Time, counts map[string]int64) {
		stat := owner
		stat.CreatedAt = createdAt
		stat.RxBytes = 10
		stat.TxBytes = 1
		// Rows only reach the byte and latency aggregates with a latency
		// above zero.
		stat.ConnectionMedianLatencyMS = 5
		stat.SessionCounts = dbgen.SessionCounts(t, counts)
		dbgen.WorkspaceAgentStat(t, db, stat)
	}

	// Superseded: the latest row replaces the older row's apps entirely, and
	// names the app family registry does not know are reported all the same.
	superseded := newAgent()
	insert(superseded, now.Add(-2*time.Minute), map[string]int64{"vscode": 2, "ssh": 1})
	insert(superseded, now.Add(-time.Minute), map[string]int64{"jetbrains": 1, "some_new_ide": 2})

	// Idle: the latest row reports no sessions, so the agent keeps its row
	// with no sessions rather than disappearing or inheriting the older row's.
	idle := newAgent()
	insert(idle, now.Add(-2*time.Minute), map[string]int64{"ssh": 3})
	insert(idle, now.Add(-time.Minute), nil)

	// Family: two apps of one family in a single row stay separate per app and
	// add up per family, which is what the metrics and telemetry paths report.
	family := newAgent()
	insert(family, now.Add(-time.Minute), map[string]int64{"cursor": 1, "vscode": 2})

	stats, err := db.GetWorkspaceAgentStats(ctx, now.Add(-time.Hour))
	require.NoError(t, err)

	byAgent := map[uuid.UUID]database.GetWorkspaceAgentStatsRow{}
	for _, stat := range stats {
		byAgent[stat.AgentID] = stat
	}
	require.Len(t, byAgent, 3)

	supersededRow := byAgent[superseded.AgentID]
	require.Equal(t, map[string]int64{"jetbrains": 1, "some_new_ide": 2},
		sessionCountsByApp(t, supersededRow.SessionCounts),
		"only the latest row's sessions count")
	// Both rows still reach the byte aggregates, which is what catches a
	// decomposed session row multiplying them.
	require.Equal(t, int64(20), supersededRow.WorkspaceRxBytes)
	require.Equal(t, int64(2), supersededRow.WorkspaceTxBytes)
	require.Equal(t, superseded.AgentID, supersededRow.AgentID_2,
		"the repeated agent_id column keeps the row layout telemetry converts between")

	idleRow := byAgent[idle.AgentID]
	require.Empty(t, sessionCountsByApp(t, idleRow.SessionCounts),
		"the previous row's sessions must not be resurrected")
	require.Equal(t, int64(20), idleRow.WorkspaceRxBytes)

	familyRow := byAgent[family.AgentID]
	require.Equal(t, map[string]int64{"cursor": 1, "vscode": 2},
		sessionCountsByApp(t, familyRow.SessionCounts))
	require.Equal(t, int64(3),
		sessionFamilyCounts(t, familyRow.SessionCounts)[codersdk.AppFamilyVSCode],
		"apps of one family add up per family")
}

func TestGetWorkspaceAgentStatsAndLabelsLatestRowSessions(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := context.Background()
	now := dbtime.Now()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})

	// newAgent returns the owner columns of an agent that the query can join
	// to a user, a workspace, and a workspace agent.
	newAgent := func() (database.WorkspaceAgentStat, database.WorkspaceAgent, database.WorkspaceTable) {
		job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
		})
		resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
			JobID: job.ID,
		})
		agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     template.ID,
		})
		return database.WorkspaceAgentStat{
			UserID:      user.ID,
			AgentID:     agent.ID,
			WorkspaceID: workspace.ID,
			TemplateID:  template.ID,
		}, agent, workspace
	}
	insert := func(owner database.WorkspaceAgentStat, createdAt time.Time, latencyMS float64, connectionCount int64, counts map[string]int64) {
		stat := owner
		stat.CreatedAt = createdAt
		stat.RxBytes = 4
		stat.TxBytes = 2
		// The latest row is the latest one reporting a latency above zero.
		stat.ConnectionMedianLatencyMS = latencyMS
		stat.ConnectionCount = connectionCount
		stat.SessionCounts = dbgen.SessionCounts(t, counts)
		dbgen.WorkspaceAgentStat(t, db, stat)
	}

	superseded, supersededAgent, supersededWorkspace := newAgent()
	insert(superseded, now.Add(-2*time.Minute), 5, 1, map[string]int64{"ssh": 1})
	insert(superseded, now.Add(-time.Minute), 7, 3, map[string]int64{"cursor": 1, "vscode": 2})
	// Newest of all, but a latency of zero keeps it out of the latest row
	// selection, so its sessions and connection count must not be reported.
	// Legacy agents report no latency, and the byte aggregates still take it.
	insert(superseded, now.Add(-30*time.Second), 0, 99, map[string]int64{"jetbrains": 5})

	idle, idleAgent, _ := newAgent()
	insert(idle, now.Add(-2*time.Minute), 5, 1, map[string]int64{"ssh": 9})
	insert(idle, now.Add(-time.Minute), 5, 2, nil)

	stats, err := db.GetWorkspaceAgentStatsAndLabels(ctx, now.Add(-time.Hour))
	require.NoError(t, err)

	byAgentName := map[string]database.GetWorkspaceAgentStatsAndLabelsRow{}
	for _, stat := range stats {
		byAgentName[stat.AgentName] = stat
	}
	require.Len(t, byAgentName, 2)

	supersededRow := byAgentName[supersededAgent.Name]
	require.Equal(t, user.Username, supersededRow.Username)
	require.Equal(t, supersededWorkspace.Name, supersededRow.WorkspaceName)
	require.Equal(t, map[string]int64{"cursor": 1, "vscode": 2},
		sessionCountsByApp(t, supersededRow.SessionCounts),
		"only the latest row reporting a latency counts")
	require.Equal(t, int64(3), supersededRow.ConnectionCount,
		"the connection count comes from that row alone")
	require.Equal(t, float64(7), supersededRow.ConnectionMedianLatencyMS)
	// Every row in the window still reaches the byte aggregates, the zero
	// latency one included.
	require.Equal(t, int64(12), supersededRow.RxBytes)
	require.Equal(t, int64(6), supersededRow.TxBytes)
	require.Equal(t, int64(3),
		sessionFamilyCounts(t, supersededRow.SessionCounts)[codersdk.AppFamilyVSCode],
		"apps of one family add up per family")

	idleRow := byAgentName[idleAgent.Name]
	require.Empty(t, sessionCountsByApp(t, idleRow.SessionCounts),
		"the previous row's sessions must not be resurrected")
	require.Equal(t, int64(2), idleRow.ConnectionCount)
	require.Equal(t, int64(8), idleRow.RxBytes)
}
