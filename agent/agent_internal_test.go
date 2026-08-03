package agent

import (
	"context"
	"net/netip"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	agentsdk "github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
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
func TestInvalidateTailnetGeneration(t *testing.T) {
	t.Parallel()

	uut := &agent{
		networkGeneration: 4,
		statsReporter:     &statsReporter{},
	}

	network, networkTransitionDone, reset := uut.invalidateTailnetGeneration(3)
	require.Nil(t, network)
	require.Nil(t, networkTransitionDone)
	require.False(t, reset)
	require.NotNil(t, uut.statsReporter)
	require.Equal(t, uint64(4), uut.networkGeneration)

	network, networkTransitionDone, reset = uut.invalidateTailnetGeneration(4)
	require.Nil(t, network)
	require.NotNil(t, networkTransitionDone)
	require.Equal(t, networkTransitionDone, uut.networkTransitionDone)
	require.True(t, reset)
	require.Nil(t, uut.statsReporter)
	require.Equal(t, uint64(5), uut.networkGeneration)
	uut.completeNetworkTransition(networkTransitionDone)
	require.Nil(t, uut.networkTransitionDone)
	_ = testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, networkTransitionDone)

	uut.closing = true
	_, _, reset = uut.invalidateTailnetGeneration(5)
	require.False(t, reset)
}

func TestRecoverTailnetAfterWatchdog(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	network, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{tailnet.TailscaleServicePrefix.RandomPrefix()},
		Logger:    logger.Named("tailnet"),
	})
	require.NoError(t, err)

	uut := &agent{
		logger:            logger,
		hardCtx:           ctx,
		tailnetWatchdog:   make(chan struct{}, 1),
		networkGeneration: 8,
		network:           network,
	}
	uut.reportTailnetWatchdogTimeout(7, "UpdateStatus")
	require.Same(t, network, uut.network)
	select {
	case <-uut.tailnetWatchdog:
		t.Fatal("stale watchdog event was queued")
	default:
	}

	uut.reportTailnetWatchdogTimeout(8, "Reconfig")

	err = uut.recoverTailnetAfterWatchdog(ctx)
	require.ErrorIs(t, err, errTailnetWatchdogTimeout)
	require.ErrorContains(t, err, "Reconfig")
	require.Equal(t, uint64(9), uut.networkGeneration)
	require.Nil(t, uut.network)
	require.Nil(t, uut.networkTransitionDone)
	_ = testutil.TryReceive(ctx, t, network.Closed())
	require.NoError(t, network.Close())

	uut.closing = true
	uut.network = network
	require.NotPanics(t, func() {
		uut.reportTailnetWatchdogTimeout(9, "Ping")
	})
}

func TestDiscardHandledTailnetWatchdogEvents(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	uut := &agent{
		logger:            slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		hardCtx:           ctx,
		tailnetWatchdog:   make(chan struct{}, 1),
		networkGeneration: 8,
	}
	uut.reportTailnetWatchdogTimeout(8, "Reconfig")
	require.Equal(t, uint64(9), uut.networkGeneration)

	uut.discardHandledTailnetWatchdogEvents()
	select {
	case <-uut.tailnetWatchdog:
		t.Fatal("handled watchdog event remained queued")
	default:
	}
}

func TestQueueTailnetWatchdogEventKeepsNewestGeneration(t *testing.T) {
	t.Parallel()

	uut := &agent{
		tailnetWatchdog: make(chan struct{}, 1),
	}
	assertNewest := func(first, second, want tailnetWatchdogEvent) {
		t.Helper()
		uut.queueTailnetWatchdogEvent(first)
		uut.queueTailnetWatchdogEvent(second)
		<-uut.tailnetWatchdog
		got, ok := uut.takeTailnetWatchdogEvent()
		require.True(t, ok)
		require.Equal(t, want, got)
	}
	older := tailnetWatchdogEvent{generation: 8, operation: "Reconfig"}
	newer := tailnetWatchdogEvent{generation: 10, operation: "UpdateStatus"}
	assertNewest(older, newer, newer)
	assertNewest(newer, older, newer)
}
