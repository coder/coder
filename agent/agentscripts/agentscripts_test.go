package agentscripts_test

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentscripts"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestExecuteBasic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	fLogger := newFakeScriptLogger()
	runner := setup(t, func(uuid2 uuid.UUID) agentscripts.ScriptLogger {
		return fLogger
	})
	defer runner.Close()
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: uuid.New(),
		Script:      "echo hello",
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)
	require.NoError(t, runner.Execute(context.Background(), agentscripts.ExecuteAllScripts))
	log := testutil.RequireRecvCtx(ctx, t, fLogger.logs)
	require.Equal(t, "hello", log.Output)
}

func TestEnv(t *testing.T) {
	t.Parallel()
	fLogger := newFakeScriptLogger()
	runner := setup(t, func(uuid2 uuid.UUID) agentscripts.ScriptLogger {
		return fLogger
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
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: id,
		Script:      script,
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitLong)

	done := testutil.Go(t, func() {
		err := runner.Execute(ctx, agentscripts.ExecuteAllScripts)
		assert.NoError(t, err)
	})
	defer func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
	}()

	var log []agentsdk.Log
	for {
		select {
		case <-ctx.Done():
			require.Fail(t, "timed out waiting for logs")
		case l := <-fLogger.logs:
			t.Logf("log: %s", l.Output)
			log = append(log, l)
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
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: uuid.New(),
		Script:      "sleep infinity",
		Timeout:     time.Millisecond,
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)
	require.ErrorIs(t, runner.Execute(context.Background(), agentscripts.ExecuteAllScripts), agentscripts.ErrTimeout)
}

func TestScriptReportsTiming(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	fLogger := newFakeScriptLogger()
	runner := setup(t, func(uuid2 uuid.UUID) agentscripts.ScriptLogger {
		return fLogger
	})

	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		DisplayName: "say-hello",
		LogSourceID: uuid.New(),
		Script:      "echo hello",
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)
	require.NoError(t, runner.Execute(ctx, agentscripts.ExecuteAllScripts))
	runner.Close()

	log := testutil.RequireRecvCtx(ctx, t, fLogger.logs)
	require.Equal(t, "hello", log.Output)

	timings := aAPI.GetTimings()
	require.Equal(t, 1, len(timings))

	timing := timings[0]
	require.Equal(t, int32(0), timing.ExitCode)
	require.GreaterOrEqual(t, timing.End.AsTime(), timing.Start.AsTime())
}

// TestCronClose exists because cron.Run() can happen after cron.Close().
// If this happens, there used to be a deadlock.
func TestCronClose(t *testing.T) {
	t.Parallel()
	runner := agentscripts.New(agentscripts.Options{})
	runner.StartCron()
	require.NoError(t, runner.Close(), "close runner")
}

func TestExecuteOptions(t *testing.T) {
	t.Parallel()

	startScript := codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.New(),
		Script:      "echo start",
		RunOnStart:  true,
	}
	stopScript := codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.New(),
		Script:      "echo stop",
		RunOnStop:   true,
	}
	postStartScript := codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.New(),
		Script:      "echo poststart",
	}
	regularScript := codersdk.WorkspaceAgentScript{
		ID:          uuid.New(),
		LogSourceID: uuid.New(),
		Script:      "echo regular",
	}

	scripts := []codersdk.WorkspaceAgentScript{
		startScript,
		stopScript,
		regularScript,
	}
	allScripts := append(slices.Clone(scripts), postStartScript)

	scriptByID := func(t *testing.T, id uuid.UUID) codersdk.WorkspaceAgentScript {
		for _, script := range allScripts {
			if script.ID == id {
				return script
			}
		}
		t.Fatal("script not found")
		return codersdk.WorkspaceAgentScript{}
	}

	wantOutput := map[uuid.UUID]string{
		startScript.ID:     "start",
		stopScript.ID:      "stop",
		postStartScript.ID: "poststart",
		regularScript.ID:   "regular",
	}

	testCases := []struct {
		name    string
		option  agentscripts.ExecuteOption
		wantRun []uuid.UUID
	}{
		{
			name:    "ExecuteAllScripts",
			option:  agentscripts.ExecuteAllScripts,
			wantRun: []uuid.UUID{startScript.ID, stopScript.ID, regularScript.ID, postStartScript.ID},
		},
		{
			name:    "ExecuteStartScripts",
			option:  agentscripts.ExecuteStartScripts,
			wantRun: []uuid.UUID{startScript.ID},
		},
		{
			name:    "ExecutePostStartScripts",
			option:  agentscripts.ExecutePostStartScripts,
			wantRun: []uuid.UUID{postStartScript.ID},
		},
		{
			name:    "ExecuteStopScripts",
			option:  agentscripts.ExecuteStopScripts,
			wantRun: []uuid.UUID{stopScript.ID},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			executedScripts := make(map[uuid.UUID]bool)
			fLogger := &executeOptionTestLogger{
				tb:              t,
				executedScripts: executedScripts,
				wantOutput:      wantOutput,
			}

			runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger {
				return fLogger
			})
			defer runner.Close()

			aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
			err := runner.Init(
				scripts,
				aAPI.ScriptCompleted,
				agentscripts.WithPostStartScripts(postStartScript),
			)
			require.NoError(t, err)

			err = runner.Execute(ctx, tc.option)
			require.NoError(t, err)

			gotRun := map[uuid.UUID]bool{}
			for _, id := range tc.wantRun {
				gotRun[id] = true
				require.True(t, executedScripts[id],
					"script %s should have run when using filter %s", scriptByID(t, id).Script, tc.name)
			}

			for _, script := range allScripts {
				if _, ok := gotRun[script.ID]; ok {
					continue
				}
				require.False(t, executedScripts[script.ID],
					"script %s should not have run when using filter %s", script.Script, tc.name)
			}
		})
	}
}

type executeOptionTestLogger struct {
	tb              testing.TB
	executedScripts map[uuid.UUID]bool
	wantOutput      map[uuid.UUID]string
	mu              sync.Mutex
}

func (l *executeOptionTestLogger) Send(_ context.Context, logs ...agentsdk.Log) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, log := range logs {
		l.tb.Log(log.Output)
		for id, output := range l.wantOutput {
			if log.Output == output {
				l.executedScripts[id] = true
				break
			}
		}
	}
	return nil
}

func (*executeOptionTestLogger) Flush(context.Context) error {
	return nil
}

func setup(t *testing.T, getScriptLogger func(logSourceID uuid.UUID) agentscripts.ScriptLogger) *agentscripts.Runner {
	t.Helper()
	if getScriptLogger == nil {
		// noop
		getScriptLogger = func(uuid.UUID) agentscripts.ScriptLogger {
			return noopScriptLogger{}
		}
	}
	fs := afero.NewMemMapFs()
	logger := testutil.Logger(t)
	s, err := agentssh.NewServer(context.Background(), logger, prometheus.NewRegistry(), fs, agentexec.DefaultExecer, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
	})
	return agentscripts.New(agentscripts.Options{
		LogDir:          t.TempDir(),
		DataDirBase:     t.TempDir(),
		Logger:          logger,
		SSHServer:       s,
		Filesystem:      fs,
		GetScriptLogger: getScriptLogger,
	})
}

type noopScriptLogger struct{}

func (noopScriptLogger) Send(context.Context, ...agentsdk.Log) error {
	return nil
}

func (noopScriptLogger) Flush(context.Context) error {
	return nil
}

type fakeScriptLogger struct {
	logs chan agentsdk.Log
}

func (f *fakeScriptLogger) Send(ctx context.Context, logs ...agentsdk.Log) error {
	for _, log := range logs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case f.logs <- log:
			// OK!
		}
	}
	return nil
}

func (*fakeScriptLogger) Flush(context.Context) error {
	return nil
}

func newFakeScriptLogger() *fakeScriptLogger {
	return &fakeScriptLogger{make(chan agentsdk.Log, 100)}
}
