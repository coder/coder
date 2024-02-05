package agentscripts_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentscripts"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestExecuteBasic(t *testing.T) {
	t.Parallel()
	logs := make(chan agentsdk.PatchLogs, 1)
	runner := setup(t, func(ctx context.Context, req agentsdk.PatchLogs) error {
		logs <- req
		return nil
	})
	defer runner.Close()
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		Script: "echo hello",
	}})
	require.NoError(t, err)
	require.NoError(t, runner.Execute(context.Background(), func(script codersdk.WorkspaceAgentScript) bool {
		return true
	}))
	log := <-logs
	require.Equal(t, "hello", log.Logs[0].Output)
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	runner := setup(t, nil)
	defer runner.Close()
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		Script:  "sleep infinity",
		Timeout: time.Millisecond,
	}})
	require.NoError(t, err)
	require.ErrorIs(t, runner.Execute(context.Background(), nil), agentscripts.ErrTimeout)
}

// TestCronClose exists because cron.Run() can happen after cron.Close().
// If this happens, there used to be a deadlock.
func TestCronClose(t *testing.T) {
	t.Parallel()
	runner := agentscripts.New(agentscripts.Options{})
	runner.StartCron()
	require.NoError(t, runner.Close(), "close runner")
}

func setup(t *testing.T, patchLogs func(ctx context.Context, req agentsdk.PatchLogs) error) *agentscripts.Runner {
	t.Helper()
	if patchLogs == nil {
		// noop
		patchLogs = func(ctx context.Context, req agentsdk.PatchLogs) error {
			return nil
		}
	}
	fs := afero.NewMemMapFs()
	logger := slogtest.Make(t, nil)
	s, err := agentssh.NewServer(context.Background(), logger, prometheus.NewRegistry(), fs, 0, "")
	require.NoError(t, err)
	s.AgentToken = func() string { return "" }
	s.Manifest = atomic.NewPointer(&agentsdk.Manifest{})
	t.Cleanup(func() {
		_ = s.Close()
	})
	return agentscripts.New(agentscripts.Options{
		LogDir:     t.TempDir(),
		Logger:     logger,
		SSHServer:  s,
		Filesystem: fs,
		PatchLogs:  patchLogs,
	})
}
