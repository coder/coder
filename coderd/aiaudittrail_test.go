package coderd_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// seedAIAuditTrail drives the real write paths: it creates an AI agent
// (which grants authorization and issues a credential in the same
// transaction), records an accepted and a refused presentation, retires the
// agent, and stores one egress session with two network events.
func seedAIAuditTrail(t *testing.T, db database.Store, ownerID uuid.UUID) entity.NewAIAgent {
	t.Helper()
	// The store handed back by coderdtest is authorization-wrapped, and the
	// journal writes assert the system actor.
	//nolint:gocritic // Test seeding writes journals the way coderd itself does.
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong))

	agent, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
		Owner:        entity.Ref{Type: entity.TypeUser, ID: ownerID},
		CreationSite: entity.CreationSite{Type: entity.CreationSiteTypeWorkspace, ID: uuid.New()},
	})
	require.NoError(t, err)

	verifier := entity.Ref{Type: entity.TypeUser, ID: ownerID}
	accepted, err := entity.VerifyCredential(ctx, db, entity.Presentation{
		Declared:            agent.CredentialID,
		AuthenticatorOutput: agent.Authenticator,
		Verifier:            verifier,
		AnnotationSource:    "test",
	})
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = entity.VerifyCredential(ctx, db, entity.Presentation{
		Declared:            agent.CredentialID,
		AuthenticatorOutput: "wrong-secret",
		Verifier:            verifier,
		AnnotationSource:    "test",
	})
	require.NoError(t, err)
	require.False(t, accepted)

	err = entity.RetireAIAgent(ctx, db, agent.ID, entity.EventAIAgentFinish, entity.Ref{Type: entity.TypeUser, ID: ownerID}, time.Time{})
	require.NoError(t, err)

	now := time.Now()
	sessionID := uuid.New()
	_, err = db.UpsertAISandboxSession(ctx, database.UpsertAISandboxSessionParams{
		ID:                sessionID,
		WorkspaceID:       uuid.New(),
		ReporterAgentID:   uuid.New(),
		ConfinedAgentID:   uuid.New(),
		AIAgentID:         agent.ID,
		SponsorUserID:     ownerID,
		EgressEnforcement: "forced",
		StartedAt:         now.Add(-time.Hour),
		EndedAt:           sql.NullTime{Time: now.Add(-time.Minute), Valid: true},
		CreatedAt:         now,
	})
	require.NoError(t, err)

	_, err = db.InsertAISandboxNetworkEvents(ctx, database.InsertAISandboxNetworkEventsParams{
		SessionID:      []uuid.UUID{sessionID, sessionID},
		OccurredAt:     []time.Time{now.Add(-30 * time.Minute), now.Add(-20 * time.Minute)},
		Protocol:       []string{"tcp", "tcp"},
		Host:           []string{"github.com", "github.com"},
		Port:           []int32{443, 443},
		Action:         []string{"denied", "denied"},
		PolicyRevision: []int64{1, 1},
		AIAgentID:      []uuid.UUID{agent.ID, agent.ID},
		SponsorUserID:  []uuid.UUID{ownerID, ownerID},
		CreatedAt:      []time.Time{now, now},
	})
	require.NoError(t, err)

	return agent
}

func TestAIAuditTrailTimeline(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	auditorClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.RoleAuditor())
	otherClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

	agent := seedAIAuditTrail(t, db, member.ID)

	t.Run("OwnTrail", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := memberClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{})
		require.NoError(t, err)
		require.Equal(t, len(res.Events), res.Count)

		byType := map[codersdk.AIAuditTrailEventType]int{}
		for _, event := range res.Events {
			byType[event.Type]++
			require.Equal(t, member.ID, event.Owner.ID)
			require.Equal(t, "user", event.Owner.Type)
			require.Equal(t, agent.ID, event.AIAgentID)
			require.False(t, event.OccurredAt.IsZero(), "event %s missing occurred_at", event.ID)
			require.False(t, event.RecordedAt.IsZero(), "event %s missing recorded_at", event.ID)
		}
		// Create and retire.
		require.Equal(t, 2, byType[codersdk.AIAuditTrailEventAIAgentLifecycle])
		// Grant and the lapse entailed by retirement.
		require.Equal(t, 2, byType[codersdk.AIAuditTrailEventAuthorizationLifecycle])
		// Issue and the discharge entailed by retirement.
		require.Equal(t, 2, byType[codersdk.AIAuditTrailEventCredentialLifecycle])
		// One accepted and one refused presentation.
		require.Equal(t, 2, byType[codersdk.AIAuditTrailEventCredentialUse])
		// Started and ended.
		require.Equal(t, 2, byType[codersdk.AIAuditTrailEventSandboxSession])
		// One (session, host, action) bucket with two occurrences.
		require.Equal(t, 1, byType[codersdk.AIAuditTrailEventEgress])

		// Newest-first presentation order.
		for i := 1; i < len(res.Events); i++ {
			require.False(t, res.Events[i].OccurredAt.After(res.Events[i-1].OccurredAt),
				"events out of order at index %d", i)
		}
	})

	t.Run("TypeFilter", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := memberClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			Types: []codersdk.AIAuditTrailEventType{codersdk.AIAuditTrailEventCredentialUse},
		})
		require.NoError(t, err)
		require.Len(t, res.Events, 2)
		for _, event := range res.Events {
			require.Equal(t, codersdk.AIAuditTrailEventCredentialUse, event.Type)
		}
	})

	t.Run("UnknownTypeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := memberClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			Types: []codersdk.AIAuditTrailEventType{"not_a_type"},
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 400, sdkErr.StatusCode())
	})

	t.Run("AgentFilter", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := memberClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			AIAgentID: uuid.New(),
		})
		require.NoError(t, err)
		require.Empty(t, res.Events)
	})

	t.Run("LimitAndPaging", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		first, err := memberClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{Limit: 3})
		require.NoError(t, err)
		require.Len(t, first.Events, 3)

		seen := map[string]struct{}{}
		for _, event := range first.Events {
			seen[event.ID] = struct{}{}
		}
		second, err := memberClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			Limit:      1000,
			BeforeTime: first.Events[len(first.Events)-1].OccurredAt,
		})
		require.NoError(t, err)
		for _, event := range second.Events {
			_, duplicate := seen[event.ID]
			require.False(t, duplicate, "event %s returned on both pages", event.ID)
		}
	})

	t.Run("OtherMemberForbidden", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := otherClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			Owner: member.Username,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 403, sdkErr.StatusCode())
	})

	t.Run("AuditorReadsAnyOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := auditorClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			Owner: member.Username,
		})
		require.NoError(t, err)
		require.NotEmpty(t, res.Events)
		for _, event := range res.Events {
			require.Equal(t, member.ID, event.Owner.ID)
		}
	})

	t.Run("OwnerRoleReadsAnyOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := client.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			Owner: member.ID.String(),
		})
		require.NoError(t, err)
		require.NotEmpty(t, res.Events)
	})

	t.Run("UnknownOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := auditorClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			Owner: "no-such-user",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 400, sdkErr.StatusCode())
	})

	t.Run("EgressAggregate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := memberClient.AIAuditTrailTimeline(ctx, codersdk.AIAuditTrailFilter{
			Types: []codersdk.AIAuditTrailEventType{codersdk.AIAuditTrailEventEgress},
		})
		require.NoError(t, err)
		require.Len(t, res.Events, 1)
		event := res.Events[0]
		require.Equal(t, "denied tcp github.com:443 (x2)", event.Summary)
		require.EqualValues(t, 2, event.Detail["count"])
	})
}
