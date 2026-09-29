package agent

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/codersdk"
	agentsdk "github.com/coder/coder/v2/codersdk/agentsdk"
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
