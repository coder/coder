package agentscripts_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentscripts"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestExecuteBasic(t *testing.T) {
	t.Parallel()
	logs := make(chan agentsdk.PatchLogs, 1)
	runner := setup(t, func(ctx context.Context, req agentsdk.PatchLogs) error {
		select {
		case <-ctx.Done():
		case logs <- req:
		}
		return nil
	})
	defer runner.Close()
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: uuid.New(),
		Script:      "echo hello",
	}})
	require.NoError(t, err)
	require.NoError(t, runner.Execute(context.Background(), func(script codersdk.WorkspaceAgentScript) bool {
		return true
	}))
	log := <-logs
	require.Equal(t, "hello", log.Logs[0].Output)
}

func TestEnv(t *testing.T) {
	t.Parallel()
	logs := make(chan agentsdk.PatchLogs, 2)
	runner := setup(t, func(ctx context.Context, req agentsdk.PatchLogs) error {
		select {
		case <-ctx.Done():
		case logs <- req:
		}
		return nil
	})
	defer runner.Close()
	id := uuid.New()
	script := "echo $CODER_SCRIPT_DATA_DIR\necho $CODER_SCRIPT_BIN_DIR\n"
	if runtime.GOOS == "windows" {
		script = `
			cmd.exe /c echo %CODER_SCRIPT_DATA_DIR%
			cmd.exe /c echo %CODER_SCRIPT_BIN_DIR%
		`
	}
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: id,
		Script:      script,
	}})
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitLong)

	testutil.Go(t, func() {
		err := runner.Execute(ctx, func(script codersdk.WorkspaceAgentScript) bool {
			return true
		})
		assert.NoError(t, err)
	})

	var log []agentsdk.Log
	for {
		select {
		case <-ctx.Done():
			require.Fail(t, "timed out waiting for logs")
		case l := <-logs:
			for _, l := range l.Logs {
				t.Logf("log: %s", l.Output)
			}
			log = append(log, l.Logs...)
		}
		if len(log) >= 2 {
			break
		}
	}
	require.Contains(t, log[0].Output, filepath.Join(runner.DataDir(), id.String()))
	require.Contains(t, log[1].Output, runner.ScriptBinDir())
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	runner := setup(t, nil)
	defer runner.Close()
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: uuid.New(),
		Script:      "sleep infinity",
		Timeout:     time.Millisecond,
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
	s, err := agentssh.NewServer(context.Background(), logger, prometheus.NewRegistry(), fs, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
	})
	return agentscripts.New(agentscripts.Options{
		LogDir:      t.TempDir(),
		DataDirBase: t.TempDir(),
		Logger:      logger,
		SSHServer:   s,
		Filesystem:  fs,
		PatchLogs:   patchLogs,
	})
}
