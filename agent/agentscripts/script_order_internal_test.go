package agentscripts

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// goleak runs from TestMain in agentscripts_test.go, which shares this
// test binary.

const (
	requireSuccess    = codersdk.WorkspaceAgentScriptDependencyRequirementSuccess
	requireCompletion = codersdk.WorkspaceAgentScriptDependencyRequirementCompletion
)

func TestLifecycleExecutorOrdersScripts(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	runs := newScriptOrderRuns()
	runs.hold("coder_script.clone")
	executor := newTestExecutor(t, runs, newScriptOrderTestLogs())

	clone := scriptOrderTestScript("coder_script.clone")
	install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))

	done := runExecute(ctx, executor, clone, install)

	runs.waitStarted(ctx, t, "coder_script.clone")
	runs.requireNotStarted(t, "coder_script.install")

	runs.release("coder_script.clone", nil)
	runs.waitStarted(ctx, t, "coder_script.install")
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

func TestLifecycleExecutorWaitsForEveryPrerequisite(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	runs := newScriptOrderRuns()
	runs.hold("coder_script.a")
	runs.hold("coder_script.b")
	executor := newTestExecutor(t, runs, newScriptOrderTestLogs())

	a := scriptOrderTestScript("coder_script.a")
	b := scriptOrderTestScript("coder_script.b")
	c := scriptOrderTestScript("coder_script.c",
		scriptOrderTestDependency("coder_script.a", requireSuccess),
		scriptOrderTestDependency("coder_script.b", requireCompletion),
	)

	done := runExecute(ctx, executor, a, b, c)

	runs.waitStarted(ctx, t, "coder_script.a")
	runs.waitStarted(ctx, t, "coder_script.b")
	runs.release("coder_script.a", nil)
	runs.requireNotStarted(t, "coder_script.c")

	runs.release("coder_script.b", nil)
	runs.waitStarted(ctx, t, "coder_script.c")
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

// The executor hands run a context with no deadline of its own, so the
// timeout context created inside run cannot have started during the wait.
// TestExecuteOrderedTimeoutStartsAfterWaiting covers the same with a real
// process.
func TestLifecycleExecutorTimeoutStartsAfterWaiting(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	executeCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runs := newScriptOrderRuns()
	runs.hold("coder_script.clone")
	executor := newTestExecutor(t, runs, newScriptOrderTestLogs())
	deadlines := make(chan bool, 2)
	executor.run = func(ctx context.Context, script codersdk.WorkspaceAgentScript, option ExecuteOption) error {
		_, hasDeadline := ctx.Deadline()
		deadlines <- hasDeadline
		return runs.run(ctx, script, option)
	}

	clone := scriptOrderTestScript("coder_script.clone")
	install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))
	install.Timeout = 10 * time.Millisecond

	done := runExecute(executeCtx, executor, clone, install)
	runs.waitStarted(ctx, t, "coder_script.clone")
	require.False(t, testutil.RequireReceive(ctx, t, deadlines))

	runs.release("coder_script.clone", nil)
	runs.waitStarted(ctx, t, "coder_script.install")
	require.False(t, testutil.RequireReceive(ctx, t, deadlines))
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

func TestScriptOutcome(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		runErr error
		ctxErr error
		want   unit.Outcome
	}{
		{name: "Succeeded", want: unit.OutcomeSucceeded},
		{name: "TimedOut", runErr: ErrTimeout, want: unit.OutcomeTimedOut},
		{name: "TimedOutDuringCancel", runErr: ErrTimeout, ctxErr: context.Canceled, want: unit.OutcomeTimedOut},
		{name: "PipesLeftOpen", runErr: ErrOutputPipesOpen, want: unit.OutcomeFailed},
		{name: "ExitCode", runErr: xerrors.New("exit status 1"), want: unit.OutcomeFailed},
		{name: "Canceled", runErr: context.Canceled, ctxErr: context.Canceled, want: unit.OutcomeCanceled},
		{name: "OtherErrorDuringCancel", runErr: xerrors.New("signal: killed"), ctxErr: context.Canceled, want: unit.OutcomeCanceled},
		{name: "WrappedTimeout", runErr: xerrors.Errorf("run agent script: %w", ErrTimeout), want: unit.OutcomeTimedOut},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, scriptOutcome(test.runErr, test.ctxErr))
		})
	}
}

func TestLifecycleExecutorRequirements(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		requirement codersdk.WorkspaceAgentScriptDependencyRequirement
		cloneErr    error
		wantRun     bool
		wantLogs    []string
	}{
		{name: "SuccessAfterFailure", requirement: requireSuccess, cloneErr: xerrors.New("exit status 128"), wantLogs: []string{`Skipping script: dependency "Clone" failed.`}},
		{name: "SuccessAfterTimeout", requirement: requireSuccess, cloneErr: ErrTimeout, wantLogs: []string{`Skipping script: dependency "Clone" timed out.`}},
		{name: "SuccessAfterSuccess", requirement: requireSuccess, wantRun: true},
		{name: "CompletionAfterFailure", requirement: requireCompletion, cloneErr: xerrors.New("exit status 128"), wantRun: true},
		{name: "CompletionAfterTimeout", requirement: requireCompletion, cloneErr: ErrTimeout, wantRun: true},
		{name: "CompletionAfterSuccess", requirement: requireCompletion, wantRun: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			runs := newScriptOrderRuns()
			runs.result("coder_script.clone", test.cloneErr)
			logs := newScriptOrderTestLogs()
			executor := newTestExecutor(t, runs, logs)

			clone := scriptOrderTestScript("coder_script.clone")
			clone.DisplayName = "Clone"
			install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", test.requirement))

			err := executor.execute(ctx, []codersdk.WorkspaceAgentScript{clone, install}, ExecuteStartScripts)
			if test.cloneErr != nil {
				require.ErrorIs(t, err, test.cloneErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.wantRun, runs.started("coder_script.install"))
			require.Equal(t, test.wantLogs, logs.outputs(install.LogSourceID))
		})
	}
}

func TestLifecycleExecutorPropagatesSkippedOutcomes(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	runs := newScriptOrderRuns()
	runs.result("coder_script.a", xerrors.New("exit status 1"))
	logs := newScriptOrderTestLogs()
	executor := newTestExecutor(t, runs, logs)

	a := scriptOrderTestScript("coder_script.a")
	b := scriptOrderTestScript("coder_script.b", scriptOrderTestDependency("coder_script.a", requireSuccess))
	c := scriptOrderTestScript("coder_script.c", scriptOrderTestDependency("coder_script.b", requireCompletion))
	d := scriptOrderTestScript("coder_script.d", scriptOrderTestDependency("coder_script.b", requireSuccess))

	err := executor.execute(ctx, []codersdk.WorkspaceAgentScript{a, b, c, d}, ExecuteStartScripts)
	require.Error(t, err)

	require.ElementsMatch(t, []string{"coder_script.a", "coder_script.c"}, runs.allStarted())
	require.Equal(t, []string{`Skipping script: dependency "coder_script.a" failed.`}, logs.outputs(b.LogSourceID))
	require.Empty(t, logs.outputs(c.LogSourceID))
	require.Equal(t, []string{`Skipping script: dependency "coder_script.b" was skipped.`}, logs.outputs(d.LogSourceID))
}

func TestLifecycleExecutorReportsSkippedWithoutRunning(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	runs := newScriptOrderRuns()
	runs.result("coder_script.clone", xerrors.New("exit status 128"))
	logs := newScriptOrderTestLogs()
	reports := newScriptOrderReports()
	executor := newTestExecutor(t, runs, logs)
	executor.reportUnstarted = func(ctx context.Context, script codersdk.WorkspaceAgentScript, option ExecuteOption, status proto.Timing_Status) {
		assert.NotEmpty(t, logs.outputs(script.LogSourceID), "the skip line is written before the timing is sent")
		reports.record(ctx, script, option, status)
	}

	clone := scriptOrderTestScript("coder_script.clone")
	clone.DisplayName = "Clone"
	install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))

	err := executor.execute(ctx, []codersdk.WorkspaceAgentScript{clone, install}, ExecuteStartScripts)
	require.Error(t, err)
	require.Equal(t, []string{"coder_script.clone"}, runs.allStarted())
	require.Equal(t, []scriptOrderReport{
		{address: "coder_script.install", option: ExecuteStartScripts, status: proto.Timing_SKIPPED},
	}, reports.all())
	require.Equal(t, []string{`Skipping script: dependency "Clone" failed.`}, logs.outputs(install.LogSourceID))
}

func TestLifecycleExecutorNilReporter(t *testing.T) {
	t.Parallel()

	t.Run("Skipped", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		runs := newScriptOrderRuns()
		runs.result("coder_script.clone", xerrors.New("exit status 128"))
		executor := newTestExecutor(t, runs, newScriptOrderTestLogs())

		clone := scriptOrderTestScript("coder_script.clone")
		install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))

		require.Error(t, executor.execute(ctx, []codersdk.WorkspaceAgentScript{clone, install}, ExecuteStartScripts))
		require.False(t, runs.started("coder_script.install"))
	})

	t.Run("Canceled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		defer cancel()

		runs := newScriptOrderRuns()
		runs.hold("coder_script.clone")
		executor := newTestExecutor(t, runs, newScriptOrderTestLogs())

		clone := scriptOrderTestScript("coder_script.clone")
		install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))

		done := runExecute(ctx, executor, clone, install)
		runs.waitStarted(ctx, t, "coder_script.clone")
		cancel()
		waitCtx := testutil.Context(t, testutil.WaitShort)
		require.ErrorIs(t, testutil.RequireReceive(waitCtx, t, done), context.Canceled)
		require.False(t, runs.started("coder_script.install"))
	})
}

func TestLifecycleExecutorMixedRequirements(t *testing.T) {
	t.Parallel()

	scripts := func() (clone, warm, build codersdk.WorkspaceAgentScript) {
		clone = scriptOrderTestScript("coder_script.clone_repo")
		clone.DisplayName = "Clone repo"
		warm = scriptOrderTestScript("coder_script.warm_cache")
		warm.DisplayName = "Warm cache"
		build = scriptOrderTestScript("coder_script.build",
			scriptOrderTestDependency("coder_script.clone_repo", requireSuccess),
			scriptOrderTestDependency("coder_script.warm_cache", requireCompletion),
		)
		return clone, warm, build
	}

	t.Run("CompletionPrerequisiteFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		runs := newScriptOrderRuns()
		runs.result("coder_script.warm_cache", xerrors.New("exit status 1"))
		logs := newScriptOrderTestLogs()
		executor := newTestExecutor(t, runs, logs)
		clone, warm, build := scripts()

		err := executor.execute(ctx, []codersdk.WorkspaceAgentScript{clone, warm, build}, ExecuteStartScripts)
		require.Error(t, err)
		require.True(t, runs.started("coder_script.build"))
		require.Empty(t, logs.outputs(build.LogSourceID))
	})

	t.Run("SuccessPrerequisiteFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		runs := newScriptOrderRuns()
		runs.result("coder_script.clone_repo", xerrors.New("exit status 128"))
		logs := newScriptOrderTestLogs()
		executor := newTestExecutor(t, runs, logs)
		clone, warm, build := scripts()

		err := executor.execute(ctx, []codersdk.WorkspaceAgentScript{clone, warm, build}, ExecuteStartScripts)
		require.Error(t, err)
		require.False(t, runs.started("coder_script.build"))
		require.Equal(t, []string{`Skipping script: dependency "Clone repo" failed.`}, logs.outputs(build.LogSourceID))
	})
}

func TestLifecycleExecutorSkipsOnFirstImpossibleEdge(t *testing.T) {
	t.Parallel()

	scripts := func() (clone, secrets, build codersdk.WorkspaceAgentScript) {
		clone = scriptOrderTestScript("coder_script.clone_repo")
		clone.DisplayName = "Clone repo"
		secrets = scriptOrderTestScript("coder_script.fetch_secrets")
		secrets.DisplayName = "Fetch secrets"
		build = scriptOrderTestScript("coder_script.build",
			scriptOrderTestDependency("coder_script.clone_repo", requireSuccess),
			scriptOrderTestDependency("coder_script.fetch_secrets", requireSuccess),
		)
		return clone, secrets, build
	}

	t.Run("OnePrerequisiteStillRunning", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		runs := newScriptOrderRuns()
		runs.hold("coder_script.clone_repo")
		runs.result("coder_script.fetch_secrets", xerrors.New("exit status 1"))
		logs := newScriptOrderTestLogs()
		executor := newTestExecutor(t, runs, logs)
		clone, secrets, build := scripts()

		done := runExecute(ctx, executor, clone, secrets, build)
		runs.waitStarted(ctx, t, "coder_script.clone_repo")

		// The skip line arrives while clone_repo is still held.
		require.Equal(t, `Skipping script: dependency "Fetch secrets" failed.`, logs.waitOutput(ctx, t, build.LogSourceID))

		runs.release("coder_script.clone_repo", nil)
		require.Error(t, testutil.RequireReceive(ctx, t, done))
		require.False(t, runs.started("coder_script.build"))
	})

	t.Run("BothPrerequisitesFinishedBadly", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		runs := newScriptOrderRuns()
		runs.result("coder_script.clone_repo", xerrors.New("exit status 128"))
		runs.result("coder_script.fetch_secrets", ErrTimeout)
		logs := newScriptOrderTestLogs()
		executor := newTestExecutor(t, runs, logs)
		clone, secrets, build := scripts()

		// Both prerequisites finish before build is evaluated, so the
		// manager lists both as impossible.
		manager, byAddress, err := lifecycleManager([]codersdk.WorkspaceAgentScript{clone, secrets, build})
		require.NoError(t, err)
		require.Error(t, executor.executeWith(ctx, manager, byAddress, []codersdk.WorkspaceAgentScript{clone, secrets}, ExecuteStartScripts))
		require.NoError(t, executor.executeWith(ctx, manager, byAddress, []codersdk.WorkspaceAgentScript{build}, ExecuteStartScripts))

		require.False(t, runs.started("coder_script.build"))
		require.Equal(t, []string{
			`Skipping script: dependency "Clone repo" failed, dependency "Fetch secrets" timed out.`,
		}, logs.outputs(build.LogSourceID))
	})
}

func TestLifecycleExecutorWaitingLogsAreCombinedAndSorted(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	clock := quartz.NewMock(t)
	tickerTrap := clock.Trap().NewTicker("agent_scripts", "dependency_wait")
	defer tickerTrap.Close()

	runs := newScriptOrderRuns()
	runs.hold("coder_script.z_clone")
	runs.hold("coder_script.a_auth")
	logs := newScriptOrderTestLogs()
	executor := newTestExecutor(t, runs, logs)
	executor.clock = clock

	zClone := scriptOrderTestScript("coder_script.z_clone")
	aAuth := scriptOrderTestScript("coder_script.a_auth")
	install := scriptOrderTestScript("coder_script.install",
		scriptOrderTestDependency("coder_script.z_clone", requireSuccess),
		scriptOrderTestDependency("coder_script.a_auth", requireCompletion),
	)

	scripts := []codersdk.WorkspaceAgentScript{zClone, aAuth, install}
	manager, byAddress, err := lifecycleManager(scripts)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- executor.executeWith(ctx, manager, byAddress, scripts, ExecuteStartScripts) }()
	tickerTrap.MustWait(ctx).MustRelease(ctx)

	clock.Advance(dependencyLogInterval).MustWait(ctx)
	require.Equal(t, `Waiting for dependencies: "coder_script.a_auth", "coder_script.z_clone" (30s)`, logs.waitOutput(ctx, t, install.LogSourceID))

	clock.Advance(dependencyLogInterval).MustWait(ctx)
	require.Equal(t, `Waiting for dependencies: "coder_script.a_auth", "coder_script.z_clone" (60s)`, logs.waitOutput(ctx, t, install.LogSourceID))

	runs.release("coder_script.a_auth", nil)
	// The outcome is recorded after run returns, so wait for the record
	// itself rather than for run.
	require.Eventually(t, func() bool {
		u, err := manager.Unit("coder_script.a_auth")
		return err == nil && u.Outcome() == unit.OutcomeSucceeded
	}, testutil.WaitShort, testutil.IntervalFast)
	clock.Advance(dependencyLogInterval).MustWait(ctx)
	require.Equal(t, `Waiting for dependencies: "coder_script.z_clone" (90s)`, logs.waitOutput(ctx, t, install.LogSourceID))

	runs.release("coder_script.z_clone", nil)
	runs.waitStarted(ctx, t, "coder_script.install")
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
	require.Len(t, logs.outputs(install.LogSourceID), 3)
}

func TestLifecycleExecutorNoWaitingLineWhenDecidedEarly(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	clock := quartz.NewMock(t)
	tickerTrap := clock.Trap().NewTicker("agent_scripts", "dependency_wait")
	defer tickerTrap.Close()

	runs := newScriptOrderRuns()
	runs.hold("coder_script.clone")
	logs := newScriptOrderTestLogs()
	executor := newTestExecutor(t, runs, logs)
	executor.clock = clock

	clone := scriptOrderTestScript("coder_script.clone")
	install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))

	done := runExecute(ctx, executor, clone, install)
	// Only install waits, so exactly one ticker is created.
	tickerTrap.MustWait(ctx).MustRelease(ctx)

	runs.release("coder_script.clone", nil)
	runs.waitStarted(ctx, t, "coder_script.install")
	require.NoError(t, testutil.RequireReceive(ctx, t, done))

	require.Empty(t, logs.outputs(clone.LogSourceID))
	require.Empty(t, logs.outputs(install.LogSourceID))
}

func TestLifecycleExecutorCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()

	runs := newScriptOrderRuns()
	runs.hold("coder_script.a")
	logs := newScriptOrderTestLogs()
	reports := newScriptOrderReports()
	executor := newTestExecutor(t, runs, logs)
	executor.reportUnstarted = reports.record

	a := scriptOrderTestScript("coder_script.a")
	b := scriptOrderTestScript("coder_script.b", scriptOrderTestDependency("coder_script.a", requireSuccess))
	c := scriptOrderTestScript("coder_script.c", scriptOrderTestDependency("coder_script.a", requireCompletion))
	d := scriptOrderTestScript("coder_script.d")
	scripts := []codersdk.WorkspaceAgentScript{a, b, c, d}

	manager, byAddress, err := lifecycleManager(scripts)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- executor.executeWith(ctx, manager, byAddress, scripts, ExecuteStartScripts) }()

	runs.waitStarted(ctx, t, "coder_script.a")
	runs.waitFinished(ctx, t, "coder_script.d")

	cancel()
	waitCtx := testutil.Context(t, testutil.WaitShort)
	require.ErrorIs(t, testutil.RequireReceive(waitCtx, t, done), context.Canceled)

	require.ElementsMatch(t, []string{"coder_script.a", "coder_script.d"}, runs.allStarted())
	// a's own timing comes from run, which the fake does not model.
	require.ElementsMatch(t, []scriptOrderReport{
		{address: "coder_script.b", option: ExecuteStartScripts, status: proto.Timing_CANCELED},
		{address: "coder_script.c", option: ExecuteStartScripts, status: proto.Timing_CANCELED},
	}, reports.all(), "only the scripts that never started report from the executor")
	require.Equal(t, []string{canceledRunningLine}, logs.outputs(a.LogSourceID))
	require.Equal(t, []string{canceledBeforeStartLine}, logs.outputs(b.LogSourceID))
	require.Equal(t, []string{canceledBeforeStartLine}, logs.outputs(c.LogSourceID))
	require.Empty(t, logs.outputs(d.LogSourceID), "a finished script gets no line")

	for address, want := range map[string]unit.Outcome{
		"coder_script.a": unit.OutcomeCanceled,
		"coder_script.b": unit.OutcomeCanceled,
		"coder_script.c": unit.OutcomeCanceled,
		"coder_script.d": unit.OutcomeSucceeded,
	} {
		u, err := manager.Unit(unit.ID(address))
		require.NoError(t, err)
		require.Equal(t, want, u.Outcome(), address)
	}
}

func TestLifecycleExecutorRootScriptCanceledBeforeStarting(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	cancel()

	runs := newScriptOrderRuns()
	logs := newScriptOrderTestLogs()
	reports := newScriptOrderReports()
	executor := newTestExecutor(t, runs, logs)
	executor.reportUnstarted = reports.record

	root := scriptOrderTestScript("coder_script.root")
	scripts := []codersdk.WorkspaceAgentScript{root}

	manager, byAddress, err := lifecycleManager(scripts)
	require.NoError(t, err)
	require.ErrorIs(t, executor.executeWith(ctx, manager, byAddress, scripts, ExecuteStartScripts), context.Canceled)

	require.Empty(t, runs.allStarted())
	require.Equal(t, []scriptOrderReport{
		{address: "coder_script.root", option: ExecuteStartScripts, status: proto.Timing_CANCELED},
	}, reports.all())
	require.Equal(t, []string{canceledBeforeStartLine}, logs.outputs(root.LogSourceID))

	u, err := manager.Unit(unit.ID("coder_script.root"))
	require.NoError(t, err)
	require.Equal(t, unit.OutcomeCanceled, u.Outcome())
}

func TestLifecycleExecutorStopScripts(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	saveErr := xerrors.New("exit status 1")
	runs := newScriptOrderRuns()
	runs.result("coder_script.save_state", saveErr)
	logs := newScriptOrderTestLogs()
	reports := newScriptOrderReports()
	executor := newTestExecutor(t, runs, logs)
	executor.reportUnstarted = reports.record

	save := scriptOrderTestScript("coder_script.save_state")
	save.DisplayName = "Save state"
	stopDB := scriptOrderTestScript("coder_script.stop_db", scriptOrderTestDependency("coder_script.save_state", requireSuccess))
	report := scriptOrderTestScript("coder_script.report_stop", scriptOrderTestDependency("coder_script.save_state", requireCompletion))

	err := executor.execute(ctx, []codersdk.WorkspaceAgentScript{save, stopDB, report}, ExecuteStopScripts)
	require.ErrorIs(t, err, saveErr)
	require.ElementsMatch(t, []string{"coder_script.save_state", "coder_script.report_stop"}, runs.allStarted())
	require.Equal(t, []string{`Skipping script: dependency "Save state" failed.`}, logs.outputs(stopDB.LogSourceID))
	require.Equal(t, ExecuteStopScripts, runs.option("coder_script.report_stop"))
	// The executor passes the option through; Runner.reportUnstarted is
	// what drops stop reports (TestRunnerReportsUnstartedTiming).
	require.Equal(t, []scriptOrderReport{
		{address: "coder_script.stop_db", option: ExecuteStopScripts, status: proto.Timing_SKIPPED},
	}, reports.all())
}

func TestLifecycleExecutorFreshManagerPerExecution(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	clone := scriptOrderTestScript("coder_script.clone")
	install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))
	scripts := []codersdk.WorkspaceAgentScript{clone, install}

	first := newScriptOrderRuns()
	first.result("coder_script.clone", xerrors.New("exit status 128"))
	executor := newTestExecutor(t, first, newScriptOrderTestLogs())
	require.Error(t, executor.execute(ctx, scripts, ExecuteStartScripts))
	require.False(t, first.started("coder_script.install"))

	second := newScriptOrderRuns()
	executor.run = second.run
	require.NoError(t, executor.execute(ctx, scripts, ExecuteStartScripts))
	require.True(t, second.started("coder_script.install"))
}

func TestLifecycleExecutorRecordsFailureOnUnrecordedExit(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	runs := newScriptOrderRuns()
	logs := newScriptOrderTestLogs()
	executor := newTestExecutor(t, runs, logs)

	clone := scriptOrderTestScript("coder_script.clone")
	install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))
	scripts := []codersdk.WorkspaceAgentScript{clone, install}
	manager, byAddress, err := lifecycleManager(scripts)
	require.NoError(t, err)
	// Pre-setting the outcome makes the executor's own update fail.
	require.NoError(t, manager.UpdateOutcome("coder_script.clone", unit.OutcomeRunning))

	err = executor.executeWith(ctx, manager, byAddress, scripts, ExecuteStartScripts)
	require.ErrorIs(t, err, unit.ErrSameOutcomeAlreadySet)
	require.NoError(t, ctx.Err())

	require.Empty(t, runs.allStarted())
	u, err := manager.Unit("coder_script.clone")
	require.NoError(t, err)
	require.Equal(t, unit.OutcomeFailed, u.Outcome())
	require.Equal(t, []string{`Skipping script: dependency "coder_script.clone" failed.`}, logs.outputs(install.LogSourceID))
}

func TestLifecycleExecutorAddresslessScriptsRunAtOnce(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	runs := newScriptOrderRuns()
	runs.hold("coder_script.clone")
	executor := newTestExecutor(t, runs, newScriptOrderTestLogs())

	clone := scriptOrderTestScript("coder_script.clone")
	install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))
	legacy := scriptOrderTestScript("")
	devcontainer := scriptOrderTestScript("")

	done := runExecute(ctx, executor, clone, install, legacy, devcontainer)

	runs.waitStarted(ctx, t, "coder_script.clone")
	runs.waitFinished(ctx, t, legacy.LogSourceID.String())
	runs.waitFinished(ctx, t, devcontainer.LogSourceID.String())
	runs.requireNotStarted(t, "coder_script.install")

	runs.release("coder_script.clone", nil)
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

func TestLifecycleExecutorSkipIsNotAnError(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		cloneErr error
	}{
		{name: "Failure", cloneErr: xerrors.New("exit status 128")},
		{name: "Timeout", cloneErr: ErrTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			runs := newScriptOrderRuns()
			runs.result("coder_script.clone", test.cloneErr)
			executor := newTestExecutor(t, runs, newScriptOrderTestLogs())

			clone := scriptOrderTestScript("coder_script.clone")
			install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", requireSuccess))
			post := scriptOrderTestScript("coder_script.post", scriptOrderTestDependency("coder_script.clone", requireCompletion))

			err := executor.execute(ctx, []codersdk.WorkspaceAgentScript{clone, install, post}, ExecuteStartScripts)
			require.ErrorIs(t, err, test.cloneErr)
			// errgroup keeps the first error only, so a skip that returned
			// an error could only show up as the whole result.
			require.Equal(t, test.cloneErr.Error(), err.Error())
			require.ElementsMatch(t, []string{"coder_script.clone", "coder_script.post"}, runs.allStarted())
		})
	}
}

func TestLifecycleExecutorRejectsInvalidDependenciesBeforeRunning(t *testing.T) {
	t.Parallel()

	noAddress := scriptOrderTestScript("", scriptOrderTestDependency("coder_script.clone", requireSuccess))
	noAddress.DisplayName = "Install tools"

	for _, test := range []struct {
		name    string
		scripts []codersdk.WorkspaceAgentScript
		wantErr string
	}{
		{
			name: "DependenciesWithoutAddress",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.clone"),
				noAddress,
			},
			wantErr: `script "Install tools" has dependencies but no Terraform resource address`,
		},
		{
			name: "DuplicateAddress",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.setup"),
				scriptOrderTestScript("coder_script.setup", scriptOrderTestDependency("coder_script.setup", requireSuccess)),
			},
			wantErr: `duplicate script resource address "coder_script.setup"`,
		},
		{
			name: "MissingPrerequisite",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.install_tools", scriptOrderTestDependency("coder_script.clone", requireSuccess)),
			},
			wantErr: `script "coder_script.install_tools" depends on "coder_script.clone", which is not part of this lifecycle execution`,
		},
		{
			name: "UnsupportedRequirement",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.clone_repo"),
				scriptOrderTestScript("coder_script.install_tools", scriptOrderTestDependency("coder_script.clone_repo", "started")),
			},
			wantErr: `script "coder_script.install_tools" dependency on "coder_script.clone_repo": unsupported requirement "started"`,
		},
		{
			name: "Cycle",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.a", scriptOrderTestDependency("coder_script.b", requireSuccess)),
				scriptOrderTestScript("coder_script.b", scriptOrderTestDependency("coder_script.a", requireSuccess)),
			},
			wantErr: `add script dependency "coder_script.b" after "coder_script.a"`,
		},
		{
			name: "ConflictingRequirement",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.clone_repo"),
				scriptOrderTestScript("coder_script.build",
					scriptOrderTestDependency("coder_script.clone_repo", requireSuccess),
					scriptOrderTestDependency("coder_script.clone_repo", requireCompletion),
				),
			},
			wantErr: `add script dependency "coder_script.build" after "coder_script.clone_repo"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			runs := newScriptOrderRuns()
			logs := newScriptOrderTestLogs()
			executor := newTestExecutor(t, runs, logs)

			err := executor.execute(ctx, test.scripts, ExecuteStartScripts)
			require.ErrorContains(t, err, test.wantErr)
			switch test.name {
			case "Cycle":
				require.ErrorIs(t, err, unit.ErrCycleDetected)
			case "ConflictingRequirement":
				require.ErrorIs(t, err, unit.ErrConflictingRequirement)
			}
			require.Empty(t, runs.allStarted())
			for _, script := range test.scripts {
				require.Equal(t, []string{"Lifecycle scripts did not run: " + err.Error()}, logs.outputs(script.LogSourceID))
			}
		})
	}
}

func TestLifecycleExecutorManyWaiters(t *testing.T) {
	t.Parallel()

	const waiters = 50
	scripts := func() []codersdk.WorkspaceAgentScript {
		all := []codersdk.WorkspaceAgentScript{scriptOrderTestScript("coder_script.clone")}
		for i := range waiters {
			all = append(all, scriptOrderTestScript(
				fmt.Sprintf("coder_script.dependent_%02d", i),
				scriptOrderTestDependency("coder_script.clone", requireSuccess),
			))
		}
		return all
	}

	t.Run("AllRun", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		runs := newScriptOrderRuns()
		runs.hold("coder_script.clone")
		executor := newTestExecutor(t, runs, newScriptOrderTestLogs())

		done := runExecute(ctx, executor, scripts()...)
		runs.waitStarted(ctx, t, "coder_script.clone")
		require.Len(t, runs.allStarted(), 1)

		runs.release("coder_script.clone", nil)
		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		require.Len(t, runs.allStarted(), waiters+1)
	})

	t.Run("AllCanceled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		defer cancel()

		runs := newScriptOrderRuns()
		runs.hold("coder_script.clone")
		reports := newScriptOrderReports()
		executor := newTestExecutor(t, runs, newScriptOrderTestLogs())
		executor.reportUnstarted = reports.record
		all := scripts()

		manager, byAddress, err := lifecycleManager(all)
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- executor.executeWith(ctx, manager, byAddress, all, ExecuteStartScripts) }()
		runs.waitStarted(ctx, t, "coder_script.clone")

		cancel()
		waitCtx := testutil.Context(t, testutil.WaitShort)
		require.ErrorIs(t, testutil.RequireReceive(waitCtx, t, done), context.Canceled)
		require.Len(t, runs.allStarted(), 1)
		for _, script := range all {
			u, err := manager.Unit(unit.ID(script.ResourceAddress))
			require.NoError(t, err)
			require.Equal(t, unit.OutcomeCanceled, u.Outcome(), script.ResourceAddress)
		}

		var want []scriptOrderReport
		for _, script := range all[1:] {
			want = append(want, scriptOrderReport{address: script.ResourceAddress, option: ExecuteStartScripts, status: proto.Timing_CANCELED})
		}
		require.ElementsMatch(t, want, reports.all(), "one report per waiting script, none for the running one")
	})
}

func newTestExecutor(t *testing.T, runs *scriptOrderRuns, logs *scriptOrderTestLogs) lifecycleExecutor {
	t.Helper()
	return lifecycleExecutor{
		clock:           quartz.NewReal(),
		logger:          testutil.Logger(t),
		getScriptLogger: logs.logger,
		run:             runs.run,
	}
}

func runExecute(ctx context.Context, executor lifecycleExecutor, scripts ...codersdk.WorkspaceAgentScript) <-chan error {
	done := make(chan error, 1)
	go func() { done <- executor.execute(ctx, scripts, ExecuteStartScripts) }()
	return done
}

func scriptOrderTestScript(address string, dependencies ...codersdk.WorkspaceAgentScriptDependency) codersdk.WorkspaceAgentScript {
	return codersdk.WorkspaceAgentScript{
		ID:              uuid.New(),
		LogSourceID:     uuid.New(),
		ResourceAddress: address,
		RunOnStart:      true,
		Dependencies:    dependencies,
	}
}

func scriptOrderTestDependency(address string, requirement codersdk.WorkspaceAgentScriptDependencyRequirement) codersdk.WorkspaceAgentScriptDependency {
	return codersdk.WorkspaceAgentScriptDependency{
		PrerequisiteResourceAddress: address,
		Requirement:                 requirement,
	}
}

// scriptOrderRuns is a fake run. A script's key is its resource address,
// or its log source ID when it has no address. A held script blocks until
// released; any other script returns its configured result at once.
type scriptOrderRuns struct {
	mu         sync.Mutex
	held       map[string]chan error
	results    map[string]error
	startedCh  map[string]chan struct{}
	finishedCh map[string]chan struct{}
	options    map[string]ExecuteOption
	order      []string
}

func newScriptOrderRuns() *scriptOrderRuns {
	return &scriptOrderRuns{
		held:       make(map[string]chan error),
		results:    make(map[string]error),
		startedCh:  make(map[string]chan struct{}),
		finishedCh: make(map[string]chan struct{}),
		options:    make(map[string]ExecuteOption),
	}
}

func scriptOrderKey(script codersdk.WorkspaceAgentScript) string {
	if script.ResourceAddress != "" {
		return script.ResourceAddress
	}
	return script.LogSourceID.String()
}

func (r *scriptOrderRuns) hold(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.held[key] = make(chan error, 1)
}

func (r *scriptOrderRuns) result(key string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[key] = err
}

func (r *scriptOrderRuns) release(key string, err error) {
	r.mu.Lock()
	release := r.held[key]
	r.mu.Unlock()
	release <- err
}

func signal(m map[string]chan struct{}, key string) chan struct{} {
	if _, ok := m[key]; !ok {
		m[key] = make(chan struct{})
	}
	return m[key]
}

func (r *scriptOrderRuns) run(ctx context.Context, script codersdk.WorkspaceAgentScript, option ExecuteOption) error {
	key := scriptOrderKey(script)
	r.mu.Lock()
	r.order = append(r.order, key)
	r.options[key] = option
	close(signal(r.startedCh, key))
	release, held := r.held[key]
	result := r.results[key]
	finished := signal(r.finishedCh, key)
	r.mu.Unlock()
	defer close(finished)

	if !held {
		return result
	}
	select {
	case err := <-release:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *scriptOrderRuns) waitStarted(ctx context.Context, t *testing.T, key string) {
	t.Helper()
	r.mu.Lock()
	started := signal(r.startedCh, key)
	r.mu.Unlock()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("script %q did not start", key)
	}
}

func (r *scriptOrderRuns) waitFinished(ctx context.Context, t *testing.T, key string) {
	t.Helper()
	r.mu.Lock()
	finished := signal(r.finishedCh, key)
	r.mu.Unlock()
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatalf("script %q did not finish", key)
	}
}

func (r *scriptOrderRuns) requireNotStarted(t *testing.T, key string) {
	t.Helper()
	require.False(t, r.started(key), "script %q started too early", key)
}

func (r *scriptOrderRuns) started(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	started, ok := r.startedCh[key]
	if !ok {
		return false
	}
	select {
	case <-started:
		return true
	default:
		return false
	}
}

func (r *scriptOrderRuns) allStarted() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}

func (r *scriptOrderRuns) option(key string) ExecuteOption {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.options[key]
}

// scriptOrderReports is a fake reportUnstarted that records each call.
type scriptOrderReports struct {
	mu      sync.Mutex
	records []scriptOrderReport
}

type scriptOrderReport struct {
	address string
	option  ExecuteOption
	status  proto.Timing_Status
}

func newScriptOrderReports() *scriptOrderReports {
	return &scriptOrderReports{}
}

func (r *scriptOrderReports) record(_ context.Context, script codersdk.WorkspaceAgentScript, option ExecuteOption, status proto.Timing_Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, scriptOrderReport{address: script.ResourceAddress, option: option, status: status})
}

func (r *scriptOrderReports) all() []scriptOrderReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]scriptOrderReport(nil), r.records...)
}

// scriptOrderTestLogs records every line sent to each script's log source.
type scriptOrderTestLogs struct {
	mu   sync.Mutex
	byID map[uuid.UUID][]string
	sent chan scriptOrderLog
}

type scriptOrderLog struct {
	logSourceID uuid.UUID
	output      string
}

func newScriptOrderTestLogs() *scriptOrderTestLogs {
	return &scriptOrderTestLogs{
		byID: make(map[uuid.UUID][]string),
		sent: make(chan scriptOrderLog, 256),
	}
}

func (l *scriptOrderTestLogs) logger(logSourceID uuid.UUID) ScriptLogger {
	return scriptOrderTestLogger{logs: l, logSourceID: logSourceID}
}

func (l *scriptOrderTestLogs) outputs(logSourceID uuid.UUID) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.byID[logSourceID]...)
}

// waitOutput blocks until the next line for the given log source arrives.
func (l *scriptOrderTestLogs) waitOutput(ctx context.Context, t *testing.T, logSourceID uuid.UUID) string {
	t.Helper()
	for {
		log := testutil.RequireReceive(ctx, t, l.sent)
		if log.logSourceID == logSourceID {
			return log.output
		}
	}
}

type scriptOrderTestLogger struct {
	logs        *scriptOrderTestLogs
	logSourceID uuid.UUID
}

func (l scriptOrderTestLogger) Send(_ context.Context, logs ...agentsdk.Log) error {
	l.logs.mu.Lock()
	defer l.logs.mu.Unlock()
	for _, log := range logs {
		l.logs.byID[l.logSourceID] = append(l.logs.byID[l.logSourceID], log.Output)
		l.logs.sent <- scriptOrderLog{logSourceID: l.logSourceID, output: log.Output}
	}
	return nil
}

func (scriptOrderTestLogger) Flush(context.Context) error {
	return nil
}
