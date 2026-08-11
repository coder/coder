package coderd_test

import (
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

type aiSandboxAuditFixture struct {
	client      *codersdk.Client
	db          database.Store
	owner       codersdk.CreateFirstUserResponse
	workspace   dbfake.WorkspaceResponse
	agentClient *agentsdk.Client
	agentUserID uuid.UUID
}

func newAISandboxAuditFixture(t *testing.T) aiSandboxAuditFixture {
	t.Helper()

	client, db := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        owner.UserID,
	}).WithAgent().Do()

	return aiSandboxAuditFixture{
		client:      client,
		db:          db,
		owner:       owner,
		workspace:   workspace,
		agentClient: agentsdk.New(client.URL, agentsdk.WithFixedToken(workspace.AgentToken)),
	}
}

func newBoundAISandboxAuditFixture(t *testing.T) aiSandboxAuditFixture {
	t.Helper()

	fixture := newAISandboxAuditFixture(t)
	fixture.agentUserID = bindAISandboxAuditAgent(t, fixture.db, fixture.owner, fixture.workspace.Agents[0].ID)
	return fixture
}

func bindAISandboxAuditAgent(t *testing.T, db database.Store, owner codersdk.CreateFirstUserResponse, agentID uuid.UUID) uuid.UUID {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	agentUser, _, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.UserID,
		OrganizationID: owner.OrganizationID,
		OriginType:     database.AIAgentOriginWorkspace,
		OriginID:       uuid.New(),
	})
	require.NoError(t, err)
	_, err = db.UpdateWorkspaceAgentAIAgentID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentAIAgentIDParams{
		ID:        agentID,
		AIAgentID: uuid.NullUUID{UUID: agentUser.ID, Valid: true},
	})
	require.NoError(t, err)
	return agentUser.ID
}

func requireAISandboxAuditStatus(t *testing.T, err error, status int) {
	t.Helper()

	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, status, sdkErr.StatusCode())
}

func requireAISandboxAuditStatusOneOf(t *testing.T, err error, statuses ...int) {
	t.Helper()

	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Contains(t, statuses, sdkErr.StatusCode())
}

func TestAISandboxAuditBoundSelfSession(t *testing.T) {
	t.Parallel()

	fixture := newBoundAISandboxAuditFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	startedAt := time.Now().UTC().Add(-time.Minute)
	sessionID := uuid.New()

	err := fixture.agentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                sessionID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
		StartedAt:         startedAt,
	})
	require.NoError(t, err)

	session, err := fixture.db.GetAISandboxSessionByID(dbauthz.AsSystemRestricted(ctx), sessionID)
	require.NoError(t, err)
	require.Equal(t, fixture.workspace.Workspace.ID, session.WorkspaceID)
	require.Equal(t, fixture.workspace.Agents[0].ID, session.ReporterAgentID)
	require.Equal(t, fixture.workspace.Agents[0].ID, session.ConfinedAgentID)
	require.Equal(t, fixture.agentUserID, session.AIAgentID)
	require.Equal(t, fixture.owner.UserID, session.SponsorUserID)
	require.Equal(t, string(codersdk.AISandboxEgressEnforcementForced), session.EgressEnforcement)
	require.WithinDuration(t, startedAt, session.StartedAt, time.Millisecond)
	require.False(t, session.EndedAt.Valid)

	endedAt := startedAt.Add(time.Hour)
	err = fixture.agentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                sessionID,
		ChildAgentID:      uuid.New(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementNone,
		StartedAt:         startedAt.Add(-time.Hour),
		EndedAt:           &endedAt,
	})
	require.NoError(t, err)

	closed, err := fixture.db.GetAISandboxSessionByID(dbauthz.AsSystemRestricted(ctx), sessionID)
	require.NoError(t, err)
	require.Equal(t, session.WorkspaceID, closed.WorkspaceID)
	require.Equal(t, session.ReporterAgentID, closed.ReporterAgentID)
	require.Equal(t, session.ConfinedAgentID, closed.ConfinedAgentID)
	require.Equal(t, session.AIAgentID, closed.AIAgentID)
	require.Equal(t, session.SponsorUserID, closed.SponsorUserID)
	require.Equal(t, session.EgressEnforcement, closed.EgressEnforcement)
	require.Equal(t, session.StartedAt, closed.StartedAt)
	require.True(t, closed.EndedAt.Valid)
	require.WithinDuration(t, endedAt, closed.EndedAt.Time, time.Millisecond)
}

func TestAISandboxAuditUnboundSelfSession(t *testing.T) {
	t.Parallel()

	fixture := newAISandboxAuditFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	err := fixture.agentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                uuid.New(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementAdvisory,
		StartedAt:         time.Now().UTC(),
	})
	requireAISandboxAuditStatus(t, err, http.StatusForbidden)
}

func TestAISandboxAuditParentChildSession(t *testing.T) {
	t.Parallel()

	fixture := newAISandboxAuditFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	parent := fixture.workspace.Agents[0]
	child := dbgen.WorkspaceAgent(t, fixture.db, database.WorkspaceAgent{
		ParentID:   uuid.NullUUID{UUID: parent.ID, Valid: true},
		ResourceID: parent.ResourceID,
	})
	childAgentUserID := bindAISandboxAuditAgent(t, fixture.db, fixture.owner, child.ID)

	sessionID := uuid.New()
	err := fixture.agentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                sessionID,
		ChildAgentID:      child.ID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
		StartedAt:         time.Now().UTC(),
	})
	require.NoError(t, err)

	session, err := fixture.db.GetAISandboxSessionByID(dbauthz.AsSystemRestricted(ctx), sessionID)
	require.NoError(t, err)
	require.Equal(t, parent.ID, session.ReporterAgentID)
	require.Equal(t, child.ID, session.ConfinedAgentID)
	require.Equal(t, childAgentUserID, session.AIAgentID)
	require.Equal(t, fixture.owner.UserID, session.SponsorUserID)

	otherWorkspace := dbfake.WorkspaceBuild(t, fixture.db, database.WorkspaceTable{
		OrganizationID: fixture.owner.OrganizationID,
		OwnerID:        fixture.owner.UserID,
	}).WithAgent().Do()
	otherClient := agentsdk.New(fixture.client.URL, agentsdk.WithFixedToken(otherWorkspace.AgentToken))
	err = otherClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                uuid.New(),
		ChildAgentID:      child.ID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
		StartedAt:         time.Now().UTC(),
	})
	requireAISandboxAuditStatus(t, err, http.StatusNotFound)

	unboundChild := dbgen.WorkspaceAgent(t, fixture.db, database.WorkspaceAgent{
		ParentID:   uuid.NullUUID{UUID: parent.ID, Valid: true},
		ResourceID: parent.ResourceID,
	})
	err = fixture.agentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                uuid.New(),
		ChildAgentID:      unboundChild.ID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
		StartedAt:         time.Now().UTC(),
	})
	requireAISandboxAuditStatus(t, err, http.StatusForbidden)
}

func TestAISandboxAuditReadAPI(t *testing.T) {
	t.Parallel()

	fixture := newBoundAISandboxAuditFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	workspaceID := fixture.workspace.Workspace.ID
	startedAt := time.Now().UTC().Add(-time.Minute)
	sessionID := uuid.New()

	err := fixture.agentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                sessionID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
		StartedAt:         startedAt,
	})
	require.NoError(t, err)

	err = fixture.agentClient.PatchAISandboxNetworkEvents(ctx, agentsdk.PatchAISandboxNetworkEventsRequest{
		Events: []agentsdk.AISandboxNetworkEvent{
			{
				SessionID:      sessionID,
				OccurredAt:     startedAt.Add(time.Second),
				Protocol:       agentsdk.AISandboxNetworkProtocolConnect,
				Host:           "first.example.com",
				Port:           443,
				Action:         agentsdk.AISandboxNetworkEventActionAllowed,
				PolicyRevision: 10,
			},
			{
				SessionID:      sessionID,
				OccurredAt:     startedAt.Add(2 * time.Second),
				Protocol:       agentsdk.AISandboxNetworkProtocolHTTP,
				Host:           "second.example.com",
				Port:           80,
				Action:         agentsdk.AISandboxNetworkEventActionDenied,
				PolicyRevision: 11,
			},
		},
	})
	require.NoError(t, err)

	sessions, err := fixture.client.WorkspaceAISandboxSessions(ctx, workspaceID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, sessionID, sessions[0].ID)
	require.Equal(t, workspaceID, sessions[0].WorkspaceID)
	require.Equal(t, fixture.workspace.Agents[0].ID, sessions[0].ReporterAgentID)
	require.Equal(t, fixture.workspace.Agents[0].ID, sessions[0].ConfinedAgentID)
	require.Equal(t, fixture.agentUserID, sessions[0].AIAgentID)
	require.Equal(t, fixture.owner.UserID, sessions[0].SponsorUserID)
	require.Equal(t, codersdk.AISandboxEgressEnforcementForced, sessions[0].EgressEnforcement)
	require.Nil(t, sessions[0].EndedAt)

	storedEvents, err := fixture.db.GetAISandboxNetworkEventsBySessionID(dbauthz.AsSystemRestricted(ctx), sessionID)
	require.NoError(t, err)
	require.Len(t, storedEvents, 2)
	firstStored, secondStored := storedEvents[0], storedEvents[1]
	if secondStored.ID < firstStored.ID {
		firstStored, secondStored = secondStored, firstStored
	}

	firstPage, err := fixture.client.AISandboxSessionNetworkEvents(ctx, workspaceID, sessionID, 0, 1)
	require.NoError(t, err)
	require.Len(t, firstPage, 1)
	require.Equal(t, firstStored.SessionID, firstPage[0].SessionID)
	require.Equal(t, firstStored.Host, firstPage[0].Host)
	require.Equal(t, int(firstStored.Port), firstPage[0].Port)
	require.Equal(t, codersdk.AISandboxNetworkProtocol(firstStored.Protocol), firstPage[0].Protocol)
	require.Equal(t, codersdk.AISandboxNetworkEventAction(firstStored.Action), firstPage[0].Action)
	require.Equal(t, firstStored.PolicyRevision, firstPage[0].PolicyRevision)

	secondPage, err := fixture.client.AISandboxSessionNetworkEvents(ctx, workspaceID, sessionID, firstStored.ID, 1)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	require.Equal(t, secondStored.Host, secondPage[0].Host)

	unrelatedClient, _ := coderdtest.CreateAnotherUser(t, fixture.client, fixture.owner.OrganizationID)
	_, err = unrelatedClient.WorkspaceAISandboxSessions(ctx, workspaceID)
	requireAISandboxAuditStatusOneOf(t, err, http.StatusForbidden, http.StatusNotFound)
	_, err = unrelatedClient.AISandboxSessionNetworkEvents(ctx, workspaceID, sessionID, 0, 1)
	requireAISandboxAuditStatusOneOf(t, err, http.StatusForbidden, http.StatusNotFound)

	otherWorkspace := dbfake.WorkspaceBuild(t, fixture.db, database.WorkspaceTable{
		OrganizationID: fixture.owner.OrganizationID,
		OwnerID:        fixture.owner.UserID,
	}).WithAgent().Do()
	bindAISandboxAuditAgent(t, fixture.db, fixture.owner, otherWorkspace.Agents[0].ID)
	otherAgentClient := agentsdk.New(fixture.client.URL, agentsdk.WithFixedToken(otherWorkspace.AgentToken))
	otherSessionID := uuid.New()
	err = otherAgentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                otherSessionID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementAdvisory,
		StartedAt:         startedAt,
	})
	require.NoError(t, err)
	_, err = fixture.client.AISandboxSessionNetworkEvents(ctx, workspaceID, otherSessionID, 0, 1)
	requireAISandboxAuditStatus(t, err, http.StatusNotFound)

	_, err = fixture.db.SetWorkspaceAIAgentID(dbauthz.AsSystemRestricted(ctx), database.SetWorkspaceAIAgentIDParams{
		ID:        workspaceID,
		AIAgentID: uuid.NullUUID{UUID: fixture.agentUserID, Valid: true},
	})
	require.NoError(t, err)
	workspace, err := fixture.client.Workspace(ctx, workspaceID)
	require.NoError(t, err)
	require.NotNil(t, workspace.AIAgentID)
	require.Equal(t, fixture.agentUserID, *workspace.AIAgentID)

	var foundAgent *codersdk.WorkspaceAgent
	for _, resource := range workspace.LatestBuild.Resources {
		for i := range resource.Agents {
			if resource.Agents[i].ID == fixture.workspace.Agents[0].ID {
				foundAgent = &resource.Agents[i]
			}
		}
	}
	require.NotNil(t, foundAgent)
	require.NotNil(t, foundAgent.AIAgentID)
	require.Equal(t, fixture.agentUserID, *foundAgent.AIAgentID)
}

func TestAISandboxAuditNetworkEvents(t *testing.T) {
	t.Parallel()

	fixture := newBoundAISandboxAuditFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	startedAt := time.Now().UTC().Add(-time.Minute)
	ownedSessionID := uuid.New()
	err := fixture.agentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                ownedSessionID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
		StartedAt:         startedAt,
	})
	require.NoError(t, err)

	occurredAt := startedAt.Add(time.Second)
	err = fixture.agentClient.PatchAISandboxNetworkEvents(ctx, agentsdk.PatchAISandboxNetworkEventsRequest{
		Events: []agentsdk.AISandboxNetworkEvent{
			{
				SessionID:      ownedSessionID,
				OccurredAt:     occurredAt,
				Protocol:       agentsdk.AISandboxNetworkProtocolConnect,
				Host:           "packages.example.com",
				Port:           443,
				Action:         agentsdk.AISandboxNetworkEventActionAllowed,
				PolicyRevision: 12,
			},
			{
				SessionID:      ownedSessionID,
				OccurredAt:     occurredAt.Add(time.Second),
				Protocol:       agentsdk.AISandboxNetworkProtocolTCP,
				Host:           "",
				Port:           0,
				Action:         agentsdk.AISandboxNetworkEventActionDenied,
				PolicyRevision: 0,
			},
		},
	})
	require.NoError(t, err)

	events, err := fixture.db.GetAISandboxNetworkEventsBySessionID(dbauthz.AsSystemRestricted(ctx), ownedSessionID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	for _, event := range events {
		require.Equal(t, fixture.agentUserID, event.AIAgentID)
		require.Equal(t, fixture.owner.UserID, event.SponsorUserID)
	}
	require.Equal(t, "packages.example.com", events[0].Host)
	require.EqualValues(t, 443, events[0].Port)
	require.EqualValues(t, 12, events[0].PolicyRevision)

	mixedOwnedSessionID := uuid.New()
	err = fixture.agentClient.PostAISandboxSession(ctx, agentsdk.PostAISandboxSessionRequest{
		ID:                mixedOwnedSessionID,
		EgressEnforcement: codersdk.AISandboxEgressEnforcementAdvisory,
		StartedAt:         startedAt,
	})
	require.NoError(t, err)
	unownedSessionID := uuid.New()
	_, err = fixture.db.UpsertAISandboxSession(dbauthz.AsSystemRestricted(ctx), database.UpsertAISandboxSessionParams{
		ID:                unownedSessionID,
		WorkspaceID:       fixture.workspace.Workspace.ID,
		ReporterAgentID:   uuid.New(),
		ConfinedAgentID:   uuid.New(),
		AIAgentID:         uuid.New(),
		SponsorUserID:     uuid.New(),
		EgressEnforcement: string(codersdk.AISandboxEgressEnforcementForced),
		StartedAt:         startedAt,
		EndedAt:           sql.NullTime{},
		CreatedAt:         startedAt,
	})
	require.NoError(t, err)

	err = fixture.agentClient.PatchAISandboxNetworkEvents(ctx, agentsdk.PatchAISandboxNetworkEventsRequest{
		Events: []agentsdk.AISandboxNetworkEvent{
			{
				SessionID:  mixedOwnedSessionID,
				OccurredAt: occurredAt,
				Protocol:   agentsdk.AISandboxNetworkProtocolHTTP,
				Host:       "owned.example.com",
				Port:       80,
				Action:     agentsdk.AISandboxNetworkEventActionAllowed,
			},
			{
				SessionID:  unownedSessionID,
				OccurredAt: occurredAt,
				Protocol:   agentsdk.AISandboxNetworkProtocolSNI,
				Host:       "unowned.example.com",
				Port:       443,
				Action:     agentsdk.AISandboxNetworkEventActionDenied,
			},
		},
	})
	requireAISandboxAuditStatus(t, err, http.StatusNotFound)

	for _, sessionID := range []uuid.UUID{mixedOwnedSessionID, unownedSessionID} {
		rows, err := fixture.db.GetAISandboxNetworkEventsBySessionID(dbauthz.AsSystemRestricted(ctx), sessionID)
		require.NoError(t, err)
		require.Empty(t, rows)
	}

	err = fixture.agentClient.PatchAISandboxNetworkEvents(ctx, agentsdk.PatchAISandboxNetworkEventsRequest{
		Events: []agentsdk.AISandboxNetworkEvent{{
			SessionID:  ownedSessionID,
			OccurredAt: occurredAt,
			Protocol:   agentsdk.AISandboxNetworkProtocol("invalid"),
			Host:       "invalid.example.com",
			Port:       443,
			Action:     agentsdk.AISandboxNetworkEventActionDenied,
		}},
	})
	requireAISandboxAuditStatus(t, err, http.StatusBadRequest)

	tooMany := make([]agentsdk.AISandboxNetworkEvent, 1001)
	for i := range tooMany {
		tooMany[i] = agentsdk.AISandboxNetworkEvent{
			SessionID:  ownedSessionID,
			OccurredAt: occurredAt,
			Protocol:   agentsdk.AISandboxNetworkProtocolConnect,
			Host:       "example.com",
			Port:       443,
			Action:     agentsdk.AISandboxNetworkEventActionAllowed,
		}
	}
	err = fixture.agentClient.PatchAISandboxNetworkEvents(ctx, agentsdk.PatchAISandboxNetworkEventsRequest{Events: tooMany})
	requireAISandboxAuditStatus(t, err, http.StatusBadRequest)
}
