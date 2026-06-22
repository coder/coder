package workspaceconnwatcher_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/coderd/workspaceconnwatcher"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

var (
	workspaceID = uuid.UUID{1}
	userID      = uuid.UUID{2}
	orgID       = uuid.UUID{3}
	agentID     = uuid.UUID{4}
)

type harness struct {
	db      *dbmock.MockStore
	watcher *workspaceconnwatcher.Watcher
	pub     pubsub.Publisher
	logger  slog.Logger

	// Initialized, but overridable before Dial()
	workspace     database.Workspace
	userID, orgID uuid.UUID
}

func newHarness(ctx context.Context, t *testing.T, logger slog.Logger) *harness {
	h := &harness{
		workspace: database.Workspace{
			ID:             workspaceID,
			OrganizationID: orgID,
			OwnerID:        userID,
		},
		orgID:  orgID,
		userID: userID,
		logger: logger,
	}
	ps := pubsub.NewInMemory()
	h.pub = ps

	var authzDB database.Store
	_, h.db, authzDB, _ = coderdtest.MockedDatabaseWithAuthz(t, logger)
	h.watcher = workspaceconnwatcher.New(ctx, logger.Named("watcher"), ps, authzDB)
	t.Cleanup(h.watcher.Close)
	return h
}

func (h *harness) Dial(ctx context.Context, url string) (*wsjson.Decoder[workspacesdk.ConnectionWatchEvent], error) {
	rt := testutil.InMemWebsocketRoundTripper{
		Handler: http.HandlerFunc(h.watcher.WorkspaceAgentConnectionWatch),
		CtxMutator: func(ctx context.Context) context.Context {
			ctx = httpmw.WithWorkspaceParam(ctx, h.workspace)
			ctx = dbauthz.As(ctx, memberSubject(userID, orgID))
			return ctx
		},
		Logger: h.logger.Named("roundtripper"),
	}
	// nolint: bodyclose
	clientSock, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: rt},
	})
	if err != nil {
		if resp.StatusCode != http.StatusSwitchingProtocols {
			return nil, codersdk.ReadBodyAsError(resp)
		}
		return nil, err
	}

	dec := wsjson.NewDecoder[workspacesdk.ConnectionWatchEvent](
		clientSock, websocket.MessageText, h.logger.Named("decoder"))
	return dec, nil
}

func TestWatcher_Agents(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                   string
		agents                 []database.WorkspaceAgent
		agentDBError           error
		url                    string
		expectedAgentUpdate    *workspacesdk.AgentUpdate
		expectedErrorCode      workspacesdk.WatchErrorCode
		expectedErrorRetryable bool
	}{
		{
			name: "noNameSingleAgent",
			agents: []database.WorkspaceAgent{
				{
					Name:           "test",
					ID:             agentID,
					LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
				},
			},
			url: "wss://local.test/",
			expectedAgentUpdate: &workspacesdk.AgentUpdate{
				Lifecycle: codersdk.WorkspaceAgentLifecycleCreated,
				ID:        agentID,
			},
		},
		{
			name: "noNameMultiAgent",
			agents: []database.WorkspaceAgent{
				{
					Name:           "agent0",
					ID:             agentID,
					LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
				},
				{
					Name:           "agent1",
					ID:             uuid.UUID{77},
					LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
				},
			},
			url:                    "wss://local.test/",
			expectedErrorCode:      workspacesdk.WatchErrorTooManyAgents,
			expectedErrorRetryable: false,
		},
		{
			name: "namedAgentMultiAgent",
			agents: []database.WorkspaceAgent{
				{
					Name:           "agent0",
					ID:             agentID,
					LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
				},
				{
					Name:           "agent1",
					ID:             uuid.UUID{77},
					LifecycleState: database.WorkspaceAgentLifecycleStateReady,
				},
			},
			url: "wss://local.test/?agent_name=agent0",
			expectedAgentUpdate: &workspacesdk.AgentUpdate{
				Lifecycle: codersdk.WorkspaceAgentLifecycleCreated,
				ID:        agentID,
			},
		},
		{
			name: "namedAgentNonexistent",
			agents: []database.WorkspaceAgent{
				{
					Name:           "agent0",
					ID:             agentID,
					LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
				},
				{
					Name:           "agent1",
					ID:             uuid.UUID{77},
					LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
				},
			},
			url:                    "wss://local.test/?agent_name=agent2",
			expectedErrorCode:      workspacesdk.WatchErrorNameNotFound,
			expectedErrorRetryable: false,
		},
		{
			name:                   "dbError",
			agentDBError:           xerrors.New("a bad thing happened"),
			url:                    "wss://local.test/",
			expectedErrorCode:      workspacesdk.WatchErrorDatabase,
			expectedErrorRetryable: true,
		},
		{
			name:                   "unauthorized",
			agentDBError:           dbauthz.NotAuthorizedError{Err: xerrors.New("not allowed")},
			url:                    "wss://local.test/",
			expectedErrorCode:      workspacesdk.WatchErrorDatabase,
			expectedErrorRetryable: false,
		},
		{
			name:                   "noAgents",
			agents:                 []database.WorkspaceAgent{},
			url:                    "wss://local.test/",
			expectedErrorCode:      workspacesdk.WatchErrorNoAgents,
			expectedErrorRetryable: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			logger := testutil.Logger(t)
			h := newHarness(ctx, t, logger)

			h.db.EXPECT().GetLatestWorkspaceBuildWithStatusByWorkspaceID(gomock.Any(), h.workspace.ID).
				Times(1).
				Return(database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow{
					Transition:  database.WorkspaceTransitionStart,
					BuildNumber: 1,
					JobStatus:   database.ProvisionerJobStatusSucceeded,
					WorkspaceTable: database.WorkspaceTable{
						ID:             h.workspace.ID,
						OwnerID:        userID,
						OrganizationID: orgID,
					},
				}, nil)
			// RBAC check for agent query
			h.db.EXPECT().GetWorkspaceByID(gomock.Any(), h.workspace.ID).
				Times(1).
				Return(h.workspace, nil)
			h.db.EXPECT().GetWorkspaceAgentsByWorkspaceAndBuildNumber(
				gomock.Any(),
				database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
					WorkspaceID: h.workspace.ID,
					BuildNumber: 1,
				}).
				Times(1).
				Return(tc.agents, tc.agentDBError)

			dec, err := h.Dial(ctx, tc.url)
			require.NoError(t, err)
			defer dec.Close()
			events := dec.Chan()
			e0 := testutil.RequireReceive(ctx, t, events)
			require.Equal(t, workspacesdk.ConnectionWatchEvent{
				BuildUpdate: &workspacesdk.BuildUpdate{
					Transition: codersdk.WorkspaceTransitionStart,
					JobStatus:  codersdk.ProvisionerJobSucceeded,
				},
			}, e0)

			e1 := testutil.RequireReceive(ctx, t, events)
			if tc.expectedAgentUpdate != nil {
				require.Equal(t, workspacesdk.ConnectionWatchEvent{AgentUpdate: tc.expectedAgentUpdate}, e1)
			} else {
				require.NotNil(t, e1.Error)
				require.Equal(t, tc.expectedErrorRetryable, e1.Error.Retryable)
				require.Equal(t, tc.expectedErrorCode, e1.Error.Code)
			}
		})
	}
}

func TestWatcher_LostAccess(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	h := newHarness(ctx, t, logger)

	h.db.EXPECT().GetLatestWorkspaceBuildWithStatusByWorkspaceID(gomock.Any(), h.workspace.ID).
		Times(1).
		Return(database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow{
			Transition:  database.WorkspaceTransitionStart,
			BuildNumber: 1,
			JobStatus:   database.ProvisionerJobStatusSucceeded,
			WorkspaceTable: database.WorkspaceTable{
				ID:             h.workspace.ID,
				OwnerID:        uuid.UUID{99}, // workspace gets a new owner, e.g.
				OrganizationID: orgID,
			},
		}, nil)

	dec, err := h.Dial(ctx, "wss://local.test/")
	require.NoError(t, err)
	defer func() {
		_ = dec.Close()
	}()
	events := dec.Chan()
	e0 := testutil.RequireReceive(ctx, t, events)
	require.NotNil(t, e0.Error)
	require.Equal(t, workspacesdk.WatchErrorDatabase, e0.Error.Code)
	require.False(t, e0.Error.Retryable)
	require.Equal(t, "unauthorized", e0.Error.Details, "should not leak internal auth details")
}

func TestWatcher_PublishChanges(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	h := newHarness(ctx, t, logger)

	// Initial build update, job is running.
	build0 := h.db.EXPECT().GetLatestWorkspaceBuildWithStatusByWorkspaceID(gomock.Any(), h.workspace.ID).
		Times(1).
		Return(database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow{
			Transition:  database.WorkspaceTransitionStart,
			BuildNumber: 1,
			JobStatus:   database.ProvisionerJobStatusRunning,
			WorkspaceTable: database.WorkspaceTable{
				ID:             h.workspace.ID,
				OwnerID:        userID,
				OrganizationID: orgID,
			},
		}, nil)

	dec, err := h.Dial(ctx, "wss://local.test/")
	require.NoError(t, err)
	defer func() {
		_ = dec.Close()
	}()
	events := dec.Chan()

	e0 := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, workspacesdk.ConnectionWatchEvent{
		BuildUpdate: &workspacesdk.BuildUpdate{
			Transition: codersdk.WorkspaceTransitionStart,
			JobStatus:  codersdk.ProvisionerJobRunning,
		},
	}, e0)

	// Since job is still running, we don't immediately query for agents. Next we set up the db queries and send in an
	// update over the pubsub to kick  a new query.
	build1 := h.db.EXPECT().GetLatestWorkspaceBuildWithStatusByWorkspaceID(gomock.Any(), h.workspace.ID).
		After(build0).
		Times(1).
		Return(database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow{
			Transition:  database.WorkspaceTransitionStart,
			BuildNumber: 1,
			JobStatus:   database.ProvisionerJobStatusSucceeded,
			WorkspaceTable: database.WorkspaceTable{
				ID:             h.workspace.ID,
				OwnerID:        userID,
				OrganizationID: orgID,
			},
		}, nil)
	// RBAC check for agent query
	h.db.EXPECT().GetWorkspaceByID(gomock.Any(), h.workspace.ID).
		After(build1).
		Times(2). // these queries are identical between the initial and the update below
		Return(h.workspace, nil)
	agent0 := h.db.EXPECT().GetWorkspaceAgentsByWorkspaceAndBuildNumber(
		gomock.Any(),
		database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
			WorkspaceID: h.workspace.ID,
			BuildNumber: 1,
		}).
		After(build1).
		Times(1).
		Return([]database.WorkspaceAgent{
			{
				Name:           "test",
				ID:             agentID,
				LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
			},
		}, nil)
	changeMsg := wspubsub.WorkspaceEvent{
		Kind:        wspubsub.WorkspaceEventKindStateChange,
		WorkspaceID: h.workspace.ID,
	}
	changeBytes, err := json.Marshal(changeMsg)
	require.NoError(t, err)
	err = h.pub.Publish(wspubsub.WorkspaceEventChannel(h.workspace.OwnerID), changeBytes)
	require.NoError(t, err)

	e1 := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, workspacesdk.ConnectionWatchEvent{
		BuildUpdate: &workspacesdk.BuildUpdate{
			Transition: codersdk.WorkspaceTransitionStart,
			JobStatus:  codersdk.ProvisionerJobSucceeded,
		},
	}, e1)
	e2 := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, workspacesdk.ConnectionWatchEvent{AgentUpdate: &workspacesdk.AgentUpdate{
		ID:        agentID,
		Lifecycle: codersdk.WorkspaceAgentLifecycleCreated,
	}}, e2)

	// Finally, send in a change event for the agent. But first, program the mock for the expected query.
	h.db.EXPECT().GetWorkspaceAgentsByWorkspaceAndBuildNumber(
		gomock.Any(),
		database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
			WorkspaceID: h.workspace.ID,
			BuildNumber: 1,
		}).
		After(agent0).
		Times(1).
		Return([]database.WorkspaceAgent{
			{
				Name:           "test",
				ID:             agentID,
				LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			},
		}, nil)
	changeMsg = wspubsub.WorkspaceEvent{
		Kind:        wspubsub.WorkspaceEventKindAgentLifecycleUpdate,
		WorkspaceID: h.workspace.ID,
		AgentID:     &agentID,
	}
	changeBytes, err = json.Marshal(changeMsg)
	require.NoError(t, err)
	err = h.pub.Publish(wspubsub.WorkspaceEventChannel(h.workspace.OwnerID), changeBytes)
	require.NoError(t, err)

	e3 := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, workspacesdk.ConnectionWatchEvent{AgentUpdate: &workspacesdk.AgentUpdate{
		ID:        agentID,
		Lifecycle: codersdk.WorkspaceAgentLifecycleReady,
	}}, e3)
}

func TestWatcher_ClosedBeforeDial(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	h := newHarness(ctx, t, logger)
	h.watcher.Close()
	_, err := h.Dial(ctx, "wss://local.test/")
	var sdkError *codersdk.Error
	require.True(t, errors.As(err, &sdkError))
	require.Equal(t, http.StatusServiceUnavailable, sdkError.StatusCode())
}

func TestWatcher_ClosedAfterDial(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	h := newHarness(ctx, t, logger)

	h.db.EXPECT().GetLatestWorkspaceBuildWithStatusByWorkspaceID(gomock.Any(), h.workspace.ID).
		Times(1).
		Return(database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow{
			Transition:  database.WorkspaceTransitionStop,
			BuildNumber: 1,
			JobStatus:   database.ProvisionerJobStatusSucceeded,
			WorkspaceTable: database.WorkspaceTable{
				ID:             h.workspace.ID,
				OwnerID:        userID,
				OrganizationID: orgID,
			},
		}, nil)

	dec, err := h.Dial(ctx, "wss://local.test/")
	require.NoError(t, err)
	events := dec.Chan()
	_ = testutil.RequireReceive(ctx, t, events)

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		h.watcher.Close()
	}()

	e := testutil.RequireReceive(ctx, t, events)
	require.NotNil(t, e.Error)
	require.Equal(t, workspacesdk.WatchErrorServerShutdown, e.Error.Code)
	require.True(t, e.Error.Retryable)

	select {
	case <-ctx.Done():
		t.Fatal("context timed out")
	case _, ok := <-events:
		require.False(t, ok, "socket not closed")
	}
	testutil.TryReceive(ctx, t, closed)
}

// memberSubject builds an RBAC subject scoped as a basic org member, used to
// drive the watcher handler through dbauthz checks. Kept local to this test
// because no other package needs it.
func memberSubject(userID, orgID uuid.UUID) rbac.Subject {
	memberRole, err := rbac.RoleByName(rbac.RoleMember())
	if err != nil {
		panic(err)
	}
	orgMember, err := rolestore.TestingGetSystemRole(
		rbac.RoleOrgMember(),
		orgID,
		rbac.OrgSettings{ShareableWorkspaceOwners: rbac.ShareableWorkspaceOwnersNone},
	)
	if err != nil {
		panic(err)
	}
	return rbac.Subject{
		FriendlyName: "coderdtest-member",
		Email:        "member@coderd.test",
		Type:         rbac.SubjectTypeUser,
		ID:           userID.String(),
		Roles:        rbac.Roles{memberRole, orgMember},
		Scope:        rbac.ScopeAll,
	}.WithCachedASTValue()
}
