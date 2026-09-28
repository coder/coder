package chattool_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const (
	interruptForegroundArgs = `{"command":"make test","timeout":"2h"}`
	interruptBackgroundArgs = `{"command":"make dev","run_in_background":true}`
)

func interruptIdentity() chattool.ToolCallIdentity {
	return chattool.ToolCallIdentity{
		ChatID:     uuid.New(),
		MessageID:  42,
		ToolCallID: "call-1",
		ToolName:   chattool.ExecuteToolName,
		Cause:      chattool.ToolCallCauseInterrupt,
	}
}

func runInterruptExecute(t *testing.T, conn workspacesdk.AgentConn, clock quartz.Clock, id chattool.ToolCallIdentity, args string) fantasy.ToolResponse {
	t.Helper()
	tool := chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
		Clock:            clock,
	})
	ctx := chattool.WithToolCallIdentity(testutil.Context(t, testutil.WaitShort), id)
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: id.ToolCallID, Name: chattool.ExecuteToolName, Input: args})
	require.NoError(t, err)
	return resp
}

func requireInterruptExecuteResult(t *testing.T, resp fantasy.ToolResponse) chattool.ExecuteResult {
	t.Helper()
	require.False(t, resp.IsError, resp.Content)
	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
	return result
}

// TestExecuteInterruptRun covers the execute tool in an interrupt run:
// it cancels the tool call first, sends the start only after the cancel
// succeeded, and builds the result from the agent's answers.
func TestExecuteInterruptRun(t *testing.T) {
	t.Parallel()

	intPtr := func(v int) *int { return &v }
	tests := []struct {
		name      string
		args      string
		cancelErr error
		// start is nil when no start may follow the cancel.
		start *startAnswer
		// output is nil when no output read may follow the start.
		output *outputAnswer
		// generic is set for the generic interrupted result.
		generic bool
		check   func(t *testing.T, id chattool.ToolCallIdentity, result chattool.ExecuteResult)
	}{
		{
			name:   "ForegroundCanceled",
			args:   interruptForegroundArgs,
			start:  &startAnswer{},
			output: &outputAnswer{wait: true, resp: workspacesdk.ProcessOutputResponse{Canceled: true, Output: "partial", ExitCode: intPtr(-1), DurationMs: 1500}},
			check: func(t *testing.T, _ chattool.ToolCallIdentity, result chattool.ExecuteResult) {
				assert.False(t, result.Success)
				assert.Equal(t, "partial", result.Output)
				assert.Contains(t, result.Error, "canceled by the user after 1.5s")
				assert.EqualValues(t, 1500, result.WallDurationMs)
			},
		},
		{
			name:   "ForegroundExitedBeforeCancel",
			args:   interruptForegroundArgs,
			start:  &startAnswer{},
			output: &outputAnswer{wait: true, resp: workspacesdk.ProcessOutputResponse{Output: "ok", ExitCode: intPtr(0), DurationMs: 300}},
			check: func(t *testing.T, _ chattool.ToolCallIdentity, result chattool.ExecuteResult) {
				assert.True(t, result.Success)
				assert.Equal(t, "ok", result.Output)
				assert.Empty(t, result.Error)
			},
		},
		{
			// US10: the agent never received the start.
			name:  "NotReceived",
			args:  interruptForegroundArgs,
			start: &startAnswer{err: &workspacesdk.ToolCallError{Code: workspacesdk.ToolCallErrorCanceled}},
			check: func(t *testing.T, _ chattool.ToolCallIdentity, result chattool.ExecuteResult) {
				assert.Contains(t, result.Error, "not run")
				assert.Contains(t, result.Error, "canceled before the workspace agent received it")
			},
		},
		{
			name:  "UnknownToAgent",
			args:  interruptForegroundArgs,
			start: &startAnswer{err: &workspacesdk.ToolCallError{Code: workspacesdk.ToolCallErrorUnknown}},
			check: func(t *testing.T, _ chattool.ToolCallIdentity, result chattool.ExecuteResult) {
				assert.Contains(t, result.Error, "outcome unknown")
			},
		},
		{
			name:      "CancelRouteMissing",
			args:      interruptForegroundArgs,
			cancelErr: codersdk.NewTestError(http.StatusNotFound, http.MethodPost, "/api/v0/tool-calls/x/cancel"),
			generic:   true,
		},
		{
			name:      "CancelRefused",
			args:      interruptBackgroundArgs,
			cancelErr: codersdk.NewTestError(http.StatusBadRequest, http.MethodPost, "/api/v0/tool-calls/x/cancel"),
			check: func(t *testing.T, id chattool.ToolCallIdentity, result chattool.ExecuteResult) {
				assert.Contains(t, result.Error, "outcome unknown")
				assert.Contains(t, result.Error, id.UUID())
				assert.Empty(t, result.BackgroundProcessID)
			},
		},
		{
			name:   "BackgroundCanceled",
			args:   interruptBackgroundArgs,
			start:  &startAnswer{},
			output: &outputAnswer{resp: workspacesdk.ProcessOutputResponse{Canceled: true, Output: "serving", ExitCode: intPtr(-1), DurationMs: 60_000}},
			check: func(t *testing.T, _ chattool.ToolCallIdentity, result chattool.ExecuteResult) {
				assert.Equal(t, "serving", result.Output)
				assert.Contains(t, result.Error, "canceled by the user after 1m0s")
				assert.False(t, result.Backgrounded)
				assert.Empty(t, result.BackgroundProcessID)
			},
		},
		{
			name:   "BackgroundStillRunning",
			args:   `{"command":"make dev &"}`,
			start:  &startAnswer{},
			output: &outputAnswer{resp: workspacesdk.ProcessOutputResponse{Running: true}},
			check: func(t *testing.T, id chattool.ToolCallIdentity, result chattool.ExecuteResult) {
				assert.True(t, result.Backgrounded)
				assert.Equal(t, id.UUID(), result.BackgroundProcessID)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id := interruptIdentity()
			conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			calls := []any{
				conn.EXPECT().CancelToolCall(gomock.Any(), id.UUID()).
					DoAndReturn(func(ctx context.Context, _ string) error {
						requireAgentToolCall(ctx, t, id)
						return tc.cancelErr
					}),
			}
			if tc.start != nil {
				calls = append(calls, conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
						requireAgentToolCall(ctx, t, id)
						if tc.start.err != nil {
							return workspacesdk.StartProcessResponse{}, tc.start.err
						}
						return workspacesdk.StartProcessResponse{ID: id.UUID(), Started: true}, nil
					}))
			}
			if tc.output != nil {
				calls = append(calls, conn.EXPECT().ProcessOutput(gomock.Any(), id.UUID(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, opts *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
						assert.Equal(t, tc.output.wait, opts != nil && opts.Wait, "output wait")
						return tc.output.resp, nil
					}))
			}
			gomock.InOrder(calls...)

			resp := runInterruptExecute(t, conn, quartz.NewMock(t), id, tc.args)
			if tc.generic {
				require.True(t, resp.IsError)
				assert.Equal(t, chattool.InterruptedToolResultMessage, resp.Content)
				return
			}
			tc.check(t, id, requireInterruptExecuteResult(t, resp))
		})
	}
}

type startAnswer struct {
	err error
}

type outputAnswer struct {
	wait bool
	resp workspacesdk.ProcessOutputResponse
}

func requireAgentToolCall(ctx context.Context, t *testing.T, id chattool.ToolCallIdentity) {
	t.Helper()
	tc, ok := workspacesdk.ToolCallFromContext(ctx)
	if assert.True(t, ok, "request must carry the tool call") {
		assert.Equal(t, id.AgentToolCall(), tc)
	}
}

// TestExecuteInterruptRun_CancelNoAnswer shows that a cancel the agent
// never answers yields an unknown result once AgentAnswerTimeout passes,
// and that no start follows it.
func TestExecuteInterruptRun_CancelNoAnswer(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().AfterFunc("chattool", "agent-answer")
	defer trap.Close()

	id := interruptIdentity()
	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	conn.EXPECT().CancelToolCall(gomock.Any(), id.UUID()).
		DoAndReturn(func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return &url.Error{Op: http.MethodPost, URL: "http://agent/api/v0/tool-calls", Err: ctx.Err()}
		})

	done := make(chan fantasy.ToolResponse)
	go func() { done <- runInterruptExecute(t, conn, clock, id, interruptForegroundArgs) }()

	call := trap.MustWait(ctx)
	assert.Equal(t, chattool.AgentAnswerTimeout, call.Duration)
	call.MustRelease(ctx)
	clock.Advance(chattool.AgentAnswerTimeout).MustWait(ctx)

	result := requireInterruptExecuteResult(t, testutil.RequireReceive(ctx, t, done))
	assert.Contains(t, result.Error, "outcome unknown")
	assert.Contains(t, result.Error, id.UUID())
}

// TestExecuteInterruptRun_OutputWaitBound shows that the output wait of
// a foreground call in an interrupt run is bounded by AgentAnswerTimeout,
// not the agent's wait cap.
func TestExecuteInterruptRun_OutputWaitBound(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().AfterFunc("chattool", "agent-answer")
	defer trap.Close()

	id := interruptIdentity()
	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	gomock.InOrder(
		conn.EXPECT().CancelToolCall(gomock.Any(), id.UUID()).Return(nil),
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
			Return(workspacesdk.StartProcessResponse{ID: id.UUID(), Started: true}, nil),
		conn.EXPECT().ProcessOutput(gomock.Any(), id.UUID(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string, _ *workspacesdk.ProcessOutputOptions) (workspacesdk.ProcessOutputResponse, error) {
				<-ctx.Done()
				return workspacesdk.ProcessOutputResponse{}, &url.Error{Op: http.MethodGet, URL: "http://agent/api/v0/processes", Err: ctx.Err()}
			}),
	)

	done := make(chan fantasy.ToolResponse)
	go func() { done <- runInterruptExecute(t, conn, clock, id, interruptForegroundArgs) }()

	// The cancel, the start, then the output wait.
	for range 3 {
		call := trap.MustWait(ctx)
		assert.Equal(t, chattool.AgentAnswerTimeout, call.Duration)
		call.MustRelease(ctx)
	}
	clock.Advance(chattool.AgentAnswerTimeout).MustWait(ctx)

	result := requireInterruptExecuteResult(t, testutil.RequireReceive(ctx, t, done))
	assert.Contains(t, result.Error, "outcome unknown")
	assert.Equal(t, id.UUID(), result.BackgroundProcessID)
}
