package telemetry_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/testutil"
)

func TestConvertWorkspaceBuild_AgentRelay(t *testing.T) {
	t.Parallel()

	hash := func(s string) string {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
	}

	tests := []struct {
		name   string
		params map[string]string
		want   *telemetry.WorkspaceBuildAgentRelay
	}{
		{
			name:   "NoParameters",
			params: nil,
			want:   nil,
		},
		{
			name: "NotRelay",
			params: map[string]string{
				"region":           "us-east",
				"agent_relay_pool": "pool",
			},
			want: nil,
		},
		{
			name: "ClaudeCode",
			params: map[string]string{
				"agent_relay_session_id":                  "session-1",
				"agent_relay_pool":                        "claude-pool",
				"agent_relay_attempt":                     "2",
				"agent_relay_client_platform":             "claude_in_slack",
				"agent_relay_claude_code_lock_to_account": "true",
			},
			want: &telemetry.WorkspaceBuildAgentRelay{
				Provider:       telemetry.AgentRelayProviderClaudeCode,
				ClientPlatform: "claude_in_slack",
				SessionIDHash:  hash("session-1"),
				PoolHash:       hash("claude-pool"),
				Attempt:        ptr.Ref(2),
			},
		},
		{
			// Attempt 0 is the first dispatch and must stay distinct from a
			// missing attempt.
			name: "AttemptZero",
			params: map[string]string{
				"agent_relay_session_id": "session-1",
				"agent_relay_attempt":    "0",
			},
			want: &telemetry.WorkspaceBuildAgentRelay{
				Provider:      telemetry.AgentRelayProviderUnknown,
				SessionIDHash: hash("session-1"),
				Attempt:       ptr.Ref(0),
			},
		},
		{
			// A template default stores an empty session ID on builds a
			// person creates by hand.
			name: "EmptySessionID",
			params: map[string]string{
				"agent_relay_session_id": "",
				"agent_relay_pool":       "pool",
			},
			want: nil,
		},
		{
			name: "ClientPlatformNormalized",
			params: map[string]string{
				"agent_relay_session_id":      "session-1",
				"agent_relay_client_platform": "Claude-In-Slack <script>",
			},
			want: &telemetry.WorkspaceBuildAgentRelay{
				Provider:       telemetry.AgentRelayProviderUnknown,
				ClientPlatform: "claude_in_slack",
				SessionIDHash:  hash("session-1"),
			},
		},
		{
			// Stop and restart builds keep the ephemeral parameter's row
			// with its default value.
			name: "ClaudeCodeRestart",
			params: map[string]string{
				"agent_relay_session_id":                  "session-1",
				"agent_relay_attempt":                     "",
				"agent_relay_claude_code_lock_to_account": "",
			},
			want: &telemetry.WorkspaceBuildAgentRelay{
				Provider:      telemetry.AgentRelayProviderClaudeCode,
				SessionIDHash: hash("session-1"),
			},
		},
		{
			// The client platform alone does not identify a provider.
			name: "UnknownWithClientPlatform",
			params: map[string]string{
				"agent_relay_session_id":      "session-1",
				"agent_relay_client_platform": "web_claude_ai",
			},
			want: &telemetry.WorkspaceBuildAgentRelay{
				Provider:       telemetry.AgentRelayProviderUnknown,
				ClientPlatform: "web_claude_ai",
				SessionIDHash:  hash("session-1"),
			},
		},
		{
			name: "Ambiguous",
			params: map[string]string{
				"agent_relay_session_id":                  "session-4",
				"agent_relay_claude_code_lock_to_account": "true",
				"agent_relay_cursor_pool_name":            "pool",
			},
			want: &telemetry.WorkspaceBuildAgentRelay{
				Provider:      telemetry.AgentRelayProviderAmbiguous,
				SessionIDHash: hash("session-4"),
			},
		},
		{
			name: "Cursor",
			params: map[string]string{
				"agent_relay_session_id":       "session-2",
				"agent_relay_pool":             "cursor-pool",
				"agent_relay_cursor_pool_name": "cursor-pool",
			},
			want: &telemetry.WorkspaceBuildAgentRelay{
				Provider:      telemetry.AgentRelayProviderCursor,
				SessionIDHash: hash("session-2"),
				PoolHash:      hash("cursor-pool"),
			},
		},
		{
			name: "UnknownProviderInvalidAttempt",
			params: map[string]string{
				"agent_relay_session_id": "session-3",
				"agent_relay_attempt":    "not-a-number",
			},
			want: &telemetry.WorkspaceBuildAgentRelay{
				Provider:      telemetry.AgentRelayProviderUnknown,
				SessionIDHash: hash("session-3"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := telemetry.ConvertWorkspaceBuild(database.WorkspaceBuild{}, tt.params)
			require.Equal(t, tt.want, got.AgentRelay)
		})
	}
}

func TestTelemetry_WorkspaceBuildAgentRelay(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})

	relayBuild := func(transition database.WorkspaceTransition) uuid.UUID {
		return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
		}).Seed(database.WorkspaceBuild{Transition: transition}).Params(
			database.WorkspaceBuildParameter{Name: "agent_relay_session_id", Value: "session-1"},
			database.WorkspaceBuildParameter{Name: "agent_relay_pool", Value: "pool"},
			database.WorkspaceBuildParameter{Name: "agent_relay_cursor_pool_name", Value: "pool"},
			database.WorkspaceBuildParameter{Name: "region", Value: "us-east"},
		).Do().Build.ID
	}
	startBuild := relayBuild(database.WorkspaceTransitionStart)
	// Stop and delete builds keep the relay parameters but are not dispatches.
	stopBuild := relayBuild(database.WorkspaceTransitionStop)
	deleteBuild := relayBuild(database.WorkspaceTransitionDelete)
	plainBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
	}).Params(
		database.WorkspaceBuildParameter{Name: "region", Value: "us-east"},
	).Do().Build.ID

	_, snapshot := collectSnapshot(ctx, t, db, nil)

	builds := make(map[uuid.UUID]telemetry.WorkspaceBuild, len(snapshot.WorkspaceBuilds))
	for _, build := range snapshot.WorkspaceBuilds {
		builds[build.ID] = build
	}
	require.Contains(t, builds, startBuild)
	require.Equal(t, &telemetry.WorkspaceBuildAgentRelay{
		Provider:      telemetry.AgentRelayProviderCursor,
		SessionIDHash: fmt.Sprintf("%x", sha256.Sum256([]byte("session-1"))),
		PoolHash:      fmt.Sprintf("%x", sha256.Sum256([]byte("pool"))),
	}, builds[startBuild].AgentRelay)
	for _, id := range []uuid.UUID{stopBuild, deleteBuild, plainBuild} {
		require.Contains(t, builds, id)
		require.Nil(t, builds[id].AgentRelay)
	}
}
