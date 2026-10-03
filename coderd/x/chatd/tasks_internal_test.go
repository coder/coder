package chatd

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestTaskAttemptContext_ProcessWaitBudget(t *testing.T) {
	t.Parallel()

	t.Run("ParallelWaits", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		parent := testutil.Context(t, testutil.WaitLong)
		ctx, cancel := taskAttemptContext(parent, clock, taskKindGeneration)
		defer cancel()
		var wg sync.WaitGroup
		for _, timeout := range []string{"31m", "10s", "20m"} {
			wg.Go(func() { reserveTestProcessWait(ctx, t, timeout) })
		}
		wg.Wait()
		clock.Advance(20 * time.Minute).MustWait(parent)
		require.NoError(t, ctx.Err())
		reserveTestProcessWait(ctx, t, "10s")
		clock.Advance(16 * time.Minute).MustWait(parent)
		require.ErrorIs(t, context.Cause(ctx), errTaskTimeout)
	})

	t.Run("ParentCancellation", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		parent, cancelParent := context.WithCancel(t.Context())
		defer cancelParent()
		ctx, cancel := taskAttemptContext(parent, clock, taskKindGeneration)
		defer cancel()
		reserveTestProcessWait(ctx, t, "31m")
		cancelParent()
		require.ErrorIs(t, context.Cause(ctx), context.Canceled)
		reserveTestProcessWait(ctx, t, "60m")
		require.ErrorIs(t, context.Cause(ctx), context.Canceled)
	})

	t.Run("NextAttemptUsesDefault", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		parent := testutil.Context(t, testutil.WaitLong)
		ctx, cancel := taskAttemptContext(parent, clock, taskKindGeneration)
		reserveTestProcessWait(ctx, t, "31m")
		cancel()
		next, cancelNext := taskAttemptContext(parent, clock, taskKindGeneration)
		defer cancelNext()
		clock.Advance(defaultTaskTimeout).MustWait(parent)
		require.ErrorIs(t, context.Cause(next), errTaskTimeout)
	})

	t.Run("MaximumDurationDoesNotOverflow", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		parent := testutil.Context(t, testutil.WaitLong)
		ctx, cancel := taskAttemptContext(parent, clock, taskKindGeneration)
		defer cancel()
		reserveTestProcessWait(ctx, t, "2562047h47m16.854775807s")
		clock.Advance(defaultTaskTimeout).MustWait(parent)
		require.NoError(t, ctx.Err())
	})
}

func reserveTestProcessWait(ctx context.Context, t *testing.T, timeout string) {
	t.Helper()
	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	conn.EXPECT().ProcessOutput(gomock.Any(), "process", gomock.Any()).Return(workspacesdk.ProcessOutputResponse{}, nil)
	tool := chattool.ProcessOutput(chattool.ProcessToolOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
	})
	input, err := json.Marshal(chattool.ProcessOutputArgs{ProcessID: "process", WaitTimeout: &timeout})
	require.NoError(t, err)
	_, err = tool.Run(ctx, fantasy.ToolCall{ID: "call", Name: "process_output", Input: string(input)})
	require.NoError(t, err)
}

func TestTaskAttemptContext_LongProcessWait(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"execute", "process_output"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			clock := quartz.NewMock(t)
			parent := testutil.Context(t, testutil.WaitLong)
			ctx, cancel := taskAttemptContext(parent, clock, taskKindGeneration)
			defer cancel()
			conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			getConn := func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil }
			var tool fantasy.AgentTool
			var input string
			if name == "execute" {
				tool = chattool.Execute(chattool.ExecuteOptions{GetWorkspaceConn: getConn})
				input = `{"command":"long-command","timeout":"31m"}`
				conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).Return(workspacesdk.StartProcessResponse{ID: "process"}, nil).Times(1)
			} else {
				tool = chattool.ProcessOutput(chattool.ProcessToolOptions{GetWorkspaceConn: getConn})
				input = `{"process_id":"process","wait_timeout":"31m"}`
			}
			conn.EXPECT().ProcessOutput(gomock.Any(), "process", gomock.Any()).DoAndReturn(
				func(callCtx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
					clock.Advance(15 * time.Minute).MustWait(parent)
					require.NoError(t, callCtx.Err(), "the task must not cancel a valid command wait")
					clock.Advance(5 * time.Minute).MustWait(parent)
					require.NoError(t, callCtx.Err(), "the task must not cancel a valid command wait")
					return workspacesdk.ProcessOutputResponse{Output: "done", ExitCode: new(0)}, nil
				})
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call", Name: name, Input: input})
			require.NoError(t, err)
			var result chattool.ExecuteResult
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			require.True(t, result.Success)
			require.Equal(t, "done", result.Output)
			require.NoError(t, ctx.Err(), "the result must still be able to commit")

			clock.Advance(16 * time.Minute).MustWait(parent)
			require.ErrorIs(t, context.Cause(ctx), errTaskTimeout, "the extended attempt must remain bounded")
		})
	}
}
