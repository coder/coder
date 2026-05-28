package agent

import (
	"context"
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
