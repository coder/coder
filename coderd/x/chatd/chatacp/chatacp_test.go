package chatacp_test

import (
	"context"
	"encoding/json"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp/chatacptest"
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

	agent := &chatacptest.FakeAgent{}
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
	outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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

	agent := &chatacptest.FakeAgent{
		Capabilities: acp.AgentCapabilities{
			SessionCapabilities: acp.SessionCapabilities{Resume: &acp.SessionResumeCapabilities{}},
		},
	}

	outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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

	agent := &chatacptest.FakeAgent{
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
	outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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

func TestRunTurnAvailableCommands(t *testing.T) {
	t.Parallel()

	unstructured := func(hint string) *acp.AvailableCommandInput {
		return &acp.AvailableCommandInput{Unstructured: &acp.UnstructuredCommandInput{Hint: hint}}
	}
	tests := []struct {
		name    string
		updates [][]acp.AvailableCommand
		want    []codersdk.ChatRuntimeCommand
	}{
		{
			name:    "NoUpdate",
			updates: nil,
			want:    nil,
		},
		{
			name: "SingleUpdate",
			updates: [][]acp.AvailableCommand{{
				{Name: "review", Description: "Review the diff", Input: unstructured("pr number")},
				{Name: "init", Description: "Create a guide"},
				{Name: "  ", Description: "dropped: blank name"},
			}},
			want: []codersdk.ChatRuntimeCommand{
				{Name: "review", Description: "Review the diff", InputHint: "pr number"},
				{Name: "init", Description: "Create a guide"},
			},
		},
		{
			name: "LatestUpdateWins",
			updates: [][]acp.AvailableCommand{
				{{Name: "review", Description: "Review the diff"}},
				{{Name: "compact", Description: "Compact history"}},
			},
			want: []codersdk.ChatRuntimeCommand{{Name: "compact", Description: "Compact history"}},
		},
		{
			name: "EmptyUpdateClearsList",
			updates: [][]acp.AvailableCommand{
				{{Name: "review", Description: "Review the diff"}},
				{},
			},
			want: []codersdk.ChatRuntimeCommand{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			agent := &chatacptest.FakeAgent{}
			agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
				for _, commands := range tc.updates {
					sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
						AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{AvailableCommands: commands},
					})
				}
				sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
					AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("done")},
				})
				return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
			}

			recorder := &partRecorder{}
			outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
				Cwd:        "/home/coder",
				PromptText: "hi",
				Publish:    recorder.publish,
				Logger:     testLogger(t),
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, outcome.AvailableCommands)
			// Command updates are session metadata, not transcript.
			require.Len(t, recorder.snapshot(), 1)
			require.Len(t, outcome.Content, 1)
		})
	}
}

func TestRunTurnAvailableCommandsDuringLoad(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &chatacptest.FakeAgent{
		Capabilities: acp.AgentCapabilities{LoadSession: true},
	}
	// Replayed history is suppressed, but the command list the agent
	// advertises alongside it describes the live session.
	agent.OnLoadSession = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.LoadSessionRequest) error {
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("replayed history")},
		})
		sendUpdate(ctx, t, conn, params.SessionId, acp.SessionUpdate{
			AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
				AvailableCommands: []acp.AvailableCommand{{Name: "review", Description: "Review the diff"}},
			},
		})
		return nil
	}

	recorder := &partRecorder{}
	outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
		SessionID:  "session-prior",
		Cwd:        "/home/coder",
		PromptText: "continue",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)
	require.True(t, outcome.Resumed)
	require.Empty(t, recorder.snapshot())
	require.Equal(t, []codersdk.ChatRuntimeCommand{{Name: "review", Description: "Review the diff"}}, outcome.AvailableCommands)
}

func TestRuntimeStateRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  func(t *testing.T) []byte
		want chatacp.RuntimeState
	}{
		{
			name: "Absent",
			raw:  func(*testing.T) []byte { return nil },
			want: chatacp.RuntimeState{},
		},
		{
			name: "Malformed",
			raw:  func(*testing.T) []byte { return []byte("{not json") },
			want: chatacp.RuntimeState{},
		},
		{
			name: "WithoutCommands",
			raw: func(t *testing.T) []byte {
				raw, err := json.Marshal(chatacp.RuntimeState{SessionID: "s1", Cwd: "/home/coder"})
				require.NoError(t, err)
				require.NotContains(t, string(raw), "available_commands")
				return raw
			},
			want: chatacp.RuntimeState{SessionID: "s1", Cwd: "/home/coder"},
		},
		{
			name: "WithCommands",
			raw: func(t *testing.T) []byte {
				raw, err := json.Marshal(chatacp.RuntimeState{
					SessionID: "s1",
					AvailableCommands: []codersdk.ChatRuntimeCommand{
						{Name: "review", Description: "Review the diff", InputHint: "pr number"},
						{Name: "init"},
					},
				})
				require.NoError(t, err)
				require.Contains(t, string(raw), `"available_commands":[{"name":"review","description":"Review the diff","input_hint":"pr number"},{"name":"init","description":""}]`)
				return raw
			},
			want: chatacp.RuntimeState{
				SessionID: "s1",
				AvailableCommands: []codersdk.ChatRuntimeCommand{
					{Name: "review", Description: "Review the diff", InputHint: "pr number"},
					{Name: "init"},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := chatacp.ParseRuntimeState(tc.raw(t))
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRuntimeStateAdvance(t *testing.T) {
	t.Parallel()

	const turnCwd = "/home/coder/turn"
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	intPtr := func(v int) *int { return &v }
	priorCommands := []codersdk.ChatRuntimeCommand{{Name: "review", Description: "Review the diff"}}
	turnCommands := []codersdk.ChatRuntimeCommand{{Name: "compact", Description: "Compact history"}}
	prior := chatacp.RuntimeState{
		SessionID:         "s1",
		Cwd:               "/home/coder/prior",
		Usage:             &chatacp.UsageTotals{InputTokens: 100, OutputTokens: 40, TotalTokens: 140, ReasoningTokens: 10},
		AvailableCommands: priorCommands,
	}

	tests := []struct {
		name      string
		prior     chatacp.RuntimeState
		outcome   chatacp.TurnOutcome
		wantState chatacp.RuntimeState
		wantUsage fantasy.Usage
	}{
		{
			name:    "FreshSessionUsesRawCounts",
			outcome: chatacp.TurnOutcome{SessionID: "s1", Usage: &acp.Usage{InputTokens: 100, OutputTokens: 40, TotalTokens: 140}},
			wantState: chatacp.RuntimeState{
				SessionID: "s1",
				Cwd:       turnCwd,
				Usage:     &chatacp.UsageTotals{InputTokens: 100, OutputTokens: 40, TotalTokens: 140},
				UpdatedAt: now,
			},
			wantUsage: fantasy.Usage{InputTokens: 100, OutputTokens: 40, TotalTokens: 140},
		},
		{
			name:  "ResumedSubtractsPriorTotals",
			prior: prior,
			outcome: chatacp.TurnOutcome{SessionID: "s1", Resumed: true, Usage: &acp.Usage{
				InputTokens: 250, OutputTokens: 90, TotalTokens: 340, ThoughtTokens: intPtr(30), CachedReadTokens: intPtr(5),
			}},
			wantState: chatacp.RuntimeState{
				SessionID:         "s1",
				Cwd:               "/home/coder/prior",
				Usage:             &chatacp.UsageTotals{InputTokens: 250, OutputTokens: 90, TotalTokens: 340, ReasoningTokens: 30, CacheReadTokens: 5},
				AvailableCommands: priorCommands,
				UpdatedAt:         now,
			},
			wantUsage: fantasy.Usage{InputTokens: 150, OutputTokens: 50, TotalTokens: 200, ReasoningTokens: 20, CacheReadTokens: 5},
		},
		{
			name:    "ResumedWithoutUsageCarriesPriorForward",
			prior:   prior,
			outcome: chatacp.TurnOutcome{SessionID: "s1", Resumed: true},
			wantState: chatacp.RuntimeState{
				SessionID:         "s1",
				Cwd:               "/home/coder/prior",
				Usage:             prior.Usage,
				AvailableCommands: priorCommands,
				UpdatedAt:         now,
			},
		},
		{
			name:    "CounterRestartFallsBackToRawCounts",
			prior:   prior,
			outcome: chatacp.TurnOutcome{SessionID: "s1", Resumed: true, Usage: &acp.Usage{InputTokens: 20, OutputTokens: 5, TotalTokens: 25}},
			wantState: chatacp.RuntimeState{
				SessionID:         "s1",
				Cwd:               "/home/coder/prior",
				Usage:             &chatacp.UsageTotals{InputTokens: 20, OutputTokens: 5, TotalTokens: 25},
				AvailableCommands: priorCommands,
				UpdatedAt:         now,
			},
			wantUsage: fantasy.Usage{InputTokens: 20, OutputTokens: 5, TotalTokens: 25},
		},
		{
			name:    "NewSessionDropsPrior",
			prior:   prior,
			outcome: chatacp.TurnOutcome{SessionID: "s2", Usage: &acp.Usage{TotalTokens: 50}},
			wantState: chatacp.RuntimeState{
				SessionID: "s2",
				Cwd:       turnCwd,
				Usage:     &chatacp.UsageTotals{TotalTokens: 50},
				UpdatedAt: now,
			},
			wantUsage: fantasy.Usage{TotalTokens: 50},
		},
		{
			name:      "NewSessionWithoutUsageDropsPriorTotals",
			prior:     prior,
			outcome:   chatacp.TurnOutcome{SessionID: "s2"},
			wantState: chatacp.RuntimeState{SessionID: "s2", Cwd: turnCwd, UpdatedAt: now},
		},
		{
			name:    "TurnCommandListWins",
			prior:   prior,
			outcome: chatacp.TurnOutcome{SessionID: "s1", Resumed: true, AvailableCommands: turnCommands},
			wantState: chatacp.RuntimeState{
				SessionID:         "s1",
				Cwd:               "/home/coder/prior",
				Usage:             prior.Usage,
				AvailableCommands: turnCommands,
				UpdatedAt:         now,
			},
		},
		{
			name:    "EmptyTurnCommandListClears",
			prior:   prior,
			outcome: chatacp.TurnOutcome{SessionID: "s1", Resumed: true, AvailableCommands: []codersdk.ChatRuntimeCommand{}},
			wantState: chatacp.RuntimeState{
				SessionID:         "s1",
				Cwd:               "/home/coder/prior",
				Usage:             prior.Usage,
				AvailableCommands: []codersdk.ChatRuntimeCommand{},
				UpdatedAt:         now,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotState, gotUsage := tc.prior.Advance(tc.outcome, turnCwd, now)
			require.Equal(t, tc.wantState, gotState)
			require.Equal(t, tc.wantUsage, gotUsage)
		})
	}
}

func TestRunTurnReseedsWhenSessionGone(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &chatacptest.FakeAgent{}

	reseed := chatacp.BuildReseedContext([]chatacp.ReseedTurn{
		{Role: "User", Text: "earlier question"},
		{Role: "Assistant", Text: "earlier answer"},
	})
	outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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

	agent := &chatacptest.FakeAgent{}
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
	outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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
	agent := &chatacptest.FakeAgent{
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

	_, err := chatacp.RunTurn(turnCtx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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

	agent := &chatacptest.FakeAgent{}
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
	outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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

	agent := &chatacptest.FakeAgent{}
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
	_, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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
	agent := &chatacptest.FakeAgent{}
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
	outcome, err := chatacp.RunTurn(turnCtx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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
	agent := &chatacptest.FakeAgent{}
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
	_, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "do something requiring permission",
		AgentName:  "Test Agent",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)

	require.NotNil(t, denied.Outcome.Selected)
	require.Equal(t, acp.PermissionOptionId("reject"), denied.Outcome.Selected.OptionId)

	parts := recorder.snapshot()
	require.NotEmpty(t, parts)
	require.Contains(t, parts[0].part.Text, "Test Agent requested a permission")
	require.Contains(t, parts[0].part.Text, "declined automatically")
}

func TestRunTurnSetsPermissionMode(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &chatacptest.FakeAgent{}
	_, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
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

	require.Empty(t, chatacp.BuildReseedContext(nil))

	small := chatacp.BuildReseedContext([]chatacp.ReseedTurn{
		{Role: "User", Text: "question"},
		{Role: "Assistant", Text: "answer"},
	})
	require.Contains(t, small, "User: question")
	require.Contains(t, small, "Assistant: answer")

	turns := make([]chatacp.ReseedTurn, 0, 100)
	for range 100 {
		turns = append(turns, chatacp.ReseedTurn{Role: "User", Text: strings.Repeat("x", 1024)})
	}
	turns = append(turns, chatacp.ReseedTurn{Role: "Assistant", Text: "most recent"})
	bounded := chatacp.BuildReseedContext(turns)
	require.Less(t, len(bounded), 40*1024)
	require.Contains(t, bounded, "most recent")
}

func TestRunTurnCancelTimeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	promptStarted := make(chan struct{})
	release := make(chan struct{})
	agent := &chatacptest.FakeAgent{}
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
	_, err := chatacp.RunTurn(turnCtx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "hang",
		Logger:     testLogger(t),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), testutil.WaitLong)
}
