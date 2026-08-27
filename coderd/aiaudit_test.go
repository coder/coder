package coderd_test

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAIAuditAgents(t *testing.T) {
	t.Parallel()

	t.Run("SponsorScope", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		agent := seedAIAgent(ctx, t, db, member.ID)

		// The caller is the default sponsor.
		agents, err := memberClient.AIAuditAgents(ctx, "")
		require.NoError(t, err)
		require.Len(t, agents, 1)
		require.Equal(t, agent.UserID, agents[0].UserID)
		require.Equal(t, member.ID, agents[0].OwnerUserID)
		require.Equal(t, string(database.AIAgentOriginWorkspace), agents[0].OriginType)
		require.NotEmpty(t, agents[0].Username)

		// "me" and the caller's own username resolve identically without
		// any audit permission.
		agents, err = memberClient.AIAuditAgents(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		agents, err = memberClient.AIAuditAgents(ctx, member.Username)
		require.NoError(t, err)
		require.Len(t, agents, 1)

		// A sponsor with no agents gets an empty list.
		agents, err = client.AIAuditAgents(ctx, "")
		require.NoError(t, err)
		require.Empty(t, agents)

		// Members cannot name another sponsor, existing or not; both are
		// 403 so the parameter cannot probe usernames.
		var sdkErr *codersdk.Error
		_, err = memberClient.AIAuditAgents(ctx, "nonexistent-user")
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("AuditorCrossSponsor", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		auditorClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleAuditor())
		ctx := testutil.Context(t, testutil.WaitLong)

		agent := seedAIAgent(ctx, t, db, member.ID)

		// Auditors can name another sponsor by username or ID.
		agents, err := auditorClient.AIAuditAgents(ctx, member.Username)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		require.Equal(t, agent.UserID, agents[0].UserID)
		agents, err = auditorClient.AIAuditAgents(ctx, member.ID.String())
		require.NoError(t, err)
		require.Len(t, agents, 1)

		// Unknown sponsors are a validation error for authorized callers.
		var sdkErr *codersdk.Error
		_, err = auditorClient.AIAuditAgents(ctx, "nonexistent-user")
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})
}

func TestAIAuditTimeline(t *testing.T) {
	t.Parallel()

	t.Run("EndToEnd", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		agent := seedAIAgent(ctx, t, db, member.ID)
		//nolint:gocritic // Tests seed sponsor-scoped audit rows through system-guarded store methods.
		sysCtx := dbauthz.AsSystemRestricted(ctx)

		base := dbtime.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
		workspaceID := uuid.New()

		// Sandbox session: started base+1m, ended base+10m.
		session, err := db.UpsertAISandboxSession(sysCtx, database.UpsertAISandboxSessionParams{
			ID:                uuid.New(),
			WorkspaceID:       workspaceID,
			ReporterAgentID:   uuid.New(),
			ConfinedAgentID:   uuid.New(),
			AIAgentID:         agent.UserID,
			SponsorUserID:     member.ID,
			EgressEnforcement: "forced",
			StartedAt:         base.Add(1 * time.Minute),
			EndedAt:           sql.NullTime{Time: base.Add(10 * time.Minute), Valid: true},
			CreatedAt:         base.Add(1 * time.Minute),
		})
		require.NoError(t, err)

		// Egress: three allowed hits on one bucket, one denied on another.
		_, err = db.InsertAISandboxNetworkEvents(sysCtx, database.InsertAISandboxNetworkEventsParams{
			SessionID:      []uuid.UUID{session.ID, session.ID, session.ID, session.ID},
			OccurredAt:     []time.Time{base.Add(2 * time.Minute), base.Add(3 * time.Minute), base.Add(4 * time.Minute), base.Add(5 * time.Minute)},
			Protocol:       []string{"tcp", "tcp", "tcp", "http"},
			Host:           []string{"github.com", "github.com", "github.com", "evil.example.com"},
			Port:           []int32{443, 443, 443, 80},
			Action:         []string{"allowed", "allowed", "allowed", "denied"},
			PolicyRevision: []int64{1, 1, 1, 1},
			AIAgentID:      []uuid.UUID{agent.UserID, agent.UserID, agent.UserID, agent.UserID},
			SponsorUserID:  []uuid.UUID{member.ID, member.ID, member.ID, member.ID},
			CreatedAt:      []time.Time{base.Add(2 * time.Minute), base.Add(3 * time.Minute), base.Add(4 * time.Minute), base.Add(5 * time.Minute)},
		})
		require.NoError(t, err)

		// Bridge session with one tool call.
		interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:   agent.UserID,
			SponsorUserID: uuid.NullUUID{UUID: member.ID, Valid: true},
			StartedAt:     base.Add(6 * time.Minute),
			Client:        sql.NullString{String: "claude-code", Valid: true},
		}, nil)
		// Tool usages are written by the bridge, whose actor carries the
		// interception update permission that the restricted system actor
		// deliberately lacks.
		_, err = db.InsertAIBridgeToolUsage(dbauthz.AsAIBridged(ctx), database.InsertAIBridgeToolUsageParams{
			ID:             uuid.New(),
			InterceptionID: interception.ID,
			Tool:           "read_file",
			ServerUrl:      sql.NullString{String: "filesystem", Valid: true},
			Input:          "{}",
			Injected:       true,
			Metadata:       []byte("{}"),
			CreatedAt:      base.Add(7 * time.Minute),
			Disposition:    "escalated_approved",
		})
		require.NoError(t, err)

		// Resolved escalation: created base+8m, approved base+9m.
		escalation, err := db.InsertMCPGatewayEscalation(sysCtx, database.InsertMCPGatewayEscalationParams{
			ID:                uuid.New(),
			MCPServerConfigID: uuid.New(),
			ServerSlug:        "filesystem",
			ServerUrl:         "https://mcp.example.com",
			Tool:              "read_file",
			Input:             []byte(`{"path":"README.md"}`),
			AIAgentID:         agent.UserID,
			SponsorUserID:     member.ID,
			WorkspaceName:     "sandboxed-ws",
			Status:            "approved",
			CreatedAt:         base.Add(8 * time.Minute),
			ExpiresAt:         base.Add(13 * time.Minute),
			ResolvedAt:        sql.NullTime{Time: base.Add(9 * time.Minute), Valid: true},
			ResolvedBy:        uuid.NullUUID{UUID: member.ID, Valid: true},
		})
		require.NoError(t, err)

		// A second agentic identity's escalation, to exercise the agent
		// filter.
		otherAgentID := uuid.New()
		_, err = db.InsertMCPGatewayEscalation(sysCtx, database.InsertMCPGatewayEscalationParams{
			ID:                uuid.New(),
			MCPServerConfigID: uuid.New(),
			ServerSlug:        "github",
			ServerUrl:         "https://mcp.example.com",
			Tool:              "create_issue",
			Input:             []byte(`{}`),
			AIAgentID:         otherAgentID,
			SponsorUserID:     member.ID,
			WorkspaceName:     "sandboxed-ws",
			Status:            "pending",
			CreatedAt:         base.Add(11 * time.Minute),
			ExpiresAt:         base.Add(16 * time.Minute),
		})
		require.NoError(t, err)

		// The sponsor sees the full merged timeline, newest first.
		timeline, err := memberClient.AIAuditTimeline(ctx, codersdk.AIAuditTimelineFilter{})
		require.NoError(t, err)
		types := make([]codersdk.AIAuditEventType, 0, len(timeline.Events))
		for _, event := range timeline.Events {
			types = append(types, event.Type)
			require.Equal(t, member.ID, event.Sponsor.ID)
			require.Equal(t, member.Username, event.Sponsor.Username)
		}
		require.Equal(t, []codersdk.AIAuditEventType{
			codersdk.AIAuditEventTypeEscalationCreated,     // other agent, base+11m
			codersdk.AIAuditEventTypeSandboxSessionEnded,   // base+10m
			codersdk.AIAuditEventTypeEscalationResolved,    // base+9m
			codersdk.AIAuditEventTypeEscalationCreated,     // base+8m
			codersdk.AIAuditEventTypeToolCall,              // base+7m
			codersdk.AIAuditEventTypeBridgeSessionStarted,  // base+6m
			codersdk.AIAuditEventTypeEgress,                // denied, base+5m
			codersdk.AIAuditEventTypeEgress,                // allowed x3, base+4m
			codersdk.AIAuditEventTypeSandboxSessionStarted, // base+1m
		}, types)
		require.Equal(t, len(timeline.Events), timeline.Count)

		// Spot-check details.
		denied := timeline.Events[6]
		require.Equal(t, "evil.example.com", denied.Detail["host"])
		require.Equal(t, "denied", denied.Detail["action"])
		require.EqualValues(t, 1, denied.Detail["count"])
		require.Equal(t, workspaceID, denied.WorkspaceID)
		allowed := timeline.Events[7]
		require.EqualValues(t, 3, allowed.Detail["count"])
		toolCall := timeline.Events[4]
		require.Equal(t, "escalated_approved", toolCall.Detail["disposition"])
		require.Equal(t, agent.UserID, toolCall.AIAgentID)
		resolved := timeline.Events[2]
		require.Equal(t, escalation.ID, resolved.ID)
		require.Equal(t, "approved", resolved.Detail["status"])
		require.Equal(t, "sandboxed-ws", resolved.WorkspaceName)
		ended := timeline.Events[1]
		require.EqualValues(t, (9 * time.Minute).Milliseconds(), ended.Detail["duration_ms"])

		// Type filter.
		timeline, err = memberClient.AIAuditTimeline(ctx, codersdk.AIAuditTimelineFilter{
			Types: []codersdk.AIAuditEventType{codersdk.AIAuditEventTypeEgress},
		})
		require.NoError(t, err)
		require.Len(t, timeline.Events, 2)
		for _, event := range timeline.Events {
			require.Equal(t, codersdk.AIAuditEventTypeEgress, event.Type)
		}

		// Agent filter excludes the other identity's escalation.
		timeline, err = memberClient.AIAuditTimeline(ctx, codersdk.AIAuditTimelineFilter{
			AIAgentID: agent.UserID,
		})
		require.NoError(t, err)
		require.Len(t, timeline.Events, 8)
		for _, event := range timeline.Events {
			require.Equal(t, agent.UserID, event.AIAgentID)
		}

		// Time window: events strictly inside (base+5m30s, base+8m30s).
		timeline, err = memberClient.AIAuditTimeline(ctx, codersdk.AIAuditTimelineFilter{
			AfterTime:  base.Add(5*time.Minute + 30*time.Second),
			BeforeTime: base.Add(8*time.Minute + 30*time.Second),
		})
		require.NoError(t, err)
		types = types[:0]
		for _, event := range timeline.Events {
			types = append(types, event.Type)
		}
		require.Equal(t, []codersdk.AIAuditEventType{
			codersdk.AIAuditEventTypeEscalationCreated,
			codersdk.AIAuditEventTypeToolCall,
			codersdk.AIAuditEventTypeBridgeSessionStarted,
		}, types)

		// Limit trims the merged page, newest first.
		timeline, err = memberClient.AIAuditTimeline(ctx, codersdk.AIAuditTimelineFilter{Limit: 3})
		require.NoError(t, err)
		require.Len(t, timeline.Events, 3)
		require.Equal(t, codersdk.AIAuditEventTypeEscalationCreated, timeline.Events[0].Type)

		// Invalid type is a validation error.
		_, err = memberClient.AIAuditTimeline(ctx, codersdk.AIAuditTimelineFilter{
			Types: []codersdk.AIAuditEventType{"bogus"},
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("SponsorAuthorization", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		otherClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		auditorClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleAuditor())
		ctx := testutil.Context(t, testutil.WaitLong)

		agent := seedAIAgent(ctx, t, db, member.ID)
		//nolint:gocritic // Tests seed sponsor-scoped audit rows through system-guarded store methods.
		sysCtx := dbauthz.AsSystemRestricted(ctx)
		_, err := db.UpsertAISandboxSession(sysCtx, database.UpsertAISandboxSessionParams{
			ID:                uuid.New(),
			WorkspaceID:       uuid.New(),
			ReporterAgentID:   uuid.New(),
			ConfinedAgentID:   uuid.New(),
			AIAgentID:         agent.UserID,
			SponsorUserID:     member.ID,
			EgressEnforcement: "forced",
			StartedAt:         dbtime.Now().Add(-time.Hour),
			CreatedAt:         dbtime.Now().Add(-time.Hour),
		})
		require.NoError(t, err)

		// Members cannot read another sponsor's timeline.
		var sdkErr *codersdk.Error
		_, err = otherClient.AIAuditTimeline(ctx, codersdk.AIAuditTimelineFilter{Sponsor: member.Username})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		// Auditors can.
		timeline, err := auditorClient.AIAuditTimeline(ctx, codersdk.AIAuditTimelineFilter{Sponsor: member.Username})
		require.NoError(t, err)
		require.Len(t, timeline.Events, 1)
		require.Equal(t, codersdk.AIAuditEventTypeSandboxSessionStarted, timeline.Events[0].Type)
		require.Equal(t, member.ID, timeline.Events[0].Sponsor.ID)
	})
}

func seedAIAgent(ctx context.Context, t *testing.T, db database.Store, ownerID uuid.UUID) database.AIAgent {
	t.Helper()

	id := uuid.New()
	//nolint:gocritic // Unit test seeds identity rows directly.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	_, err := db.InsertAIAgentUser(sysCtx, database.InsertAIAgentUserParams{
		ID:        id,
		Username:  "ai-" + id.String()[:8],
		CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	agent, err := db.InsertAIAgent(sysCtx, database.InsertAIAgentParams{
		UserID:      id,
		OwnerUserID: ownerID,
		OriginType:  database.AIAgentOriginWorkspace,
		OriginID:    uuid.New(),
		CreatedAt:   dbtime.Now(),
	})
	require.NoError(t, err)
	return agent
}
