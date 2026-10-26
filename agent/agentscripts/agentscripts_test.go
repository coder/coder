package agentscripts_test

import (
	"context"
	"path/filepath"
	"runtime"
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
	"github.com/coder/coder/v2/agent/proto"
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
	log := testutil.TryReceive(ctx, t, fLogger.logs)
	require.Equal(t, "hello", log.Output)
}

func TestExecuteCreatesMissingLogDir(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	fs := afero.NewOsFs()
	logger := testutil.Logger(t)
	s, err := agentssh.NewServer(context.Background(), logger, prometheus.NewRegistry(), fs, agentexec.DefaultExecer, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = s.Close()
	})

	fLogger := newFakeScriptLogger()
	runner := agentscripts.New(agentscripts.Options{
		LogDir:      t.TempDir(),
		DataDirBase: t.TempDir(),
		Logger:      logger,
		SSHServer:   s,
		Filesystem:  fs,
		GetScriptLogger: func(uuid.UUID) agentscripts.ScriptLogger {
			return fLogger
		},
	})
	defer runner.Close()

	logPath := filepath.Join(t.TempDir(), "does", "not", "exist", "install.log")

	aAPI := agenttest.NewFakeAgentAPI(t, logger, nil, nil)
	err = runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: uuid.New(),
		LogPath:     logPath,
		Script:      "echo hello",
	}}, aAPI.ScriptCompleted)
	require.NoError(t, err)
	require.NoError(t, runner.Execute(ctx, agentscripts.ExecuteAllScripts))

	log := testutil.TryReceive(ctx, t, fLogger.logs)
	require.Equal(t, "hello", log.Output)

	exists, err := afero.Exists(fs, logPath)
	require.NoError(t, err)
	require.True(t, exists, "expected log file to be created at %s", logPath)
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
	if runtime.GOOS == "darwin" {
		t.Skip("this test is flaky on macOS, see https://github.com/coder/internal/issues/329")
	}
	runner := setup(t, nil)
	defer runner.Close()
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
	err := runner.Init([]codersdk.WorkspaceAgentScript{{
		LogSourceID: uuid.New(),
		Script:      "sleep infinity",
		Timeout:     100 * time.Millisecond,
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

	log := testutil.TryReceive(ctx, t, fLogger.logs)
	require.Equal(t, "hello", log.Output)

	timings := aAPI.GetTimings()
	require.Equal(t, 1, len(timings))

	timing := timings[0]
	require.Equal(t, int32(0), timing.ExitCode)
	if assert.True(t, timing.Start.IsValid(), "start time should be valid") {
		require.NotZero(t, timing.Start.AsTime(), "start time should not be zero")
	}
	if assert.True(t, timing.End.IsValid(), "end time should be valid") {
		require.NotZero(t, timing.End.AsTime(), "end time should not be zero")
	}
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

	scriptByID := func(t *testing.T, id uuid.UUID) codersdk.WorkspaceAgentScript {
		for _, script := range scripts {
			if script.ID == id {
				return script
			}
		}
		t.Fatal("script not found")
		return codersdk.WorkspaceAgentScript{}
	}

	wantOutput := map[uuid.UUID]string{
		startScript.ID:   "start",
		stopScript.ID:    "stop",
		regularScript.ID: "regular",
	}

	testCases := []struct {
		name    string
		option  agentscripts.ExecuteOption
		wantRun []uuid.UUID
	}{
		{
			name:    "ExecuteAllScripts",
			option:  agentscripts.ExecuteAllScripts,
			wantRun: []uuid.UUID{startScript.ID, stopScript.ID, regularScript.ID},
		},
		{
			name:    "ExecuteStartScripts",
			option:  agentscripts.ExecuteStartScripts,
			wantRun: []uuid.UUID{startScript.ID},
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

			for _, script := range scripts {
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

func TestExecuteFastPath(t *testing.T) {
	t.Parallel()

	t.Run("AddressesWithoutDependencies", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		logs := newRecordingScriptLogs()
		runner := setup(t, logs.logger)
		defer runner.Close()
		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)

		scripts := []codersdk.WorkspaceAgentScript{
			{ID: uuid.New(), LogSourceID: uuid.New(), ResourceAddress: "coder_script.a", Script: "echo a", RunOnStart: true},
			{ID: uuid.New(), LogSourceID: uuid.New(), ResourceAddress: "coder_script.b", Script: "echo b", RunOnStart: true},
		}
		require.NoError(t, runner.Init(scripts, aAPI.ScriptCompleted))
		require.NoError(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))
		require.Equal(t, []string{"a"}, logs.outputs(scripts[0].LogSourceID))
		require.Equal(t, []string{"b"}, logs.outputs(scripts[1].LogSourceID))
	})

	t.Run("DependencyOnlyAtStop", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		logs := newRecordingScriptLogs()
		runner := setup(t, logs.logger)
		defer runner.Close()
		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)

		// "exit 1" alone runs the same under sh and PowerShell.
		save := codersdk.WorkspaceAgentScript{
			ID: uuid.New(), LogSourceID: uuid.New(), DisplayName: "Save state",
			ResourceAddress: "coder_script.save_state", Script: "exit 1",
			RunOnStart: true, RunOnStop: true,
		}
		stopDB := codersdk.WorkspaceAgentScript{
			ID: uuid.New(), LogSourceID: uuid.New(), DisplayName: "Stop database",
			ResourceAddress: "coder_script.stop_db", Script: "echo stopping", RunOnStop: true,
			Dependencies: []codersdk.WorkspaceAgentScriptDependency{{
				PrerequisiteResourceAddress: "coder_script.save_state",
				Requirement:                 codersdk.WorkspaceAgentScriptDependencyRequirementSuccess,
			}},
		}
		require.NoError(t, runner.Init([]codersdk.WorkspaceAgentScript{save, stopDB}, aAPI.ScriptCompleted))

		require.Error(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))
		require.Empty(t, logs.outputs(stopDB.LogSourceID))

		require.Error(t, runner.Execute(ctx, agentscripts.ExecuteStopScripts))
		require.Equal(t, []string{`Skipping script: dependency "Save state" failed.`}, logs.outputs(stopDB.LogSourceID))
	})
}

func TestExecuteOrderedTimeoutStartsAfterWaiting(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	ctx := testutil.Context(t, testutil.WaitShort)
	logs := newRecordingScriptLogs()
	runner := setup(t, logs.logger)
	defer runner.Close()
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)

	clone := codersdk.WorkspaceAgentScript{
		ID: uuid.New(), LogSourceID: uuid.New(), ResourceAddress: "coder_script.clone",
		Script: "sleep 2", RunOnStart: true,
	}
	install := codersdk.WorkspaceAgentScript{
		ID: uuid.New(), LogSourceID: uuid.New(), ResourceAddress: "coder_script.install",
		Script: "echo installed", RunOnStart: true, Timeout: time.Second,
		Dependencies: []codersdk.WorkspaceAgentScriptDependency{{
			PrerequisiteResourceAddress: "coder_script.clone",
			Requirement:                 codersdk.WorkspaceAgentScriptDependencyRequirementSuccess,
		}},
	}
	require.NoError(t, runner.Init([]codersdk.WorkspaceAgentScript{clone, install}, aAPI.ScriptCompleted))
	require.NoError(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))
	require.Equal(t, []string{"installed"}, logs.outputs(install.LogSourceID))
}

func TestExecuteCanceledScriptReportsCanceled(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}

	for _, test := range []struct {
		name       string
		option     agentscripts.ExecuteOption
		wantStatus proto.Timing_Status
		wantStage  proto.Timing_Stage
	}{
		{name: "Start", option: agentscripts.ExecuteStartScripts, wantStatus: proto.Timing_CANCELED, wantStage: proto.Timing_START},
		{name: "Stop", option: agentscripts.ExecuteStopScripts, wantStatus: proto.Timing_EXIT_FAILURE, wantStage: proto.Timing_STOP},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			execCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			fLogger := newFakeScriptLogger()
			runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger })
			aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)

			// exec keeps the shell from forking sleep. A SIGHUP that lands
			// in that fork window can miss the child, which then holds the
			// output pipes open for cmd.WaitDelay and the test times out.
			script := codersdk.WorkspaceAgentScript{
				ID: uuid.New(), LogSourceID: uuid.New(),
				Script: "echo started && exec sleep 30", RunOnStart: true, RunOnStop: true,
			}
			require.NoError(t, runner.Init([]codersdk.WorkspaceAgentScript{script}, aAPI.ScriptCompleted))

			done := make(chan error, 1)
			go func() { done <- runner.Execute(execCtx, test.option) }()
			require.Equal(t, "started", testutil.RequireReceive(ctx, t, fLogger.logs).Output)
			cancel()
			require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), context.Canceled)
			require.NoError(t, runner.Close())

			timings := aAPI.GetTimings()
			require.Len(t, timings, 1)
			require.Equal(t, test.wantStatus, timings[0].Status)
			require.Equal(t, test.wantStage, timings[0].Stage)
			require.True(t, timings[0].End.AsTime().After(timings[0].Start.AsTime()), "the real interval is kept")
		})
	}

	t.Run("ExitedBeforeCancel", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		execCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		runner := setup(t, nil)
		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)

		script := codersdk.WorkspaceAgentScript{ID: uuid.New(), LogSourceID: uuid.New(), Script: "exit 1", RunOnStart: true}
		require.NoError(t, runner.Init([]codersdk.WorkspaceAgentScript{script}, aAPI.ScriptCompleted))

		require.Error(t, runner.Execute(execCtx, agentscripts.ExecuteStartScripts))
		cancel()
		require.NoError(t, runner.Close())

		timings := aAPI.GetTimings()
		require.Len(t, timings, 1)
		require.Equal(t, proto.Timing_EXIT_FAILURE, timings[0].Status)
		require.Equal(t, int32(1), timings[0].ExitCode)
	})
}

// TestCloseWaitsForCanceledStartReports follows the agent's shutdown order:
// cancel the start execution and call Close at once, without waiting for
// Execute to return.
func TestCloseWaitsForCanceledStartReports(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}

	t.Run("Unordered", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		execCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		fLogger := newFakeScriptLogger()
		runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger })
		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)

		script := codersdk.WorkspaceAgentScript{
			ID: uuid.New(), LogSourceID: uuid.New(),
			Script: "echo started && exec sleep 30", RunOnStart: true,
		}
		require.NoError(t, runner.Init([]codersdk.WorkspaceAgentScript{script}, aAPI.ScriptCompleted))

		done := make(chan error, 1)
		go func() { done <- runner.Execute(execCtx, agentscripts.ExecuteStartScripts) }()
		require.Equal(t, "started", testutil.RequireReceive(ctx, t, fLogger.logs).Output)
		cancel()
		require.NoError(t, runner.Close())
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), context.Canceled)

		timings := aAPI.GetTimings()
		require.Len(t, timings, 1)
		require.Equal(t, proto.Timing_CANCELED, timings[0].Status)
		require.Equal(t, proto.Timing_START, timings[0].Stage)

		require.ErrorContains(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts), "execute: closed")
	})

	t.Run("Ordered", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		execCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		fLogger := newFakeScriptLogger()
		running := codersdk.WorkspaceAgentScript{
			ID: uuid.New(), LogSourceID: uuid.New(), ResourceAddress: "coder_script.running",
			Script: "echo started && exec sleep 30", RunOnStart: true,
		}
		waiting := codersdk.WorkspaceAgentScript{
			ID: uuid.New(), LogSourceID: uuid.New(), ResourceAddress: "coder_script.waiting",
			Script: "echo never", RunOnStart: true,
			Dependencies: []codersdk.WorkspaceAgentScriptDependency{{
				PrerequisiteResourceAddress: running.ResourceAddress,
				Requirement:                 codersdk.WorkspaceAgentScriptDependencyRequirementCompletion,
			}},
		}
		// Only the running script's output goes to the channel the test
		// reads, so the waiting line does not block Execute.
		runner := setup(t, func(logSourceID uuid.UUID) agentscripts.ScriptLogger {
			if logSourceID == running.LogSourceID {
				return fLogger
			}
			return noopScriptLogger{}
		})
		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
		require.NoError(t, runner.Init([]codersdk.WorkspaceAgentScript{running, waiting}, aAPI.ScriptCompleted))

		done := make(chan error, 1)
		go func() { done <- runner.Execute(execCtx, agentscripts.ExecuteStartScripts) }()
		require.Equal(t, "started", testutil.RequireReceive(ctx, t, fLogger.logs).Output)
		cancel()
		require.NoError(t, runner.Close())
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), context.Canceled)

		timings := aAPI.GetTimings()
		require.Len(t, timings, 2)
		byScript := make(map[uuid.UUID]*proto.Timing, len(timings))
		for _, timing := range timings {
			byScript[uuid.UUID(timing.ScriptId)] = timing
		}
		require.Equal(t, proto.Timing_CANCELED, byScript[running.ID].Status)
		require.True(t, byScript[running.ID].End.AsTime().After(byScript[running.ID].Start.AsTime()), "the running script keeps its real interval")
		require.Equal(t, proto.Timing_CANCELED, byScript[waiting.ID].Status)
		require.Equal(t, byScript[waiting.ID].Start.AsTime(), byScript[waiting.ID].End.AsTime(), "the waiting script never ran")
	})
}

// The agent calls Execute and then StartCron from one goroutine while Close
// runs on another, so StartCron can land after Close has started waiting
// for Execute. Close must still return.
func TestStartCronDuringCloseDoesNotBlockClose(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	ctx := testutil.Context(t, testutil.WaitShort)
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	fLogger := newFakeScriptLogger()
	runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger })
	aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)

	script := codersdk.WorkspaceAgentScript{
		ID: uuid.New(), LogSourceID: uuid.New(),
		Script: "echo started && exec sleep 30", RunOnStart: true,
	}
	require.NoError(t, runner.Init([]codersdk.WorkspaceAgentScript{script}, aAPI.ScriptCompleted))

	done := make(chan error, 1)
	go func() {
		err := runner.Execute(execCtx, agentscripts.ExecuteStartScripts)
		runner.StartCron()
		done <- err
	}()
	require.Equal(t, "started", testutil.RequireReceive(ctx, t, fLogger.logs).Output)

	closed := make(chan error, 1)
	go func() { closed <- runner.Close() }()
	// There are no stop scripts, so this probe runs nothing. It fails only
	// once Close has begun and is waiting for the start script.
	require.Eventually(t, func() bool {
		return runner.Execute(ctx, agentscripts.ExecuteStopScripts) != nil
	}, testutil.WaitShort, testutil.IntervalFast)

	cancel()
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), context.Canceled)
	require.NoError(t, testutil.RequireReceive(ctx, t, closed))
}

// recordingScriptLogs keeps every line per log source so a test can read a
// script's output and its dependency lines together.
type recordingScriptLogs struct {
	mu   sync.Mutex
	byID map[uuid.UUID][]string
}

func newRecordingScriptLogs() *recordingScriptLogs {
	return &recordingScriptLogs{byID: make(map[uuid.UUID][]string)}
}

func (l *recordingScriptLogs) logger(logSourceID uuid.UUID) agentscripts.ScriptLogger {
	return recordingScriptLogger{logs: l, logSourceID: logSourceID}
}

func (l *recordingScriptLogs) outputs(logSourceID uuid.UUID) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.byID[logSourceID]...)
}

type recordingScriptLogger struct {
	logs        *recordingScriptLogs
	logSourceID uuid.UUID
}

func (l recordingScriptLogger) Send(_ context.Context, logs ...agentsdk.Log) error {
	l.logs.mu.Lock()
	defer l.logs.mu.Unlock()
	for _, log := range logs {
		l.logs.byID[l.logSourceID] = append(l.logs.byID[l.logSourceID], log.Output)
	}
	return nil
}

func (recordingScriptLogger) Flush(context.Context) error {
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
