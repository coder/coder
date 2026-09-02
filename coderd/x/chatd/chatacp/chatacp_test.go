package chatacp_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp/chatacptest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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

// agentScript is one fake agent's behavior for a turn: what it
// advertises, whether it accepts session/resume, the updates it streams
// during session/load and session/prompt, and the usage it reports.
type agentScript struct {
	caps          acp.AgentCapabilities
	resumeErr     error
	loadUpdates   []acp.SessionUpdate
	promptUpdates []acp.SessionUpdate
	usage         *acp.Usage
}

// runScriptedTurn runs one turn against a scripted fake agent and
// returns the outcome, the agent, and the streamed preview parts.
func runScriptedTurn(t *testing.T, script agentScript, input chatacp.TurnInput) (chatacp.TurnOutcome, *chatacptest.FakeAgent, []publishedPart) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &chatacptest.FakeAgent{Capabilities: script.caps}
	if script.resumeErr != nil {
		agent.OnResumeSession = func(acp.ResumeSessionRequest) error { return script.resumeErr }
	}
	agent.OnLoadSession = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.LoadSessionRequest) error {
		for _, update := range script.loadUpdates {
			sendUpdate(ctx, t, conn, params.SessionId, update)
		}
		return nil
	}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		for _, update := range script.promptUpdates {
			sendUpdate(ctx, t, conn, params.SessionId, update)
		}
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn, Usage: script.usage}, nil
	}

	recorder := &partRecorder{}
	input.Publish = recorder.publish
	input.Logger = testLogger(t)
	outcome, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, input)
	require.NoError(t, err)
	return outcome, agent, recorder.snapshot()
}

// requireOutcome compares whole turn outcomes. Tool failures carry
// xerrors values whose captured frames defeat plain equality, so errors
// compare by message.
func requireOutcome(t *testing.T, want, got chatacp.TurnOutcome) {
	t.Helper()
	require.Empty(t, cmp.Diff(want, got, cmp.Comparer(func(a, b error) bool {
		return (a == nil) == (b == nil) && (a == nil || a.Error() == b.Error())
	})))
}

// sessionRequest is the session a setup RPC named and the cwd it sent.
type sessionRequest struct {
	session acp.SessionId
	cwd     string
}

// agentCalls is the fake agent's request log as one comparable value.
type agentCalls struct {
	newCwds []string
	resumes []sessionRequest
	loads   []sessionRequest
	modes   []acp.SessionModeId
	prompts []string
}

func recordedCalls(agent *chatacptest.FakeAgent) agentCalls {
	var calls agentCalls
	for _, req := range agent.NewSessions() {
		calls.newCwds = append(calls.newCwds, req.Cwd)
	}
	for _, req := range agent.ResumeSessions() {
		calls.resumes = append(calls.resumes, sessionRequest{req.SessionId, req.Cwd})
	}
	for _, req := range agent.LoadSessions() {
		calls.loads = append(calls.loads, sessionRequest{req.SessionId, req.Cwd})
	}
	for _, req := range agent.Modes() {
		calls.modes = append(calls.modes, req.ModeId)
	}
	for _, req := range agent.Prompts() {
		calls.prompts = append(calls.prompts, req.Prompt[0].Text.Text)
	}
	return calls
}

func textChunk(text string) acp.SessionUpdate {
	return acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(text)}}
}

func thoughtChunk(text string) acp.SessionUpdate {
	return acp.SessionUpdate{AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{Content: acp.TextBlock(text)}}
}

func commandsUpdate(commands ...acp.AvailableCommand) acp.SessionUpdate {
	if commands == nil {
		commands = []acp.AvailableCommand{}
	}
	return acp.SessionUpdate{AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{AvailableCommands: commands}}
}

func toolCall(id, title string, kind acp.ToolKind, rawInput any) acp.SessionUpdate {
	return acp.SessionUpdate{ToolCall: &acp.SessionUpdateToolCall{
		ToolCallId: acp.ToolCallId(id),
		Title:      title,
		Kind:       kind,
		Status:     acp.ToolCallStatusInProgress,
		RawInput:   rawInput,
	}}
}

func toolCallUpdate(id string, status *acp.ToolCallStatus, rawInput any, output string) acp.SessionUpdate {
	update := &acp.SessionToolCallUpdate{ToolCallId: acp.ToolCallId(id), Status: status, RawInput: rawInput}
	if output != "" {
		update.Content = []acp.ToolCallContent{acp.ToolContent(acp.TextBlock(output))}
	}
	return acp.SessionUpdate{ToolCallUpdate: update}
}

// part is the preview part the persisted pipeline renders for content,
// so preview and durable parts are asserted to match.
func part(role codersdk.ChatMessageRole, content fantasy.Content) publishedPart {
	return publishedPart{role: role, part: chatprompt.PartFromContent(content)}
}

func TestRunTurnSessionEstablishment(t *testing.T) {
	t.Parallel()

	const (
		prior = "session-prior"
		cwd   = "/home/coder"
	)
	resumable := acp.AgentCapabilities{SessionCapabilities: acp.SessionCapabilities{Resume: &acp.SessionResumeCapabilities{}}}
	loadable := acp.AgentCapabilities{LoadSession: true}
	review := acp.AvailableCommand{Name: "review", Description: "Review the diff"}
	reseed := chatacp.BuildReseedContext([]chatacp.ReseedTurn{
		{Role: "User", Text: "earlier question"},
		{Role: "Assistant", Text: "earlier answer"},
	})
	// session/load replays history as session updates; the chat already
	// has it persisted, so nothing is republished.
	replay := []acp.SessionUpdate{textChunk("replayed history"), commandsUpdate(review)}

	tests := []struct {
		name      string
		script    agentScript
		input     chatacp.TurnInput
		want      chatacp.TurnOutcome
		wantCalls agentCalls
	}{
		{
			name:      "NewSession",
			input:     chatacp.TurnInput{Cwd: cwd, PromptText: "hi"},
			want:      chatacp.TurnOutcome{SessionID: "session-new"},
			wantCalls: agentCalls{newCwds: []string{cwd}, prompts: []string{"hi"}},
		},
		{
			name:      "SetsPermissionMode",
			input:     chatacp.TurnInput{Cwd: cwd, PromptText: "hi", PermissionMode: "acceptEdits"},
			want:      chatacp.TurnOutcome{SessionID: "session-new"},
			wantCalls: agentCalls{newCwds: []string{cwd}, modes: []acp.SessionModeId{"acceptEdits"}, prompts: []string{"hi"}},
		},
		{
			// A resumed session keeps the cwd it was created with and
			// needs no reseed.
			name:      "ResumesWithSessionCwd",
			script:    agentScript{caps: resumable},
			input:     chatacp.TurnInput{SessionID: prior, SessionCwd: "/prior/cwd", Cwd: cwd, PromptText: "continue", ReseedContext: reseed},
			want:      chatacp.TurnOutcome{SessionID: prior, Resumed: true},
			wantCalls: agentCalls{resumes: []sessionRequest{{prior, "/prior/cwd"}}, prompts: []string{"continue"}},
		},
		{
			name: "ResumeFallsBackToLoad",
			script: agentScript{
				caps:        acp.AgentCapabilities{LoadSession: true, SessionCapabilities: resumable.SessionCapabilities},
				resumeErr:   xerrors.New("resume unsupported for this session"),
				loadUpdates: replay[:1],
			},
			input: chatacp.TurnInput{SessionID: prior, Cwd: cwd, PromptText: "continue"},
			want:  chatacp.TurnOutcome{SessionID: prior, Resumed: true},
			wantCalls: agentCalls{
				resumes: []sessionRequest{{prior, cwd}},
				loads:   []sessionRequest{{prior, cwd}},
				prompts: []string{"continue"},
			},
		},
		{
			// The command list advertised alongside a replay describes
			// the live session and is kept.
			name:      "LoadRefreshesCommands",
			script:    agentScript{caps: loadable, loadUpdates: replay},
			input:     chatacp.TurnInput{SessionID: prior, Cwd: cwd, PromptText: "continue"},
			want:      chatacp.TurnOutcome{SessionID: prior, Resumed: true, AvailableCommands: []codersdk.ChatRuntimeCommand{{Name: "review", Description: "Review the diff"}}},
			wantCalls: agentCalls{loads: []sessionRequest{{prior, cwd}}, prompts: []string{"continue"}},
		},
		{
			name:      "ReseedsWhenSessionGone",
			input:     chatacp.TurnInput{SessionID: prior, Cwd: cwd, PromptText: "follow-up", ReseedContext: reseed},
			want:      chatacp.TurnOutcome{SessionID: "session-new"},
			wantCalls: agentCalls{newCwds: []string{cwd}, prompts: []string{reseed + "\n\nfollow-up"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			outcome, agent, parts := runScriptedTurn(t, tc.script, tc.input)
			requireOutcome(t, tc.want, outcome)
			require.Equal(t, tc.wantCalls, recordedCalls(agent))
			require.Empty(t, parts)
		})
	}
}

func TestRunTurnContentMapping(t *testing.T) {
	t.Parallel()

	const (
		assistant = codersdk.ChatMessageRoleAssistant
		tool      = codersdk.ChatMessageRoleTool
	)
	completed, failed := ptr.Ref(acp.ToolCallStatusCompleted), ptr.Ref(acp.ToolCallStatusFailed)
	usage := &acp.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, ThoughtTokens: ptr.Ref(7)}
	readInput := map[string]any{"path": "main.go"}
	readCall := fantasy.ToolCallContent{ToolCallID: "tool-1", ToolName: "Read file", Input: `{"path":"main.go"}`}
	readResult := fantasy.ToolResultContent{ToolCallID: "tool-1", ToolName: "Read file", Result: fantasy.ToolResultOutputContentText{Text: "package main"}}
	review := acp.AvailableCommand{Name: "review", Description: "Review the diff"}
	compact := acp.AvailableCommand{Name: "compact", Description: "Compact history"}

	tests := []struct {
		name      string
		script    agentScript
		want      chatacp.TurnOutcome
		wantParts []publishedPart
	}{
		{
			name: "TextAndReasoningChunks",
			script: agentScript{
				promptUpdates: []acp.SessionUpdate{thoughtChunk("thinking..."), textChunk("Hello "), textChunk("world")},
				usage:         usage,
			},
			want: chatacp.TurnOutcome{
				SessionID: "session-new",
				Content:   []fantasy.Content{fantasy.ReasoningContent{Text: "thinking..."}, fantasy.TextContent{Text: "Hello world"}},
				Usage:     usage,
			},
			wantParts: []publishedPart{
				part(assistant, fantasy.ReasoningContent{Text: "thinking..."}),
				part(assistant, fantasy.TextContent{Text: "Hello "}),
				part(assistant, fantasy.TextContent{Text: "world"}),
			},
		},
		{
			name: "ToolCallKeepsArrivalOrder",
			script: agentScript{promptUpdates: []acp.SessionUpdate{
				textChunk("Let me check."),
				toolCall("tool-1", "Read file", acp.ToolKindRead, readInput),
				toolCallUpdate("tool-1", completed, nil, "package main"),
				textChunk("Done."),
			}},
			want: chatacp.TurnOutcome{
				SessionID: "session-new",
				Content:   []fantasy.Content{fantasy.TextContent{Text: "Let me check."}, readCall, readResult, fantasy.TextContent{Text: "Done."}},
			},
			wantParts: []publishedPart{
				part(assistant, fantasy.TextContent{Text: "Let me check."}),
				part(assistant, readCall),
				part(tool, readResult),
				part(assistant, fantasy.TextContent{Text: "Done."}),
			},
		},
		{
			// Input delivered after the call opened patches the durable
			// content and re-emits the call part.
			name: "ToolCallInputArrivesLater",
			script: agentScript{promptUpdates: []acp.SessionUpdate{
				toolCall("tool-1", "Read file", acp.ToolKindRead, nil),
				toolCallUpdate("tool-1", nil, readInput, ""),
				toolCallUpdate("tool-1", completed, nil, "package main"),
			}},
			want: chatacp.TurnOutcome{SessionID: "session-new", Content: []fantasy.Content{readCall, readResult}},
			wantParts: []publishedPart{
				part(assistant, fantasy.ToolCallContent{ToolCallID: "tool-1", ToolName: "Read file"}),
				part(assistant, readCall),
				part(tool, readResult),
			},
		},
		{
			name: "FailedToolCall",
			script: agentScript{promptUpdates: []acp.SessionUpdate{
				toolCall("tool-1", "", acp.ToolKindExecute, nil),
				toolCallUpdate("tool-1", failed, nil, "command not found"),
			}},
			want: chatacp.TurnOutcome{SessionID: "session-new", Content: []fantasy.Content{
				fantasy.ToolCallContent{ToolCallID: "tool-1", ToolName: "execute"},
				fantasy.ToolResultContent{ToolCallID: "tool-1", ToolName: "execute", Result: fantasy.ToolResultOutputContentError{Error: xerrors.New("command not found")}},
			}},
			wantParts: []publishedPart{
				part(assistant, fantasy.ToolCallContent{ToolCallID: "tool-1", ToolName: "execute"}),
				part(tool, fantasy.ToolResultContent{ToolCallID: "tool-1", ToolName: "execute", Result: fantasy.ToolResultOutputContentError{Error: xerrors.New("command not found")}}),
			},
		},
		{
			name:      "NoCommandUpdate",
			script:    agentScript{promptUpdates: []acp.SessionUpdate{textChunk("done")}},
			want:      chatacp.TurnOutcome{SessionID: "session-new", Content: []fantasy.Content{fantasy.TextContent{Text: "done"}}},
			wantParts: []publishedPart{part(assistant, fantasy.TextContent{Text: "done"})},
		},
		{
			// Command updates are session metadata, not transcript.
			name: "CommandUpdateDropsBlankNames",
			script: agentScript{promptUpdates: []acp.SessionUpdate{
				commandsUpdate(
					acp.AvailableCommand{Name: "review", Description: "Review the diff", Input: &acp.AvailableCommandInput{Unstructured: &acp.UnstructuredCommandInput{Hint: "pr number"}}},
					acp.AvailableCommand{Name: "init", Description: "Create a guide"},
					acp.AvailableCommand{Name: "  ", Description: "dropped: blank name"},
				),
				textChunk("done"),
			}},
			want: chatacp.TurnOutcome{
				SessionID: "session-new",
				Content:   []fantasy.Content{fantasy.TextContent{Text: "done"}},
				AvailableCommands: []codersdk.ChatRuntimeCommand{
					{Name: "review", Description: "Review the diff", InputHint: "pr number"},
					{Name: "init", Description: "Create a guide"},
				},
			},
			wantParts: []publishedPart{part(assistant, fantasy.TextContent{Text: "done"})},
		},
		{
			name:   "LatestCommandUpdateWins",
			script: agentScript{promptUpdates: []acp.SessionUpdate{commandsUpdate(review), commandsUpdate(compact), textChunk("done")}},
			want: chatacp.TurnOutcome{
				SessionID:         "session-new",
				Content:           []fantasy.Content{fantasy.TextContent{Text: "done"}},
				AvailableCommands: []codersdk.ChatRuntimeCommand{{Name: "compact", Description: "Compact history"}},
			},
			wantParts: []publishedPart{part(assistant, fantasy.TextContent{Text: "done"})},
		},
		{
			name:   "EmptyCommandUpdateClearsList",
			script: agentScript{promptUpdates: []acp.SessionUpdate{commandsUpdate(review), commandsUpdate(), textChunk("done")}},
			want: chatacp.TurnOutcome{
				SessionID:         "session-new",
				Content:           []fantasy.Content{fantasy.TextContent{Text: "done"}},
				AvailableCommands: []codersdk.ChatRuntimeCommand{},
			},
			wantParts: []publishedPart{part(assistant, fantasy.TextContent{Text: "done"})},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			outcome, _, parts := runScriptedTurn(t, tc.script, chatacp.TurnInput{Cwd: "/home/coder", PromptText: "hi"})
			requireOutcome(t, tc.want, outcome)
			require.Equal(t, tc.wantParts, parts)
		})
	}
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
	_, err := chatacp.RunTurn(turnCtx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
		Cwd:        "/home/coder",
		PromptText: "long task",
		Publish:    recorder.publish,
		Logger:     testLogger(t),
	})
	require.NoError(t, err)
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
	require.Contains(t, parts[0].part.Text, "ask an organization administrator to change the runtime's permission mode")
}

// TestRunTurnRejectedSessionMode verifies that an adapter refusing the
// configured session mode fails the turn before any prompt is sent,
// instead of silently running under a different mode.
func TestRunTurnRejectedSessionMode(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	agent := &chatacptest.FakeAgent{}
	agent.OnSetSessionMode = func(acp.SetSessionModeRequest) error {
		return xerrors.New("unknown mode")
	}

	_, err := chatacp.RunTurn(ctx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
		Cwd:            "/home/coder",
		PromptText:     "hello",
		PermissionMode: "bogus-mode",
		Publish:        func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {},
		Logger:         testLogger(t),
	})
	modeErr, ok := errors.AsType[*chatacp.SessionModeError](err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, "bogus-mode", modeErr.Mode)
	require.Empty(t, agent.Prompts())
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
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().NewTimer("chatacp", "cancel-resolve")
	defer trap.Close()

	promptStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	agent := &chatacptest.FakeAgent{}
	agent.OnPrompt = func(context.Context, *acp.AgentSideConnection, acp.PromptRequest) (acp.PromptResponse, error) {
		promptStarted <- struct{}{}
		// Never resolve until released: simulates an adapter that
		// ignores session/cancel.
		<-release
		return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
	}
	t.Cleanup(func() { close(release) })

	turnCtx, cancelTurn := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := chatacp.RunTurn(turnCtx, &chatacptest.PipeTransport{Agent: agent}, chatacp.TurnInput{
			Cwd:        "/home/coder",
			PromptText: "hang",
			Logger:     testLogger(t),
			Clock:      clock,
		})
		done <- err
	}()

	testutil.RequireReceive(ctx, t, promptStarted)
	cancelTurn()
	call := trap.MustWait(ctx)
	call.MustRelease(ctx)
	clock.Advance(call.Duration).MustWait(ctx)
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), context.Canceled)
}
