package agentsocket_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentsocket"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/testutil"
)

// fakeAgentAPI implements just the UpdateAppStatus method of
// DRPCAgentClient211 for testing. Calling any other method will panic.
type fakeAgentAPI struct {
	agentproto.DRPCAgentClient211
	updateAppStatus func(context.Context, *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error)
}

func (m *fakeAgentAPI) UpdateAppStatus(ctx context.Context, req *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error) {
	return m.updateAppStatus(ctx, req)
}

// fakeWorkspaceIdentity stands in for what a real workspace_agent knows about
// itself. known is separate from id so that a test can express the state
// before the first manifest has arrived.
type fakeWorkspaceIdentity struct {
	id         uuid.UUID
	known      bool
	credential string
}

func (f fakeWorkspaceIdentity) WorkspaceID() (uuid.UUID, bool) { return f.id, f.known }
func (f fakeWorkspaceIdentity) Credential() string             { return f.credential }

// newSocketClient creates a DRPC client connected to the Unix socket at the given path.
func newSocketClient(ctx context.Context, t *testing.T, socketPath string) *agentsocket.Client {
	t.Helper()

	client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(socketPath))
	t.Cleanup(func() {
		_ = client.Close()
	})
	require.NoError(t, err)

	return client
}

func TestDRPCAgentSocketService(t *testing.T) {
	t.Parallel()

	t.Run("Ping", func(t *testing.T) {
		t.Parallel()

		socketPath := testutil.AgentSocketPath(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		server, err := agentsocket.NewServer(
			slog.Make().Leveled(slog.LevelDebug),
			agentsocket.WithPath(socketPath),
		)
		require.NoError(t, err)
		defer server.Close()

		client := newSocketClient(ctx, t, socketPath)

		err = client.Ping(ctx)
		require.NoError(t, err)
	})

	// CreateAIAgent verifies the caller against what the workspace_agent knows
	// about itself, then forwards to coderd. These cases cover the verifying,
	// and all of them outlive the proof of concept: a caller that cannot show
	// it belongs to this workspace has no business creating an AI agent in it.
	t.Run("CreateAIAgent", func(t *testing.T) {
		t.Parallel()

		const ownCredential = "the workspace_agent's own credential"
		ownWorkspace := uuid.New()

		for _, tc := range []struct {
			name                string
			identity            agentsocket.WorkspaceIdentity
			workspaceID         uuid.UUID
			workspaceCredential []byte
			wantErr             string
		}{
			{
				// Fails closed. An agent that cannot verify refuses rather
				// than waving the caller through.
				name:                "NoIdentity",
				identity:            nil,
				workspaceID:         ownWorkspace,
				workspaceCredential: []byte(ownCredential),
				wantErr:             "cannot verify callers",
			},
			{
				name:                "WorkspaceNotYetKnown",
				identity:            fakeWorkspaceIdentity{credential: ownCredential},
				workspaceID:         ownWorkspace,
				workspaceCredential: []byte(ownCredential),
				wantErr:             "does not yet know its workspace",
			},
			{
				name:                "WrongWorkspace",
				identity:            fakeWorkspaceIdentity{id: ownWorkspace, known: true, credential: ownCredential},
				workspaceID:         uuid.New(),
				workspaceCredential: []byte(ownCredential),
				wantErr:             "is not this workspace",
			},
			{
				name:                "NoWorkspaceCredential",
				identity:            fakeWorkspaceIdentity{id: ownWorkspace, known: true, credential: ownCredential},
				workspaceID:         ownWorkspace,
				workspaceCredential: nil,
				wantErr:             "does not match this workspace",
			},
			{
				name:                "WrongCredential",
				identity:            fakeWorkspaceIdentity{id: ownWorkspace, known: true, credential: ownCredential},
				workspaceID:         ownWorkspace,
				workspaceCredential: []byte("a credential from somewhere else"),
				wantErr:             "does not match this workspace",
			},
			{
				// Verification passes, so the request reaches the forwarding
				// guard. The server under test never had SetAgentAPI called.
				name:                "NotConnectedToCoderd",
				identity:            fakeWorkspaceIdentity{id: ownWorkspace, known: true, credential: ownCredential},
				workspaceID:         ownWorkspace,
				workspaceCredential: []byte(ownCredential),
				wantErr:             "agent not connected to coderd",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				socketPath := testutil.AgentSocketPath(t)
				ctx := testutil.Context(t, testutil.WaitShort)
				opts := []agentsocket.Option{agentsocket.WithPath(socketPath)}
				if tc.identity != nil {
					opts = append(opts, agentsocket.WithWorkspaceIdentity(tc.identity))
				}
				server, err := agentsocket.NewServer(
					slog.Make().Leveled(slog.LevelDebug),
					opts...,
				)
				require.NoError(t, err)
				defer server.Close()

				client := newSocketClient(ctx, t, socketPath)

				id, credential, err := client.CreateAIAgent(ctx, tc.workspaceID, tc.workspaceCredential)
				require.ErrorContains(t, err, tc.wantErr)
				require.Equal(t, uuid.Nil, id, "a rejected request mints nothing")
				require.Empty(t, credential, "a rejected request issues nothing")
			})
		}
	})

	t.Run("SyncStart", func(t *testing.T) {
		t.Parallel()

		t.Run("NewUnit", func(t *testing.T) {
			t.Parallel()
			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)
		})

		t.Run("UnitAlreadyStarted", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// First Start
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)
			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)

			// Second Start
			err = client.SyncStart(ctx, "test-unit")
			require.ErrorContains(t, err, unit.ErrSameStatusAlreadySet.Error())

			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)
		})

		t.Run("UnitAlreadyCompleted", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// First start
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)

			// Complete the unit
			err = client.SyncComplete(ctx, "test-unit")
			require.NoError(t, err)

			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusComplete, status.Status)

			// Second start
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)
		})

		t.Run("UnitNotReady", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			err = client.SyncWant(ctx, "test-unit", "dependency-unit")
			require.NoError(t, err)

			err = client.SyncStart(ctx, "test-unit")
			require.ErrorContains(t, err, "unit not ready")

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusPending, status.Status)
			require.False(t, status.IsReady)
		})
	})

	t.Run("SyncWant", func(t *testing.T) {
		t.Parallel()

		t.Run("NewUnits", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// If dependency units are not registered, they are registered automatically
			err = client.SyncWant(ctx, "test-unit", "dependency-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Len(t, status.Dependencies, 1)
			require.Equal(t, unit.ID("dependency-unit"), status.Dependencies[0].DependsOn)
			require.Equal(t, unit.StatusComplete, status.Dependencies[0].RequiredStatus)
		})

		t.Run("DependencyAlreadyRegistered", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// Start the dependency unit
			err = client.SyncStart(ctx, "dependency-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "dependency-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)

			// Add the dependency after the dependency unit has already started
			err = client.SyncWant(ctx, "test-unit", "dependency-unit")

			// Dependencies can be added even if the dependency unit has already started
			require.NoError(t, err)

			// The dependency is now reflected in the test unit's status
			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.ID("dependency-unit"), status.Dependencies[0].DependsOn)
			require.Equal(t, unit.StatusComplete, status.Dependencies[0].RequiredStatus)
		})

		t.Run("DependencyAddedAfterDependentStarted", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// Start the dependent unit
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)

			// Add the dependency after the dependency unit has already started
			err = client.SyncWant(ctx, "test-unit", "dependency-unit")

			// Dependencies can be added even if the dependent unit has already started.
			// The dependency applies the next time a unit is started. The current status is not updated.
			// This is to allow flexible dependency management. It does mean that users of this API should
			// take care to add dependencies before they start their dependent units.
			require.NoError(t, err)

			// The dependency is now reflected in the test unit's status
			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.ID("dependency-unit"), status.Dependencies[0].DependsOn)
			require.Equal(t, unit.StatusComplete, status.Dependencies[0].RequiredStatus)
		})
	})

	t.Run("SyncReady", func(t *testing.T) {
		t.Parallel()

		t.Run("UnregisteredUnit", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			ready, err := client.SyncReady(ctx, "unregistered-unit")
			require.NoError(t, err)
			require.True(t, ready)
		})

		t.Run("UnitNotReady", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// Register a unit with an unsatisfied dependency
			err = client.SyncWant(ctx, "test-unit", "dependency-unit")
			require.NoError(t, err)

			// Check readiness - should be false because dependency is not satisfied
			ready, err := client.SyncReady(ctx, "test-unit")
			require.NoError(t, err)
			require.False(t, ready)
		})

		t.Run("UnitReady", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// Register a unit with no dependencies - should be ready immediately
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			// Check readiness - should be true
			ready, err := client.SyncReady(ctx, "test-unit")
			require.NoError(t, err)
			require.True(t, ready)

			// Also test a unit with satisfied dependencies
			err = client.SyncWant(ctx, "dependent-unit", "test-unit")
			require.NoError(t, err)

			// Complete the dependency
			err = client.SyncComplete(ctx, "test-unit")
			require.NoError(t, err)

			// Now dependent-unit should be ready
			ready, err = client.SyncReady(ctx, "dependent-unit")
			require.NoError(t, err)
			require.True(t, ready)
		})
	})

	t.Run("UpdateAppStatus", func(t *testing.T) {
		t.Parallel()

		t.Run("NotConnected", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			_, err = client.UpdateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
				Slug:    "test-app",
				State:   agentproto.UpdateAppStatusRequest_WORKING,
				Message: "doing stuff",
			})
			require.ErrorContains(t, err, "not connected")
		})

		t.Run("ForwardsToAgentAPI", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			var gotReq *agentproto.UpdateAppStatusRequest
			mock := &fakeAgentAPI{
				updateAppStatus: func(_ context.Context, req *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error) {
					gotReq = req
					return &agentproto.UpdateAppStatusResponse{}, nil
				},
			}
			server.SetAgentAPI(mock)

			client := newSocketClient(ctx, t, socketPath)

			resp, err := client.UpdateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
				Slug:    "test-app",
				State:   agentproto.UpdateAppStatusRequest_IDLE,
				Message: "all done",
				Uri:     "https://example.com",
			})
			require.NoError(t, err)
			require.NotNil(t, resp)

			require.NotNil(t, gotReq)
			require.Equal(t, "test-app", gotReq.Slug)
			require.Equal(t, agentproto.UpdateAppStatusRequest_IDLE, gotReq.State)
			require.Equal(t, "all done", gotReq.Message)
			require.Equal(t, "https://example.com", gotReq.Uri)
		})

		t.Run("ForwardsError", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			mock := &fakeAgentAPI{
				updateAppStatus: func(context.Context, *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error) {
					return nil, xerrors.New("app not found")
				},
			}
			server.SetAgentAPI(mock)

			client := newSocketClient(ctx, t, socketPath)

			_, err = client.UpdateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
				Slug:    "nonexistent",
				State:   agentproto.UpdateAppStatusRequest_WORKING,
				Message: "testing",
			})
			require.ErrorContains(t, err, "app not found")
		})

		t.Run("ClearAgentAPI", func(t *testing.T) {
			t.Parallel()

			socketPath := testutil.AgentSocketPath(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			mock := &fakeAgentAPI{
				updateAppStatus: func(context.Context, *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error) {
					return &agentproto.UpdateAppStatusResponse{}, nil
				},
			}
			server.SetAgentAPI(mock)
			server.ClearAgentAPI()

			client := newSocketClient(ctx, t, socketPath)

			_, err = client.UpdateAppStatus(ctx, &agentproto.UpdateAppStatusRequest{
				Slug:    "test-app",
				State:   agentproto.UpdateAppStatusRequest_WORKING,
				Message: "should fail",
			})
			require.ErrorContains(t, err, "not connected")
		})
	})
}
