package agent

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	agentsdk "github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// platformAbsPath constructs an absolute path that is valid
// on the current platform. On Windows, paths must include a
// drive letter to be considered absolute.
func platformAbsPath(parts ...string) string {
	if runtime.GOOS == "windows" {
		return `C:\` + filepath.Join(parts...)
	}
	return "/" + filepath.Join(parts...)
}

// TestReportConnectionEmpty tests that reportConnection() doesn't choke if given an empty IP string, which is what we
// send if we cannot get the remote address.
func TestReportConnectionEmpty(t *testing.T) {
	t.Parallel()
	connID := uuid.UUID{1}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := testutil.Context(t, testutil.WaitShort)

	uut := &agent{
		hardCtx: ctx,
		logger:  logger,
	}
	disconnected := uut.reportConnection(connID, proto.Connection_TYPE_UNSPECIFIED, "")

	require.Len(t, uut.reportConnections, 1)
	req0 := uut.reportConnections[0]
	require.Equal(t, proto.Connection_TYPE_UNSPECIFIED, req0.GetConnection().GetType())
	require.Equal(t, "", req0.GetConnection().Ip)
	require.Equal(t, connID[:], req0.GetConnection().GetId())
	require.Equal(t, proto.Connection_CONNECT, req0.GetConnection().GetAction())

	disconnected(0, "because")
	require.Len(t, uut.reportConnections, 2)
	req1 := uut.reportConnections[1]
	require.Equal(t, proto.Connection_TYPE_UNSPECIFIED, req1.GetConnection().GetType())
	require.Equal(t, "", req1.GetConnection().Ip)
	require.Equal(t, connID[:], req1.GetConnection().GetId())
	require.Equal(t, proto.Connection_DISCONNECT, req1.GetConnection().GetAction())
	require.Equal(t, "because", req1.GetConnection().GetReason())
}

func TestContextConfigAPI_InitOnce(t *testing.T) {
	t.Parallel()

	// After the fix, contextConfigAPI is set once in init() and
	// never reassigned. Resolve() evaluates lazily via the
	// manifest, so there is no concurrent write to race with.
	dir1 := platformAbsPath("dir1")
	dir2 := platformAbsPath("dir2")

	a := &agent{}
	a.manifest.Store(&agentsdk.Manifest{Directory: dir1})
	a.contextConfigAPI = agentcontextconfig.NewAPI(func() string {
		if m := a.manifest.Load(); m != nil {
			return m.Directory
		}
		return ""
	}, agentcontextconfig.Config{})

	mcpFiles1 := a.contextConfigAPI.MCPConfigFiles()
	require.NotEmpty(t, mcpFiles1)
	require.Contains(t, mcpFiles1[0], dir1)

	// Simulate manifest update on reconnection -- no field
	// reassignment needed, the lazy closure picks it up.
	a.manifest.Store(&agentsdk.Manifest{Directory: dir2})
	mcpFiles2 := a.contextConfigAPI.MCPConfigFiles()
	require.NotEmpty(t, mcpFiles2)
	require.Contains(t, mcpFiles2[0], dir2)
}

// TestResendLastLifecycleState covers the helper added to fix
// https://github.com/coder/coder/issues/18571. When the agent reconnects to a
// new workspace_agent row (new build after a suspend/resume on a long-lived
// VM), it must replay its latest lifecycle state to the new row.
func TestResendLastLifecycleState(t *testing.T) {
	t.Parallel()

	newAgent := func(states ...codersdk.WorkspaceAgentLifecycle) *agent {
		lifecycleStates := make([]agentsdk.PostLifecycleRequest, 0, len(states))
		for _, s := range states {
			lifecycleStates = append(lifecycleStates, agentsdk.PostLifecycleRequest{State: s})
		}
		return &agent{
			lifecycleUpdate:            make(chan struct{}, 1),
			lifecycleStates:            lifecycleStates,
			lifecycleLastReportedIndex: len(lifecycleStates) - 1,
		}
	}

	t.Run("OnlyCreated_NoRewind", func(t *testing.T) {
		t.Parallel()
		a := newAgent(codersdk.WorkspaceAgentLifecycleCreated)
		a.resendLastLifecycleState()
		require.Equal(t, 0, a.lifecycleLastReportedIndex,
			"with only Created in history, the index should stay at 0")
		// Channel should have been signaled (non-blocking send into buffered chan).
		select {
		case <-a.lifecycleUpdate:
		default:
			t.Fatal("lifecycleUpdate channel was not signaled")
		}
	})

	t.Run("StartingReady_RewindsToReadyMinus1", func(t *testing.T) {
		t.Parallel()
		a := newAgent(
			codersdk.WorkspaceAgentLifecycleCreated,
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleReady,
		)
		require.Equal(t, 2, a.lifecycleLastReportedIndex)
		a.resendLastLifecycleState()
		// After rewind, the reporter loop will send lifecycleStates[index+1]
		// which is Ready.
		require.Equal(t, 1, a.lifecycleLastReportedIndex,
			"index should point one before the latest state so the reporter re-sends it")
		select {
		case <-a.lifecycleUpdate:
		default:
			t.Fatal("lifecycleUpdate channel was not signaled")
		}
	})

	t.Run("ShuttingDown_RewindsToShuttingDownMinus1", func(t *testing.T) {
		t.Parallel()
		a := newAgent(
			codersdk.WorkspaceAgentLifecycleCreated,
			codersdk.WorkspaceAgentLifecycleStarting,
			codersdk.WorkspaceAgentLifecycleReady,
			codersdk.WorkspaceAgentLifecycleShuttingDown,
		)
		a.resendLastLifecycleState()
		require.Equal(t, 2, a.lifecycleLastReportedIndex,
			"during shutdown reconnect, only the latest (ShuttingDown) should re-emit, not earlier states")
	})

	t.Run("SignalIsNonBlockingWhenChannelFull", func(t *testing.T) {
		t.Parallel()
		a := newAgent(
			codersdk.WorkspaceAgentLifecycleCreated,
			codersdk.WorkspaceAgentLifecycleReady,
		)
		// Pre-fill the buffered channel; the helper must not block.
		a.lifecycleUpdate <- struct{}{}
		done := make(chan struct{})
		go func() {
			a.resendLastLifecycleState()
			close(done)
		}()
		select {
		case <-done:
		case <-testutil.Context(t, testutil.WaitShort).Done():
			t.Fatal("resendLastLifecycleState blocked on a full channel")
		}
	})
}

// fakeAgentAPIClient is a tiny stub of proto.DRPCAgentClient28 that satisfies
// the interface via an embedded nil and overrides only the two methods
// handleManifest's reconnect path actually invokes. Calling any other method
// will panic at runtime, which is fine -- the test only exercises the
// AgentID-change branch where script init + executor are skipped.
type fakeAgentAPIClient struct {
	proto.DRPCAgentClient28

	manifest *proto.Manifest
}

func (f *fakeAgentAPIClient) GetManifest(context.Context, *proto.GetManifestRequest) (*proto.Manifest, error) {
	return f.manifest, nil
}

func (*fakeAgentAPIClient) UpdateStartup(context.Context, *proto.UpdateStartupRequest) (*proto.Startup, error) {
	return nil, nil
}

// fakeAgentClient stubs out agent.Client with no-op DERPMap rewriting.
// handleManifest only touches client.RewriteDERPMap on this path.
type fakeAgentClient struct {
	Client
}

func (*fakeAgentClient) RewriteDERPMap(_ *tailcfg.DERPMap) {}

// TestHandleManifestResendsLifecycleOnNewAgentID exercises the integration
// between handleManifest and resendLastLifecycleState: when handleManifest
// fetches a manifest whose AgentID differs from the previously-stored
// manifest's AgentID, the lifecycle reporter must be rewound so the next
// reportLifecycle wake-up replays the latest state to the new
// workspace_agent row. Issue: https://github.com/coder/coder/issues/18571.
func TestHandleManifestResendsLifecycleOnNewAgentID(t *testing.T) {
	t.Parallel()

	mkAgent := func() *agent {
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		a := &agent{
			hardCtx:         ctx,
			gracefulCtx:     ctx,
			logger:          logger,
			filesystem:      afero.NewMemMapFs(),
			client:          &fakeAgentClient{},
			lifecycleUpdate: make(chan struct{}, 1),
			lifecycleStates: []agentsdk.PostLifecycleRequest{
				{State: codersdk.WorkspaceAgentLifecycleCreated},
				{State: codersdk.WorkspaceAgentLifecycleStarting},
				{State: codersdk.WorkspaceAgentLifecycleReady},
			},
			lifecycleLastReportedIndex: 2,
		}
		a.secrets.Store(new([]agentsdk.WorkspaceSecret))
		return a
	}

	t.Run("NewAgentID_TriggersResend", func(t *testing.T) {
		t.Parallel()
		a := mkAgent()

		oldAgentID := uuid.New()
		newAgentID := uuid.New()
		workspaceID := uuid.New()

		// Pre-populate a.manifest with the "old" manifest so the
		// next handleManifest call sees oldManifest != nil.
		a.manifest.Store(&agentsdk.Manifest{
			AgentID:     oldAgentID,
			WorkspaceID: workspaceID,
			Directory:   "/tmp",
		})

		// Drain any signal from agent init.
		select {
		case <-a.lifecycleUpdate:
		default:
		}

		fake := &fakeAgentAPIClient{
			manifest: &proto.Manifest{
				AgentId:     newAgentID[:],
				WorkspaceId: workspaceID[:],
				Directory:   "/tmp",
			},
		}

		mok := newCheckpoint(a.logger)
		err := a.handleManifest(mok)(a.hardCtx, fake)
		require.NoError(t, err)

		require.Equal(t, 1, a.lifecycleLastReportedIndex,
			"new AgentID should rewind index so the reporter re-emits Ready")
		select {
		case <-a.lifecycleUpdate:
		default:
			t.Fatal("expected lifecycleUpdate signal on AgentID change")
		}
	})

	t.Run("SameAgentID_NoResend", func(t *testing.T) {
		t.Parallel()
		a := mkAgent()

		sameAgentID := uuid.New()
		workspaceID := uuid.New()

		a.manifest.Store(&agentsdk.Manifest{
			AgentID:     sameAgentID,
			WorkspaceID: workspaceID,
			Directory:   "/tmp",
		})
		select {
		case <-a.lifecycleUpdate:
		default:
		}

		fake := &fakeAgentAPIClient{
			manifest: &proto.Manifest{
				AgentId:     sameAgentID[:],
				WorkspaceId: workspaceID[:],
				Directory:   "/tmp",
			},
		}

		mok := newCheckpoint(a.logger)
		err := a.handleManifest(mok)(a.hardCtx, fake)
		require.NoError(t, err)

		require.Equal(t, 2, a.lifecycleLastReportedIndex,
			"same AgentID reconnect must not rewind the reporter (preserves TestAgent_ReconnectNoLifecycleReemit invariant)")
		select {
		case <-a.lifecycleUpdate:
			t.Fatal("did not expect lifecycleUpdate signal on same-AgentID reconnect")
		default:
		}
	})
}
