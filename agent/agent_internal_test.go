package agent

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
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
// VM), it must replay its latest lifecycle state to the new row. The helper
// sets an atomic flag and signals the lifecycle channel; reportLifecycle
// performs the actual rewind so lifecycleLastReportedIndex stays
// single-writer.
func TestResendLastLifecycleState(t *testing.T) {
	t.Parallel()

	t.Run("SetsFlagAndSignals", func(t *testing.T) {
		t.Parallel()
		a := &agent{
			lifecycleUpdate: make(chan struct{}, 1),
		}
		require.False(t, a.lifecycleResendRequested.Load())

		a.resendLastLifecycleState()

		require.True(t, a.lifecycleResendRequested.Load(),
			"flag should be set so reportLifecycle replays on next wake")
		select {
		case <-a.lifecycleUpdate:
		default:
			t.Fatal("lifecycleUpdate channel was not signaled")
		}
	})

	t.Run("SignalIsNonBlockingWhenChannelFull", func(t *testing.T) {
		t.Parallel()
		a := &agent{
			lifecycleUpdate: make(chan struct{}, 1),
		}
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
		require.True(t, a.lifecycleResendRequested.Load())
	})
}

// fakeAgentAPIClient is a tiny stub of proto.DRPCAgentClient28 that satisfies
// the interface via an embedded nil and overrides only the two methods
// handleManifest's reconnect path actually invokes. Calling any other method
// will panic at runtime, which is fine. The test only exercises the
// AgentID-change branch where script init and executor are skipped.
type fakeAgentAPIClient struct {
	proto.DRPCAgentClient28

	manifest *proto.Manifest
}

func (f *fakeAgentAPIClient) GetManifest(context.Context, *proto.GetManifestRequest) (*proto.Manifest, error) {
	return f.manifest, nil
}

func (*fakeAgentAPIClient) UpdateStartup(_ context.Context, req *proto.UpdateStartupRequest) (*proto.Startup, error) {
	return req.Startup, nil
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
// manifest's AgentID, the resend flag must be set so reportLifecycle replays
// the most recent state to the new workspace_agent row on its next wake.
// Issue: https://github.com/coder/coder/issues/18571.
func TestHandleManifestResendsLifecycleOnNewAgentID(t *testing.T) {
	t.Parallel()

	newIntegrationAgent := func(t *testing.T) *agent {
		t.Helper()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		a := &agent{
			hardCtx:         t.Context(),
			gracefulCtx:     t.Context(),
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

	t.Run("NewAgentID_SetsResendFlag", func(t *testing.T) {
		t.Parallel()
		a := newIntegrationAgent(t)

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

		manifestOK := newCheckpoint(a.logger)
		err := a.handleManifest(manifestOK)(a.hardCtx, fake)
		require.NoError(t, err)

		require.True(t, a.lifecycleResendRequested.Load(),
			"new AgentID should request a lifecycle replay")
		select {
		case <-a.lifecycleUpdate:
		default:
			t.Fatal("expected lifecycleUpdate signal on AgentID change")
		}
		require.Equal(t, 2, a.lifecycleLastReportedIndex,
			"handleManifest must not write lifecycleLastReportedIndex; only reportLifecycle does")
	})

	t.Run("SameAgentID_DoesNotSetFlag", func(t *testing.T) {
		t.Parallel()
		a := newIntegrationAgent(t)

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

		manifestOK := newCheckpoint(a.logger)
		err := a.handleManifest(manifestOK)(a.hardCtx, fake)
		require.NoError(t, err)

		require.False(t, a.lifecycleResendRequested.Load(),
			"same AgentID reconnect must not request a replay; preserves TestAgent_ReconnectNoLifecycleReemit")
		require.Equal(t, 2, a.lifecycleLastReportedIndex)
		select {
		case <-a.lifecycleUpdate:
			t.Fatal("did not expect lifecycleUpdate signal on same-AgentID reconnect")
		default:
		}
	})
}

// fakeLifecycleCapturingClient implements DRPCAgentClient28 just enough to
// capture UpdateLifecycle calls for assertion. All other methods panic.
// The captured channel is non-blocking on send so a buggy reporter that
// emits more than expected fails the test cleanly instead of deadlocking.
type fakeLifecycleCapturingClient struct {
	proto.DRPCAgentClient28

	captured chan *proto.UpdateLifecycleRequest
}

func (f *fakeLifecycleCapturingClient) UpdateLifecycle(ctx context.Context, req *proto.UpdateLifecycleRequest) (*proto.Lifecycle, error) {
	select {
	case f.captured <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return req.Lifecycle, nil
}

// TestReportLifecycleRewindRefreshesChangedAt covers the resend path inside
// reportLifecycle. When lifecycleResendRequested is set, the reporter must
// rewind lifecycleLastReportedIndex AND refresh the latest entry's ChangedAt
// so the server's new workspace_agent row gets started_at/ready_at at the
// resend time, not the original build's stale wall clock. See DEREM-6 in the
// coder-agents-review on coder/coder#25406.
func TestReportLifecycleRewindRefreshesChangedAt(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	staleTime := time.Now().Add(-time.Hour)

	a := &agent{
		hardCtx:           ctx,
		gracefulCtx:       ctx,
		logger:            logger,
		lifecycleUpdate:   make(chan struct{}, 1),
		lifecycleReported: make(chan codersdk.WorkspaceAgentLifecycle, 1),
		lifecycleStates: []agentsdk.PostLifecycleRequest{
			{State: codersdk.WorkspaceAgentLifecycleCreated, ChangedAt: staleTime},
			{State: codersdk.WorkspaceAgentLifecycleStarting, ChangedAt: staleTime},
			{State: codersdk.WorkspaceAgentLifecycleReady, ChangedAt: staleTime},
		},
		lifecycleLastReportedIndex: 2,
	}

	// Simulate a reconnect to a new workspace_agent row.
	a.lifecycleResendRequested.Store(true)
	a.lifecycleUpdate <- struct{}{}

	captured := make(chan *proto.UpdateLifecycleRequest, 1)
	fake := &fakeLifecycleCapturingClient{captured: captured}

	reporterDone := make(chan struct{})
	go func() {
		defer close(reporterDone)
		_ = a.reportLifecycle(ctx, fake)
	}()

	select {
	case req := <-captured:
		require.Equal(t, proto.Lifecycle_READY, req.Lifecycle.State,
			"replay should emit the latest state (Ready)")
		require.True(t, req.Lifecycle.ChangedAt.AsTime().After(staleTime),
			"ChangedAt should be refreshed away from the original build's wall clock")
		require.WithinDuration(t, time.Now(), req.Lifecycle.ChangedAt.AsTime(), testutil.WaitShort,
			"refreshed ChangedAt should be close to time.Now()")
	case <-time.After(testutil.WaitShort):
		t.Fatal("reporter did not emit a replayed lifecycle state")
	}

	// Reporter increments lifecycleLastReportedIndex after UpdateLifecycle
	// returns, so we wait for the post-send notification on lifecycleReported
	// before asserting on the index.
	select {
	case <-a.lifecycleReported:
	case <-time.After(testutil.WaitShort):
		t.Fatal("reporter did not signal lifecycleReported after replay")
	}

	require.False(t, a.lifecycleResendRequested.Load(),
		"reporter must consume the resend flag")
	require.Equal(t, 2, a.lifecycleLastReportedIndex,
		"after the rewind + replay, the reporter should have advanced back to len-1")

	cancel()
	<-reporterDone
}

func TestClassifyCoordinatorRPCExit(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name      string
		ctx       context.Context
		retErr    error
		reason    codersdk.DisconnectReason
		initiator codersdk.DisconnectInitiator
	}{
		{
			name:      "local shutdown, no error",
			ctx:       canceled,
			retErr:    nil,
			reason:    codersdk.DisconnectReasonServerShutdown,
			initiator: codersdk.DisconnectInitiatorAgent,
		},
		{
			name:      "local shutdown, with cleanup error",
			ctx:       canceled,
			retErr:    xerrors.New("close timed out"),
			reason:    codersdk.DisconnectReasonServerShutdown,
			initiator: codersdk.DisconnectInitiatorAgent,
		},
		{
			name:      "remote graceful, no error",
			ctx:       context.Background(),
			retErr:    nil,
			reason:    codersdk.DisconnectReasonGraceful,
			initiator: codersdk.DisconnectInitiatorServer,
		},
		{
			name:      "stream broke unexpectedly",
			ctx:       context.Background(),
			retErr:    xerrors.New("read: connection reset"),
			reason:    codersdk.DisconnectReasonNetworkError,
			initiator: codersdk.DisconnectInitiatorNetwork,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reason, initiator := classifyCoordinatorRPCExit(tc.ctx, tc.retErr)
			require.Equal(t, tc.reason, reason)
			require.Equal(t, tc.initiator, initiator)
		})
	}
}
