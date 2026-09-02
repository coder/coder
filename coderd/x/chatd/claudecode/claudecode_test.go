package claudecode_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/coderd/x/chatd/claudecode"
	"github.com/coder/coder/v2/coderd/x/chatd/claudecode/claudecodetest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func testLogger(t *testing.T) slog.Logger {
	return slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
}

func sendUpdate(ctx context.Context, t *testing.T, conn *acp.AgentSideConnection, sessionID acp.SessionId, update acp.SessionUpdate) {
	t.Helper()
	require.NoError(t, conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sessionID,
		Update:    update,
	}))
}

type publishedPart struct {
	role codersdk.ChatMessageRole
	part codersdk.ChatMessagePart
}

type partRecorder struct {
	mu    sync.Mutex
	parts []publishedPart
}

func (r *partRecorder) publish(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.parts = append(r.parts, publishedPart{role: role, part: part})
}

func (r *partRecorder) snapshot() []publishedPart {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]publishedPart{}, r.parts...)
}

func TestRunTurnNewSession(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &claudecodetest.FakeAgent{}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{Content: acp.TextBlock("thinking...")},
		})
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("Hello ")},
		})
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("world")},
		})
		thought := 7
		return acp.PromptResponse{
			StopReason: acp.StopReasonEndTurn,
			Usage:      &acp.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, ThoughtTokens: &thought},
		}, nil
	}

	recorder := &partRecorder{}
	outcome, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "hi",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)

	require.Equal(t, "session-new", outcome.SessionID)
	require.False(t, outcome.Resumed)
	require.Equal(t, acp.StopReasonEndTurn, outcome.StopReason)
	require.NotNil(t, outcome.Usage)
	require.Equal(t, 15, outcome.Usage.TotalTokens)

	require.Len(t, outcome.Content, 2)

	require.Len(t, agent.NewSessions(), 1)
	require.Equal(t, "/home/coder", agent.NewSessions()[0].Cwd)
	require.Len(t, agent.Prompts(), 1)
	require.Len(t, agent.Prompts()[0].Prompt, 1)
	require.Equal(t, "hi", agent.Prompts()[0].Prompt[0].Text.Text)

	parts := recorder.snapshot()
	require.Len(t, parts, 3)
	assert.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[0].part.Type)
	assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[1].part.Type)
	assert.Equal(t, "Hello ", parts[1].part.Text)
	assert.Equal(t, "world", parts[2].part.Text)
}

func TestRunTurnResumeSession(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &claudecodetest.FakeAgent{
		Capabilities: acp.AgentCapabilities{
			SessionCapabilities: acp.SessionCapabilities{Resume: &acp.SessionResumeCapabilities{}},
		},
	}

	outcome, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		SessionID:  "session-prior",
		SessionCwd: "/prior/cwd",
		Cwd:        "/home/coder",
		PromptText: "continue",
		Logger:     testLogger(t),
	})
	require.NoError(t, err)

	require.True(t, outcome.Resumed)
	require.Equal(t, "session-prior", outcome.SessionID)
	require.Len(t, agent.ResumeSessions(), 1)
	require.Equal(t, acp.SessionId("session-prior"), agent.ResumeSessions()[0].SessionId)
	require.Equal(t, "/prior/cwd", agent.ResumeSessions()[0].Cwd)
	require.Empty(t, agent.NewSessions())
}

func TestRunTurnResumeFallsBackToLoad(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &claudecodetest.FakeAgent{
		Capabilities: acp.AgentCapabilities{
			LoadSession: true,
			SessionCapabilities: acp.SessionCapabilities{
				Resume: &acp.SessionResumeCapabilities{},
			},
		},
	}
	agent.OnResumeSession = func(acp.ResumeSessionRequest) error {
		return xerrors.New("resume unsupported for this session")
	}
	// session/load replays history; the client must not re-publish it.
	agent.OnLoadSession = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.LoadSessionRequest) error {
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("replayed history")},
		})
		return nil
	}

	recorder := &partRecorder{}
	outcome, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		SessionID:  "session-prior",
		Cwd:        "/home/coder",
		PromptText: "continue",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)

	require.True(t, outcome.Resumed)
	require.Len(t, agent.LoadSessions(), 1)
	require.Empty(t, agent.NewSessions())
	require.Empty(t, recorder.snapshot())
	require.Empty(t, outcome.Content)
}

func TestRunTurnReseedsWhenSessionGone(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &claudecodetest.FakeAgent{}

	reseed := claudecode.BuildReseedContext([]claudecode.ReseedTurn{
		{Role: "User", Text: "earlier question"},
		{Role: "Assistant", Text: "earlier answer"},
	})
	outcome, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		SessionID:     "session-prior",
		Cwd:           "/home/coder",
		PromptText:    "follow-up",
		ReseedContext: reseed,
		Logger:        testLogger(t),
	})
	require.NoError(t, err)

	require.False(t, outcome.Resumed)
	require.Equal(t, "session-new", outcome.SessionID)
	require.Len(t, agent.Prompts(), 1)
	prompt := agent.Prompts()[0].Prompt[0].Text.Text
	require.Contains(t, prompt, "earlier question")
	require.Contains(t, prompt, "earlier answer")
	require.True(t, strings.HasSuffix(prompt, "follow-up"))
}

func TestRunTurnToolCallMapping(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &claudecodetest.FakeAgent{}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("Let me check.")},
		})
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			ToolCall: &acp.SessionUpdateToolCall{
				ToolCallId: "tool-1",
				Title:      "Read file",
				Kind:       acp.ToolKindRead,
				Status:     acp.ToolCallStatusInProgress,
				RawInput:   map[string]any{"path": "main.go"},
			},
		})
		completed := acp.ToolCallStatusCompleted
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{
				ToolCallId: "tool-1",
				Status:     &completed,
				Content: []acp.ToolCallContent{
					acp.ToolContent(acp.TextBlock("package main")),
				},
			},
		})
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("Done.")},
		})
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}

	recorder := &partRecorder{}
	outcome, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "read main.go",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)

	require.Len(t, outcome.Content, 4)

	parts := recorder.snapshot()
	require.Len(t, parts, 4)
	assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].part.Type)
	assert.Equal(t, codersdk.ChatMessagePartTypeToolCall, parts[1].part.Type)
	assert.Equal(t, "tool-1", parts[1].part.ToolCallID)
	assert.Equal(t, "Read file", parts[1].part.ToolName)
	assert.JSONEq(t, `{"path":"main.go"}`, string(parts[1].part.Args))
	assert.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[2].part.Type)
	assert.Equal(t, codersdk.ChatMessageRoleTool, parts[2].role)
	assert.False(t, parts[2].part.IsError)
	// Non-JSON text output is wrapped the same way the persisted
	// pipeline wraps it, so preview and durable parts match.
	assert.JSONEq(t, `{"output":"package main"}`, string(parts[2].part.Result))
	assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[3].part.Type)
}

func TestRunTurnCancelDuringSetup(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	// A hung setup RPC (session/resume here) must abort when the turn
	// context is canceled instead of wedging the runner forever.
	hung := make(chan struct{})
	agent := &claudecodetest.FakeAgent{
		Capabilities: acp.AgentCapabilities{
			SessionCapabilities: acp.SessionCapabilities{
				Resume: &acp.SessionResumeCapabilities{},
			},
		},
	}
	agent.OnResumeSession = func(acp.ResumeSessionRequest) error {
		close(hung)
		<-ctx.Done()
		return ctx.Err()
	}

	turnCtx, cancel := context.WithCancel(ctx)
	go func() {
		<-hung
		cancel()
	}()

	_, err := claudecode.RunTurn(turnCtx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:        "/home/coder",
		SessionID:  "sess-hang",
		PromptText: "hello",
		Publish:    func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {},
		Logger:     testLogger(t),
	})
	// The SDK surfaces the abort as a JSON-RPC "Request canceled"
	// error rather than context.Canceled; what matters is that the
	// turn failed promptly instead of racing a fallback session or
	// prompting.
	require.Error(t, err)
	require.ErrorContains(t, err, "resume session")
	require.Empty(t, agent.NewSessions())
	require.Empty(t, agent.Prompts())
}

func TestRunTurnToolCallInputUpdate(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &claudecodetest.FakeAgent{}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			ToolCall: &acp.SessionUpdateToolCall{
				ToolCallId: "tool-1",
				Title:      "Read file",
				Kind:       acp.ToolKindRead,
				Status:     acp.ToolCallStatusInProgress,
			},
		})
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{
				ToolCallId: "tool-1",
				RawInput:   map[string]any{"path": "main.go"},
			},
		})
		completed := acp.ToolCallStatusCompleted
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{
				ToolCallId: "tool-1",
				Status:     &completed,
				Content: []acp.ToolCallContent{
					acp.ToolContent(acp.TextBlock("package main")),
				},
			},
		})
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}

	recorder := &partRecorder{}
	outcome, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "read main.go",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)

	require.Len(t, outcome.Content, 2)
	call, ok := outcome.Content[0].(fantasy.ToolCallContent)
	require.True(t, ok)
	assert.Equal(t, "tool-1", call.ToolCallID)
	assert.JSONEq(t, `{"path":"main.go"}`, call.Input)

	parts := recorder.snapshot()
	require.Len(t, parts, 3)
	assert.Equal(t, codersdk.ChatMessagePartTypeToolCall, parts[0].part.Type)
	assert.Equal(t, codersdk.ChatMessagePartTypeToolCall, parts[1].part.Type)
	assert.Equal(t, "tool-1", parts[1].part.ToolCallID)
	assert.JSONEq(t, `{"path":"main.go"}`, string(parts[1].part.Args))
	assert.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[2].part.Type)
}

func TestRunTurnFailedToolCall(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &claudecodetest.FakeAgent{}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			ToolCall: &acp.SessionUpdateToolCall{
				ToolCallId: "tool-1",
				Kind:       acp.ToolKindExecute,
				Status:     acp.ToolCallStatusInProgress,
			},
		})
		failed := acp.ToolCallStatusFailed
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{
				ToolCallId: "tool-1",
				Status:     &failed,
				Content: []acp.ToolCallContent{
					acp.ToolContent(acp.TextBlock("command not found")),
				},
			},
		})
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}

	recorder := &partRecorder{}
	_, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "run it",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)

	parts := recorder.snapshot()
	require.Len(t, parts, 2)
	assert.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[1].part.Type)
	assert.True(t, parts[1].part.IsError)
}

func TestRunTurnCancellation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	promptStarted := make(chan struct{})
	agent := &claudecodetest.FakeAgent{}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("partial")},
		})
		close(promptStarted)
		// Per spec, session/cancel resolves the in-flight prompt with
		// stopReason=canceled.
		<-ctx.Done()
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}

	turnCtx, cancelTurn := context.WithCancel(ctx)
	go func() {
		select {
		case <-promptStarted:
		case <-ctx.Done():
		}
		cancelTurn()
	}()

	recorder := &partRecorder{}
	outcome, err := claudecode.RunTurn(turnCtx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "long task",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)
	require.Equal(t, acp.StopReasonCancelled, outcome.StopReason)
	require.NotEmpty(t, recorder.snapshot())

	require.Len(t, agent.Cancels(), 1)
}

func TestRunTurnPermissionAutoDeny(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var denied acp.RequestPermissionResponse
	agent := &claudecodetest.FakeAgent{}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		resp, err := conn.RequestPermission(ctx, acp.RequestPermissionRequest{
			SessionId: params.SessionId,
			ToolCall:  acp.ToolCallUpdate{ToolCallId: "tool-1"},
			Options: []acp.PermissionOption{
				{OptionId: "allow", Kind: acp.PermissionOptionKindAllowOnce},
				{OptionId: "reject", Kind: acp.PermissionOptionKindRejectOnce},
			},
		})
		if err != nil {
			return acp.PromptResponse{}, err
		}
		denied = resp
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}

	recorder := &partRecorder{}
	_, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "do something requiring permission",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)

	require.NotNil(t, denied.Outcome.Selected)
	require.Equal(t, acp.PermissionOptionId("reject"), denied.Outcome.Selected.OptionId)

	parts := recorder.snapshot()
	require.NotEmpty(t, parts)
	require.Contains(t, parts[0].part.Text, "declined automatically")
}

func TestRunTurnSetsPermissionMode(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &claudecodetest.FakeAgent{}
	_, err := claudecode.RunTurn(ctx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:            "/home/coder",
		PromptText:     "hi",
		PermissionMode: "acceptEdits",
		Logger:         testLogger(t),
	})
	require.NoError(t, err)

	require.Len(t, agent.Modes(), 1)
	require.Equal(t, acp.SessionModeId("acceptEdits"), agent.Modes()[0].ModeId)
}

func TestBuildReseedContext(t *testing.T) {
	t.Parallel()

	require.Empty(t, claudecode.BuildReseedContext(nil))

	small := claudecode.BuildReseedContext([]claudecode.ReseedTurn{
		{Role: "User", Text: "question"},
		{Role: "Assistant", Text: "answer"},
	})
	require.Contains(t, small, "User: question")
	require.Contains(t, small, "Assistant: answer")

	turns := make([]claudecode.ReseedTurn, 0, 100)
	for range 100 {
		turns = append(turns, claudecode.ReseedTurn{Role: "User", Text: strings.Repeat("x", 1024)})
	}
	turns = append(turns, claudecode.ReseedTurn{Role: "Assistant", Text: "most recent"})
	bounded := claudecode.BuildReseedContext(turns)
	require.Less(t, len(bounded), 40*1024)
	require.Contains(t, bounded, "most recent")
}

func TestRunTurnCancelTimeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	promptStarted := make(chan struct{})
	release := make(chan struct{})
	agent := &claudecodetest.FakeAgent{}
	agent.OnPrompt = func(context.Context, *acp.AgentSideConnection, acp.PromptRequest) (acp.PromptResponse, error) {
		close(promptStarted)
		// Never resolve until released: simulates an adapter that
		// ignores session/cancel.
		<-release
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}
	t.Cleanup(func() { close(release) })

	turnCtx, cancelTurn := context.WithCancel(ctx)
	go func() {
		select {
		case <-promptStarted:
		case <-ctx.Done():
		}
		cancelTurn()
	}()

	start := time.Now()
	_, err := claudecode.RunTurn(turnCtx, &claudecodetest.PipeTransport{Agent: agent}, claudecode.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "hang",
		Logger:     testLogger(t),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), testutil.WaitLong)
}
