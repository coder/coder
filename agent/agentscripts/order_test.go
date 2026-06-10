package agentscripts_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentscripts"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestScriptOrder(t *testing.T) {
	t.Parallel()

	startScript := func(displayName, script string, deps ...codersdk.WorkspaceAgentScriptOrderDependency) codersdk.WorkspaceAgentScript {
		return codersdk.WorkspaceAgentScript{
			ID:                uuid.New(),
			LogSourceID:       uuid.New(),
			DisplayName:       displayName,
			Script:            script,
			RunOnStart:        true,
			OrderDependencies: deps,
		}
	}
	afterSuccess := func(s codersdk.WorkspaceAgentScript) codersdk.WorkspaceAgentScriptOrderDependency {
		return codersdk.WorkspaceAgentScriptOrderDependency{
			ScriptID: s.ID,
			Requires: codersdk.WorkspaceAgentScriptOrderRequiresSuccess,
		}
	}
	afterCompletion := func(s codersdk.WorkspaceAgentScript) codersdk.WorkspaceAgentScriptOrderDependency {
		return codersdk.WorkspaceAgentScriptOrderDependency{
			ScriptID: s.ID,
			Requires: codersdk.WorkspaceAgentScriptOrderRequiresCompletion,
		}
	}
	// drainLogs empties the fake logger. Callers invoke it after Execute
	// has returned, so no further logs can arrive.
	drainLogs := func(fLogger *fakeScriptLogger) []string {
		var outputs []string
		for {
			select {
			case log := <-fLogger.logs:
				outputs = append(outputs, log.Output)
			default:
				return outputs
			}
		}
	}

	t.Run("RunsInDependencyOrder", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		fLogger := newFakeScriptLogger()
		runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger })
		defer runner.Close()

		first := startScript("first", "echo first")
		second := startScript("second", "echo second", afterSuccess(first))
		third := startScript("third", "echo third", afterSuccess(second))

		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
		// Scripts are declared in reverse order to prove that execution
		// order comes from the dependency graph, not slice order.
		err := runner.Init([]codersdk.WorkspaceAgentScript{third, second, first}, aAPI.ScriptCompleted)
		require.NoError(t, err)
		require.NoError(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))

		require.Equal(t, []string{"first", "second", "third"}, drainLogs(fLogger))
	})

	t.Run("SkipsWhenRequiredDependencyFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		fLogger := newFakeScriptLogger()
		runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger })
		defer runner.Close()

		boom := startScript("boom", "exit 1")
		dependent := startScript("dependent", "echo notrun", afterSuccess(boom))

		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
		err := runner.Init([]codersdk.WorkspaceAgentScript{boom, dependent}, aAPI.ScriptCompleted)
		require.NoError(t, err)
		// The failing dependency is the only error; the skip itself is
		// not one.
		require.Error(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))

		outputs := drainLogs(fLogger)
		require.Len(t, outputs, 1)
		require.Contains(t, outputs[0], `dependency "boom" failed`)
		require.NotContains(t, strings.Join(outputs, "\n"), "notrun")

		// The failed dependency reports its own timing; the skipped
		// script reports a zero-duration SKIPPED timing so the skip is
		// visible in the build timeline.
		require.NoError(t, runner.Close())
		timings := aAPI.GetTimings()
		require.Len(t, timings, 2)
		var skipped *proto.Timing
		for _, timing := range timings {
			if timing.Status == proto.Timing_SKIPPED {
				skipped = timing
			}
		}
		require.NotNil(t, skipped)
		require.Equal(t, dependent.ID[:], skipped.ScriptId)
		require.Equal(t, proto.Timing_START, skipped.Stage)
		require.EqualValues(t, -1, skipped.ExitCode)
		require.True(t, skipped.Start.AsTime().Equal(skipped.End.AsTime()))
	})

	t.Run("RunsWhenCompletionDependencyFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		fLogger := newFakeScriptLogger()
		runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger })
		defer runner.Close()

		boom := startScript("boom", "exit 1")
		dependent := startScript("dependent", "echo ran", afterCompletion(boom))

		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
		err := runner.Init([]codersdk.WorkspaceAgentScript{boom, dependent}, aAPI.ScriptCompleted)
		require.NoError(t, err)
		require.Error(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))

		require.Equal(t, []string{"ran"}, drainLogs(fLogger))
	})

	t.Run("TransitiveSkip", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		fLogger := newFakeScriptLogger()
		runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger })
		defer runner.Close()

		boom := startScript("boom", "exit 1")
		second := startScript("second", "echo second", afterSuccess(boom))
		third := startScript("third", "echo third", afterSuccess(second))

		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
		err := runner.Init([]codersdk.WorkspaceAgentScript{boom, second, third}, aAPI.ScriptCompleted)
		require.NoError(t, err)
		require.Error(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))

		outputs := drainLogs(fLogger)
		require.Len(t, outputs, 2)
		require.Contains(t, outputs[0], `dependency "boom" failed`)
		require.Contains(t, outputs[1], `dependency "second" was skipped`)
	})

	t.Run("IgnoresUnsatisfiableDependency", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		fLogger := newFakeScriptLogger()
		runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger })
		defer runner.Close()

		// The dependency target is not a start script in this runner
		// (e.g. an extracted devcontainer script), so the rule is
		// dropped and the script runs ungated.
		dependent := startScript("dependent", "echo ran", codersdk.WorkspaceAgentScriptOrderDependency{
			ScriptID: uuid.New(),
			Requires: codersdk.WorkspaceAgentScriptOrderRequiresSuccess,
		})

		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
		err := runner.Init([]codersdk.WorkspaceAgentScript{dependent}, aAPI.ScriptCompleted)
		require.NoError(t, err)
		require.NoError(t, runner.Execute(ctx, agentscripts.ExecuteStartScripts))

		require.Equal(t, []string{"ran"}, drainLogs(fLogger))
	})

	t.Run("LogsWaitingHeartbeat", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		fLogger := newFakeScriptLogger()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("agentscripts", "scriptOrderWait")
		defer trap.Close()
		runner := setup(t, func(uuid.UUID) agentscripts.ScriptLogger { return fLogger }, func(opts *agentscripts.Options) {
			opts.Clock = mClock
		})
		defer runner.Close()

		// The dependency blocks until the gate file exists so the
		// dependent stays in its wait loop while the clock advances.
		gate := filepath.Join(t.TempDir(), "release")
		slow := startScript("slow", fmt.Sprintf("while [ ! -e %q ]; do sleep 0.05; done", gate))
		dependent := startScript("dependent", "echo ran", afterSuccess(slow))

		aAPI := agenttest.NewFakeAgentAPI(t, testutil.Logger(t), nil, nil)
		err := runner.Init([]codersdk.WorkspaceAgentScript{slow, dependent}, aAPI.ScriptCompleted)
		require.NoError(t, err)

		execDone := make(chan error, 1)
		go func() {
			execDone <- runner.Execute(ctx, agentscripts.ExecuteStartScripts)
		}()

		// Both ordered participants create a wait ticker. Releasing both
		// guarantees the dependent's ticker exists before the clock
		// advances.
		for range 2 {
			trap.MustWait(ctx).MustRelease(ctx)
		}
		mClock.Advance(30 * time.Second).MustWait(ctx)

		log := testutil.RequireReceive(ctx, t, fLogger.logs)
		require.Contains(t, log.Output, `Waiting for "slow"`)
		require.Contains(t, log.Output, "(30s)")
		require.Equal(t, codersdk.LogLevelInfo, log.Level)

		require.NoError(t, os.WriteFile(gate, nil, 0o600))
		require.NoError(t, testutil.RequireReceive(ctx, t, execDone))
		require.Equal(t, []string{"ran"}, drainLogs(fLogger))
	})
}
