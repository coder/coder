package agentscripts

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestLifecycleExecutorOrdersScripts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	releasePrerequisite := make(chan struct{})
	started := make(chan string, 2)
	executor := lifecycleExecutor{
		clock:           quartz.NewReal(),
		logger:          slogtest.Make(t, nil),
		getScriptLogger: newScriptOrderTestLogs().logger,
		run: func(ctx context.Context, script codersdk.WorkspaceAgentScript, _ ExecuteOption) error {
			started <- script.ResourceAddress
			if script.ResourceAddress == "coder_script.clone" {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-releasePrerequisite:
				}
			}
			return nil
		},
	}
	scripts := []codersdk.WorkspaceAgentScript{
		scriptOrderTestScript("coder_script.clone"),
		scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess)),
	}

	done := make(chan error, 1)
	go func() {
		done <- executor.execute(ctx, scripts, ExecuteStartScripts)
	}()

	require.Equal(t, "coder_script.clone", testutil.RequireReceive(ctx, t, started))
	select {
	case address := <-started:
		t.Fatalf("dependent script %q started before its prerequisite completed", address)
	default:
	}
	close(releasePrerequisite)
	require.Equal(t, "coder_script.install", testutil.RequireReceive(ctx, t, started))
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

func TestLifecycleExecutorRequirements(t *testing.T) {
	t.Parallel()

	errFailure := xerrors.New("script failed")
	tests := []struct {
		name            string
		requirement     codersdk.WorkspaceAgentScriptDependencyRequirement
		prerequisiteErr error
		wantDependent   bool
		wantSkipLog     string
	}{
		{
			name:            "success skips after failure",
			requirement:     codersdk.WorkspaceAgentScriptDependencyRequirementSuccess,
			prerequisiteErr: errFailure,
			wantSkipLog:     `Skipping script: dependency "Clone" failed.`,
		},
		{
			name:            "success skips after timeout",
			requirement:     codersdk.WorkspaceAgentScriptDependencyRequirementSuccess,
			prerequisiteErr: ErrTimeout,
			wantSkipLog:     `Skipping script: dependency "Clone" failed.`,
		},
		{
			name:            "completion runs after failure",
			requirement:     codersdk.WorkspaceAgentScriptDependencyRequirementCompletion,
			prerequisiteErr: errFailure,
			wantDependent:   true,
		},
		{
			name:            "completion runs after timeout",
			requirement:     codersdk.WorkspaceAgentScriptDependencyRequirementCompletion,
			prerequisiteErr: ErrTimeout,
			wantDependent:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			logs := newScriptOrderTestLogs()
			dependentRan := make(chan struct{}, 1)
			executor := lifecycleExecutor{
				clock:           quartz.NewReal(),
				logger:          slogtest.Make(t, nil),
				getScriptLogger: logs.logger,
				run: func(_ context.Context, script codersdk.WorkspaceAgentScript, _ ExecuteOption) error {
					if script.ResourceAddress == "coder_script.clone" {
						return test.prerequisiteErr
					}
					dependentRan <- struct{}{}
					return nil
				},
			}
			clone := scriptOrderTestScript("coder_script.clone")
			clone.DisplayName = "Clone"
			install := scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", test.requirement))

			err := executor.execute(ctx, []codersdk.WorkspaceAgentScript{clone, install}, ExecuteStartScripts)
			require.ErrorIs(t, err, test.prerequisiteErr)
			if test.wantDependent {
				testutil.RequireReceive(ctx, t, dependentRan)
			} else {
				select {
				case <-dependentRan:
					t.Fatal("dependent script ran after an unsatisfied success dependency")
				default:
				}
			}
			outputs := logs.outputs(install.LogSourceID)
			if test.wantSkipLog == "" {
				require.Empty(t, outputs)
			} else {
				require.Equal(t, []string{test.wantSkipLog}, outputs)
			}
		})
	}
}

func TestLifecycleExecutorPropagatesSkippedOutcomes(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logs := newScriptOrderTestLogs()
	executed := map[string]bool{}
	var executedMu sync.Mutex
	executor := lifecycleExecutor{
		clock:           quartz.NewReal(),
		logger:          slogtest.Make(t, nil),
		getScriptLogger: logs.logger,
		run: func(_ context.Context, script codersdk.WorkspaceAgentScript, _ ExecuteOption) error {
			executedMu.Lock()
			executed[script.ResourceAddress] = true
			executedMu.Unlock()
			if script.ResourceAddress == "coder_script.a" {
				return xerrors.New("a failed")
			}
			return nil
		},
	}
	a := scriptOrderTestScript("coder_script.a")
	b := scriptOrderTestScript("coder_script.b", scriptOrderTestDependency("coder_script.a", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess))
	c := scriptOrderTestScript("coder_script.c", scriptOrderTestDependency("coder_script.b", codersdk.WorkspaceAgentScriptDependencyRequirementCompletion))
	d := scriptOrderTestScript("coder_script.d", scriptOrderTestDependency("coder_script.b", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess))

	require.Error(t, executor.execute(ctx, []codersdk.WorkspaceAgentScript{a, b, c, d}, ExecuteStartScripts))
	executedMu.Lock()
	require.Equal(t, map[string]bool{
		"coder_script.a": true,
		"coder_script.c": true,
	}, executed)
	executedMu.Unlock()
	require.Equal(t, []string{`Skipping script: dependency "coder_script.a" failed.`}, logs.outputs(b.LogSourceID))
	require.Equal(t, []string{`Skipping script: dependency "coder_script.b" was skipped.`}, logs.outputs(d.LogSourceID))
}

func TestLifecycleExecutorWaitsForEveryPrerequisite(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	releaseA := make(chan struct{})
	releaseB := make(chan struct{})
	started := make(chan string, 3)
	executor := lifecycleExecutor{
		clock:           quartz.NewReal(),
		logger:          slogtest.Make(t, nil),
		getScriptLogger: newScriptOrderTestLogs().logger,
		run: func(ctx context.Context, script codersdk.WorkspaceAgentScript, _ ExecuteOption) error {
			started <- script.ResourceAddress
			var release <-chan struct{}
			switch script.ResourceAddress {
			case "coder_script.a":
				release = releaseA
			case "coder_script.b":
				release = releaseB
			default:
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
				return nil
			}
		},
	}
	c := scriptOrderTestScript("coder_script.c",
		scriptOrderTestDependency("coder_script.b", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess),
		scriptOrderTestDependency("coder_script.a", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess),
	)
	done := make(chan error, 1)
	go func() {
		done <- executor.execute(ctx, []codersdk.WorkspaceAgentScript{
			scriptOrderTestScript("coder_script.a"),
			scriptOrderTestScript("coder_script.b"),
			c,
		}, ExecuteStartScripts)
	}()

	startedPrerequisites := map[string]bool{}
	for range 2 {
		startedPrerequisites[testutil.RequireReceive(ctx, t, started)] = true
	}
	require.Equal(t, map[string]bool{"coder_script.a": true, "coder_script.b": true}, startedPrerequisites)
	close(releaseA)
	select {
	case address := <-started:
		t.Fatalf("dependent script %q started before every prerequisite completed", address)
	default:
	}
	close(releaseB)
	require.Equal(t, "coder_script.c", testutil.RequireReceive(ctx, t, started))
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

func TestLifecycleExecutorCancellationDoesNotSkipWaitingScripts(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	logs := newScriptOrderTestLogs()
	started := make(chan string, 2)
	executor := lifecycleExecutor{
		clock:           quartz.NewReal(),
		logger:          slogtest.Make(t, nil),
		getScriptLogger: logs.logger,
		run: func(ctx context.Context, script codersdk.WorkspaceAgentScript, _ ExecuteOption) error {
			started <- script.ResourceAddress
			<-ctx.Done()
			return ctx.Err()
		},
	}
	a := scriptOrderTestScript("coder_script.a")
	b := scriptOrderTestScript("coder_script.b", scriptOrderTestDependency("coder_script.a", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess))
	done := make(chan error, 1)
	go func() {
		done <- executor.execute(ctx, []codersdk.WorkspaceAgentScript{a, b}, ExecuteStartScripts)
	}()

	require.Equal(t, "coder_script.a", testutil.RequireReceive(ctx, t, started))
	cancel()
	waitCtx := testutil.Context(t, testutil.WaitShort)
	require.ErrorIs(t, testutil.RequireReceive(waitCtx, t, done), context.Canceled)
	select {
	case address := <-started:
		t.Fatalf("waiting script %q started after lifecycle cancellation", address)
	default:
	}
	require.Empty(t, logs.outputs(b.LogSourceID))
}

func TestLifecycleExecutorWaitingLogsAreCombinedAndSorted(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	tickerTrap := clock.Trap().NewTicker()
	defer tickerTrap.Close()
	logs := newScriptOrderTestLogs()
	release := make(chan struct{})
	started := make(chan string, 2)
	executor := lifecycleExecutor{
		clock:           clock,
		logger:          slogtest.Make(t, nil),
		getScriptLogger: logs.logger,
		run: func(ctx context.Context, script codersdk.WorkspaceAgentScript, _ ExecuteOption) error {
			if script.ResourceAddress == "coder_script.install" {
				return nil
			}
			started <- script.ResourceAddress
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
				return nil
			}
		},
	}
	install := scriptOrderTestScript("coder_script.install",
		scriptOrderTestDependency("coder_script.z_clone", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess),
		scriptOrderTestDependency("coder_script.a_auth", codersdk.WorkspaceAgentScriptDependencyRequirementCompletion),
	)
	done := make(chan error, 1)
	go func() {
		done <- executor.execute(ctx, []codersdk.WorkspaceAgentScript{
			scriptOrderTestScript("coder_script.z_clone"),
			scriptOrderTestScript("coder_script.a_auth"),
			install,
		}, ExecuteStartScripts)
	}()

	for range 2 {
		testutil.RequireReceive(ctx, t, started)
	}
	tickerTrap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(30 * time.Second).MustWait(ctx)
	require.Equal(t, scriptOrderLog{
		logSourceID: install.LogSourceID,
		output:      `Waiting for dependencies: "coder_script.a_auth", "coder_script.z_clone"... (30s)`,
	}, testutil.RequireReceive(ctx, t, logs.sent))
	close(release)
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

func TestLifecycleExecutorRejectsInvalidDependenciesBeforeRunning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scripts  []codersdk.WorkspaceAgentScript
		contains string
	}{
		{
			name: "dependent has no address",
			scripts: []codersdk.WorkspaceAgentScript{{
				DisplayName: "install",
				Dependencies: []codersdk.WorkspaceAgentScriptDependency{
					scriptOrderTestDependency("coder_script.clone", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess),
				},
			}},
			contains: "has dependencies but no Terraform resource address",
		},
		{
			name: "duplicate address",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.setup"),
				scriptOrderTestScript("coder_script.setup"),
			},
			contains: "duplicate script resource address",
		},
		{
			name: "missing prerequisite",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", codersdk.WorkspaceAgentScriptDependencyRequirementSuccess)),
			},
			contains: "not part of this lifecycle execution",
		},
		{
			name: "unsupported requirement",
			scripts: []codersdk.WorkspaceAgentScript{
				scriptOrderTestScript("coder_script.clone"),
				scriptOrderTestScript("coder_script.install", scriptOrderTestDependency("coder_script.clone", "started")),
			},
			contains: "unsupported requirement",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ran := make(chan struct{}, len(test.scripts))
			executor := lifecycleExecutor{
				clock:           quartz.NewReal(),
				logger:          slogtest.Make(t, nil),
				getScriptLogger: newScriptOrderTestLogs().logger,
				run: func(context.Context, codersdk.WorkspaceAgentScript, ExecuteOption) error {
					ran <- struct{}{}
					return nil
				},
			}

			err := executor.execute(t.Context(), test.scripts, ExecuteStartScripts)
			require.ErrorContains(t, err, test.contains)
			require.Empty(t, ran)
		})
	}
}

func TestScriptOutcome(t *testing.T) {
	t.Parallel()

	require.Equal(t, unit.OutcomeSucceeded, scriptOutcome(nil))
	require.Equal(t, unit.OutcomeTimedOut, scriptOutcome(ErrTimeout))
	require.Equal(t, unit.OutcomeFailed, scriptOutcome(ErrOutputPipesOpen))
	require.Equal(t, unit.OutcomeFailed, scriptOutcome(xerrors.New("failed")))
}

func scriptOrderTestScript(address string, dependencies ...codersdk.WorkspaceAgentScriptDependency) codersdk.WorkspaceAgentScript {
	return codersdk.WorkspaceAgentScript{
		ID:              uuid.New(),
		LogSourceID:     uuid.New(),
		DisplayName:     address,
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

type scriptOrderLog struct {
	logSourceID uuid.UUID
	output      string
}

type scriptOrderTestLogs struct {
	mu   sync.Mutex
	byID map[uuid.UUID][]string
	sent chan scriptOrderLog
}

func newScriptOrderTestLogs() *scriptOrderTestLogs {
	return &scriptOrderTestLogs{
		byID: make(map[uuid.UUID][]string),
		sent: make(chan scriptOrderLog, 16),
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
