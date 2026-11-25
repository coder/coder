package agentscripts

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRunnerReportsUnstartedTiming(t *testing.T) {
	t.Parallel()

	newRunner := func(t *testing.T, now time.Time) (*Runner, codersdk.WorkspaceAgentScript, chan *proto.WorkspaceAgentScriptCompletedRequest) {
		t.Helper()
		clock := quartz.NewMock(t)
		clock.Set(now)
		runner := New(Options{
			Logger:     testutil.Logger(t),
			Filesystem: afero.NewMemMapFs(),
			Clock:      clock,
		})
		completed := make(chan *proto.WorkspaceAgentScriptCompletedRequest, 1)
		script := scriptOrderTestScript("coder_script.install")
		require.NoError(t, runner.Init([]codersdk.WorkspaceAgentScript{script}, func(_ context.Context, req *proto.WorkspaceAgentScriptCompletedRequest) (*proto.WorkspaceAgentScriptCompletedResponse, error) {
			completed <- req
			return &proto.WorkspaceAgentScriptCompletedResponse{}, nil
		}))
		return runner, script, completed
	}

	for _, test := range []struct {
		name       string
		option     ExecuteOption
		status     proto.Timing_Status
		wantReport bool
	}{
		{name: "StartSkipped", option: ExecuteStartScripts, status: proto.Timing_SKIPPED, wantReport: true},
		{name: "StartCanceled", option: ExecuteStartScripts, status: proto.Timing_CANCELED, wantReport: true},
		{name: "StopSkipped", option: ExecuteStopScripts, status: proto.Timing_SKIPPED},
		{name: "StopCanceled", option: ExecuteStopScripts, status: proto.Timing_CANCELED},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
			runner, script, completed := newRunner(t, now)

			// A canceled execution context must not lose the report.
			execCtx, cancel := context.WithCancel(ctx)
			cancel()
			runner.reportUnstarted(execCtx, script, test.option, test.status)
			require.NoError(t, runner.Close())

			if !test.wantReport {
				require.Empty(t, completed)
				return
			}
			req := testutil.RequireReceive(ctx, t, completed)
			require.Equal(t, script.ID[:], req.Timing.ScriptId)
			require.Equal(t, now, req.Timing.Start.AsTime())
			require.Equal(t, now, req.Timing.End.AsTime())
			require.Zero(t, req.Timing.ExitCode)
			require.Equal(t, proto.Timing_START, req.Timing.Stage)
			require.Equal(t, test.status, req.Timing.Status)
		})
	}

	t.Run("AfterClose", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		runner, script, completed := newRunner(t, time.Now())

		require.NoError(t, runner.Close())
		runner.reportUnstarted(ctx, script, ExecuteStartScripts, proto.Timing_SKIPPED)
		require.Empty(t, completed)
	})
}
