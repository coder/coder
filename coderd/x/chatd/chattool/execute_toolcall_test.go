package chattool_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func newToolCallIdentity() chattool.ToolCallIdentity {
	return chattool.ToolCallIdentity{
		ChatID:     uuid.New(),
		MessageID:  42,
		ToolCallID: "call-1",
		ToolName:   chattool.ExecuteToolName,
	}
}

// runExecute runs the execute tool with input and returns its result.
// A zero id runs it without a tool call identity.
func runExecute(ctx context.Context, t *testing.T, conn workspacesdk.AgentConn, clock quartz.Clock, id chattool.ToolCallIdentity, input string) (fantasy.ToolResponse, chattool.ExecuteResult) {
	t.Helper()
	if id.ChatID != uuid.Nil {
		ctx = chattool.WithToolCallIdentity(ctx, id)
	}
	tool := chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
		Clock:            clock,
	})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: id.ToolCallID, Name: chattool.ExecuteToolName, Input: input})
	require.NoError(t, err)
	var result chattool.ExecuteResult
	if !resp.IsError {
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
	}
	return resp, result
}

func TestExecuteToolCall(t *testing.T) {
	t.Parallel()

	exitCode := func(code int) *int { return &code }
	wait := &workspacesdk.ProcessOutputOptions{Wait: true}

	// expectStart expects one start request that carries the tool call
	// headers of id and equals want.
	expectStart := func(t *testing.T, conn *agentconnmock.MockAgentConn, id chattool.ToolCallIdentity, want workspacesdk.StartProcessRequest, resp workspacesdk.StartProcessResponse, err error) *gomock.Call {
		return conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				tc, ok := workspacesdk.ToolCallFromContext(ctx)
				assert.True(t, ok, "start request must carry the tool call")
				assert.Equal(t, id.AgentToolCall(), tc)
				assert.Equal(t, want.Command, req.Command)
				assert.Equal(t, want.Background, req.Background)
				assert.Equal(t, want.TimeoutMs, req.TimeoutMs)
				return resp, err
			})
	}

	t.Run("ForegroundSendsTimeoutAndWaitsForTimedOut", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		id := newToolCallIdentity()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		gomock.InOrder(
			expectStart(t, conn, id, workspacesdk.StartProcessRequest{Command: "make test", TimeoutMs: (10 * time.Minute).Milliseconds()},
				workspacesdk.StartProcessResponse{ID: id.UUID()}, nil),
			// The agent's wait cap ends before the deadline.
			conn.EXPECT().ProcessOutput(gomock.Any(), id.UUID(), wait).
				Return(workspacesdk.ProcessOutputResponse{Running: true, Output: "part", DurationMs: 300_000}, nil),
			conn.EXPECT().ProcessOutput(gomock.Any(), id.UUID(), wait).
				Return(workspacesdk.ProcessOutputResponse{Running: true, TimedOut: true, Output: "partial", DurationMs: 600_000}, nil),
		)

		_, result := runExecute(ctx, t, conn, quartz.NewMock(t), id, `{"command":"make test","timeout":"10m"}`)
		assert.False(t, result.Success)
		assert.Equal(t, -1, result.ExitCode)
		assert.Equal(t, "command timed out after 10m0s", result.Error)
		assert.Equal(t, "partial", result.Output)
		assert.Equal(t, id.UUID(), result.BackgroundProcessID)
		assert.Equal(t, int64(600_000), result.WallDurationMs)
	})

	t.Run("ForegroundExited", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		id := newToolCallIdentity()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		gomock.InOrder(
			expectStart(t, conn, id, workspacesdk.StartProcessRequest{Command: "make test", TimeoutMs: 10_000},
				workspacesdk.StartProcessResponse{ID: id.UUID()}, nil),
			conn.EXPECT().ProcessOutput(gomock.Any(), id.UUID(), wait).
				Return(workspacesdk.ProcessOutputResponse{ExitCode: exitCode(2), Output: "FAIL", DurationMs: 1234}, nil),
		)

		_, result := runExecute(ctx, t, conn, quartz.NewMock(t), id, `{"command":"make test"}`)
		assert.False(t, result.Success)
		assert.Equal(t, 2, result.ExitCode)
		assert.Equal(t, "FAIL", result.Output)
		assert.Empty(t, result.Error)
		assert.Empty(t, result.BackgroundProcessID)
		assert.Equal(t, int64(1234), result.WallDurationMs)
	})

	t.Run("ForegroundCanceled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		id := newToolCallIdentity()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		gomock.InOrder(
			expectStart(t, conn, id, workspacesdk.StartProcessRequest{Command: "sleep 100", TimeoutMs: 60_000},
				workspacesdk.StartProcessResponse{ID: id.UUID()}, nil),
			conn.EXPECT().ProcessOutput(gomock.Any(), id.UUID(), wait).
				Return(workspacesdk.ProcessOutputResponse{Canceled: true, ExitCode: exitCode(-1), Output: "partial", DurationMs: 5000}, nil),
		)

		_, result := runExecute(ctx, t, conn, quartz.NewMock(t), id, `{"command":"sleep 100","timeout":"1m"}`)
		assert.False(t, result.Success)
		assert.Equal(t, "command canceled by the user after 5s", result.Error)
		assert.Equal(t, "partial", result.Output)
		assert.Equal(t, int64(5000), result.WallDurationMs)
	})

	t.Run("Background", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		id := newToolCallIdentity()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		expectStart(t, conn, id, workspacesdk.StartProcessRequest{Command: "make serve", Background: true},
			workspacesdk.StartProcessResponse{ID: id.UUID()}, nil)

		_, result := runExecute(ctx, t, conn, quartz.NewMock(t), id, `{"command":"make serve","run_in_background":true}`)
		assert.True(t, result.Success)
		assert.True(t, result.Backgrounded)
		assert.Equal(t, id.UUID(), result.BackgroundProcessID)
	})

	t.Run("NoIdentitySendsNoHeaders", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		gomock.InOrder(
			conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
					_, ok := workspacesdk.ToolCallFromContext(ctx)
					assert.False(t, ok)
					assert.Zero(t, req.TimeoutMs)
					return workspacesdk.StartProcessResponse{ID: "proc-1"}, nil
				}),
			conn.EXPECT().ProcessOutput(gomock.Any(), "proc-1", wait).
				Return(workspacesdk.ProcessOutputResponse{ExitCode: exitCode(0), Output: "ok"}, nil),
		)

		_, result := runExecute(ctx, t, conn, quartz.NewMock(t), chattool.ToolCallIdentity{}, `{"command":"true"}`)
		assert.True(t, result.Success)
		assert.Equal(t, "ok", result.Output)
	})

	t.Run("StartAnswers", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			err     error
			want    []string
			notWant []string
		}{
			{
				name: "Canceled",
				err:  &workspacesdk.ToolCallError{Code: workspacesdk.ToolCallErrorCanceled},
				want: []string{"command not run: the tool call was canceled before the workspace agent received it."},
			},
			{
				name:    "Unknown",
				err:     &workspacesdk.ToolCallError{Code: workspacesdk.ToolCallErrorUnknown},
				want:    []string{"outcome unknown: the workspace agent cannot tell whether this tool call ran", "the command may have run", "Check the workspace state before running it again."},
				notWant: []string{"process_output"},
			},
			{
				// An HTTP error answer is the recorded response of the
				// tool call's one run, so it keeps today's text.
				name: "HTTPErrorAnswer",
				err: codersdk.NewError(http.StatusBadRequest, codersdk.Response{
					Message: "Failed to start process.",
					Detail:  "no such directory",
				}),
				want:    []string{"start process: ", "Failed to start process.", "no such directory"},
				notWant: []string{"outcome unknown"},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitShort)
				id := newToolCallIdentity()
				conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
				// Times(1): an answer is never sent again.
				expectStart(t, conn, id, workspacesdk.StartProcessRequest{Command: "make test", TimeoutMs: 10_000},
					workspacesdk.StartProcessResponse{}, tt.err).Times(1)

				_, result := runExecute(ctx, t, conn, quartz.NewMock(t), id, `{"command":"make test"}`)
				assert.False(t, result.Success)
				for _, want := range tt.want {
					assert.Contains(t, result.Error, want)
				}
				for _, notWant := range tt.notWant {
					assert.NotContains(t, result.Error, notWant)
				}
			})
		}
	})

	t.Run("StartRetriedUntilAnswered", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		id := newToolCallIdentity()
		clock := quartz.NewMock(t)
		retryTrap := clock.Trap().NewTimer("chattool", "agent-retry")
		defer retryTrap.Close()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		transportErr := &url.Error{Op: "Post", URL: "http://agent/api/v0/processes/start", Err: xerrors.New("connection reset by peer")}
		gomock.InOrder(
			expectStart(t, conn, id, workspacesdk.StartProcessRequest{Command: "make serve", Background: true},
				workspacesdk.StartProcessResponse{}, transportErr),
			expectStart(t, conn, id, workspacesdk.StartProcessRequest{Command: "make serve", Background: true},
				workspacesdk.StartProcessResponse{ID: id.UUID()}, nil),
		)

		done := make(chan chattool.ExecuteResult, 1)
		go func() {
			_, result := runExecute(ctx, t, conn, clock, id, `{"command":"make serve","run_in_background":true}`)
			done <- result
		}()
		retryTrap.MustWait(ctx).MustRelease(ctx)
		clock.Advance(time.Second).MustWait(ctx)

		result := testutil.RequireReceive(ctx, t, done)
		assert.True(t, result.Success)
		assert.Equal(t, id.UUID(), result.BackgroundProcessID)
	})

	t.Run("StartNoAnswerIsUnknown", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		id := newToolCallIdentity()
		clock := quartz.NewMock(t)
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		sent := make(chan struct{}, 1)
		conn.EXPECT().StartProcess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
				sent <- struct{}{}
				<-ctx.Done()
				return workspacesdk.StartProcessResponse{}, &url.Error{Op: "Post", URL: "http://agent", Err: ctx.Err()}
			})

		done := make(chan chattool.ExecuteResult, 1)
		go func() {
			_, result := runExecute(ctx, t, conn, clock, id, `{"command":"make test"}`)
			done <- result
		}()
		testutil.RequireReceive(ctx, t, sent)
		clock.Advance(chattool.AgentAnswerTimeout).MustWait(ctx)

		result := testutil.RequireReceive(ctx, t, done)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "outcome unknown: the workspace agent did not answer within 1m0s")
		assert.Contains(t, result.Error, "the command may have run")
		assert.Contains(t, result.Error, "process ID "+id.UUID())
	})

	t.Run("OutputReadFailsIsUnknown", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		id := newToolCallIdentity()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		gomock.InOrder(
			expectStart(t, conn, id, workspacesdk.StartProcessRequest{Command: "make test", TimeoutMs: 10_000},
				workspacesdk.StartProcessResponse{ID: id.UUID()}, nil),
			conn.EXPECT().ProcessOutput(gomock.Any(), id.UUID(), wait).
				Return(workspacesdk.ProcessOutputResponse{}, codersdk.NewError(http.StatusNotFound, codersdk.Response{Message: "process not found"})),
		)

		_, result := runExecute(ctx, t, conn, quartz.NewMock(t), id, `{"command":"make test"}`)
		assert.False(t, result.Success)
		assert.Equal(t, -1, result.ExitCode)
		assert.Contains(t, result.Error, "outcome unknown: the command's result could not be read")
		assert.Contains(t, result.Error, "process not found")
		assert.NotContains(t, result.Error, "timed out")
		assert.Equal(t, id.UUID(), result.BackgroundProcessID)
	})

	t.Run("ConnErrors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			err         error
			wantUnknown bool
		}{
			{name: "NoWorkspace", err: chattool.ErrChatHasNoWorkspace},
			{name: "WorkspaceDeleted", err: chattool.ErrWorkspaceDeleted},
			{name: "NoRunningAgent", err: xerrors.Errorf("resolve: %w", chattool.ErrWorkspaceHasNoAgent)},
			{name: "DialFailed", err: xerrors.New("connection to the workspace agent timed out"), wantUnknown: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				id := newToolCallIdentity()
				tool := chattool.Execute(chattool.ExecuteOptions{
					GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return nil, tt.err },
				})
				ctx := chattool.WithToolCallIdentity(testutil.Context(t, testutil.WaitShort), id)
				resp, err := tool.Run(ctx, fantasy.ToolCall{ID: id.ToolCallID, Name: chattool.ExecuteToolName, Input: `{"command":"true"}`})
				require.NoError(t, err)
				if !tt.wantUnknown {
					assert.True(t, resp.IsError)
					assert.Equal(t, tt.err.Error(), resp.Content)
					return
				}
				var result chattool.ExecuteResult
				require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
				assert.Contains(t, result.Error, "outcome unknown: the workspace agent could not be reached")
				assert.Contains(t, result.Error, "process ID "+id.UUID())
			})
		}
	})
}
