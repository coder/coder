package chatd

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const rawFinalizerMarker = "RAW_FINALIZER_MARKER"

func finalizerCall(id, input string) fantasy.ToolCallContent {
	return fantasy.ToolCallContent{ToolCallID: id, ToolName: chatstructured.FinalizerToolName, Input: input}
}

func governingGate(requestID uuid.UUID) *finalizerGate {
	return newFinalizerGate(func() (uuid.UUID, bool, error) { return requestID, true, nil })
}

func TestScreenFinalizerCalls(t *testing.T) {
	t.Parallel()
	requestID := uuid.New()
	valid := `{"output":{"note":"` + rawFinalizerMarker + `"}}`
	toolCalls, stop := fantasy.FinishReasonToolCalls, fantasy.FinishReasonStop
	for _, tt := range []struct {
		name   string
		input  string
		finish fantasy.FinishReason
		want   error
	}{
		{"DuplicateKey", `{"output":1,"output":"` + rawFinalizerMarker + `"}`, toolCalls, chatstructured.ErrDuplicateKey},
		{"NullCharacter", `{"output":"` + rawFinalizerMarker + `\u0000"}`, toolCalls, chatstructured.ErrNullCharacter},
		{"UnpairedSurrogate", `{"output":"` + rawFinalizerMarker + `\ud800"}`, toolCalls, chatstructured.ErrUnpairedSurrogate},
		{"HugeExponent", `{"output":1e2000}`, toolCalls, chatstructured.ErrExponentTooLarge},
		{"InvalidUTF8", `{"output":"` + rawFinalizerMarker + "\xff" + `"}`, toolCalls, chatstructured.ErrInvalidUTF8},
		{"TooLarge", `{"output":"` + rawFinalizerMarker + strings.Repeat("a", 80<<10) + `"}`, stop, chatstructured.ErrTooLarge},
		{"MissingOutput", `{"value":"` + rawFinalizerMarker + `"}`, toolCalls, chatstructured.ErrInvalidFinalizerArguments},
		{"ExtraKey", `{"output":1,"x":"` + rawFinalizerMarker + `"}`, toolCalls, chatstructured.ErrInvalidFinalizerArguments},
		{"FinishLength", valid, fantasy.FinishReasonLength, chatstructured.ErrFinalizerCallIncomplete},
		{"FinishContentFilter", valid, fantasy.FinishReasonContentFilter, chatstructured.ErrFinalizerCallIncomplete},
		{"FinishError", valid, fantasy.FinishReasonError, chatstructured.ErrFinalizerCallIncomplete},
		{"AcceptedToolCalls", valid, toolCalls, nil},
		{"AcceptedStop", valid, stop, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			content := []fantasy.Content{fantasy.TextContent{Text: "t"}, finalizerCall("c1", tt.input)}
			got, rejected, extra, err := governingGate(requestID).screen(content, tt.finish)
			require.NoError(t, err)
			if tt.want == nil {
				require.Equal(t, content, got)
				require.Empty(t, rejected)
				require.Empty(t, extra)
				return
			}
			require.Equal(t, map[string]bool{"c1": true}, rejected)
			require.Len(t, got, 3)
			require.Equal(t, finalizerCall("c1", "{}"), got[1])
			result, ok := fantasy.AsContentType[fantasy.ToolResultContent](got[2])
			require.True(t, ok)
			require.Equal(t, "c1", result.ToolCallID)
			output, ok := result.Result.(fantasy.ToolResultOutputContentError)
			require.True(t, ok)
			require.Equal(t, chatstructured.FinalizerFeedback(tt.want), output.Error.Error())
			require.Len(t, extra, 1)
			control, err := chatstructured.DecodeControlPart(extra[0])
			require.NoError(t, err)
			require.Equal(t, chatstructured.Control{RequestID: requestID, Kind: chatstructured.ControlRejection}, control)
		})
	}

	// Two bad calls in one step count as one rejection with two results.
	bad := `{"output":1,"output":2}`
	got, rejected, extra, err := governingGate(requestID).screen([]fantasy.Content{finalizerCall("a", bad), finalizerCall("b", bad)}, toolCalls)
	require.NoError(t, err)
	require.Len(t, rejected, 2)
	require.Len(t, got, 4)
	require.Len(t, extra, 1)

	// Without an open request the name is ordinary, and steps without a
	// finalizer call never read history.
	loads := 0
	ordinary := newFinalizerGate(func() (uuid.UUID, bool, error) { loads++; return uuid.Nil, false, nil })
	content := []fantasy.Content{fantasy.ToolCallContent{ToolCallID: "x", ToolName: "execute", Input: bad}}
	got, rejected, extra, err = ordinary.screen(content, fantasy.FinishReasonLength)
	require.NoError(t, err)
	require.Equal(t, content, got)
	require.Empty(t, rejected)
	require.Empty(t, extra)
	require.Zero(t, loads)
	content = []fantasy.Content{finalizerCall("c1", bad)}
	got, rejected, extra, err = ordinary.screen(content, toolCalls)
	require.NoError(t, err)
	require.Equal(t, content, got)
	require.Empty(t, rejected)
	require.Empty(t, extra)
	require.Equal(t, 1, loads)
}

func TestInterruptPlaceholdersGoverningFinalizerArguments(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newTaskTestFixture(t)
	requestID := uuid.New()
	chat, _ := createStructuredTestChat(t, f, structuredUserMessage(t, f, requestID))
	workerID, runnerID := uuid.New(), uuid.New()
	acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	key := messagepartbuffer.Key{ChatID: chat.ID, HistoryVersion: acquired.HistoryVersion, GenerationAttempt: acquired.GenerationAttempt}
	buffer := starter.opts.MessagePartBuffer
	require.NoError(t, buffer.CreateEpisode(key))
	finalizer := codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolName: chatstructured.FinalizerToolName}
	complete, streaming := finalizer, finalizer
	complete.ToolCallID, complete.Args = "c1", json.RawMessage(`{"output":1,"output":"`+rawFinalizerMarker+`"}`)
	streaming.ToolCallID, streaming.ArgsDelta = "c2", `{"output":"`+rawFinalizerMarker+`"}`
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, complete))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, streaming))
	interrupting := f.forceExecutionState(t, chat.ID, database.ChatStatusInterrupting, false, sql.NullTime{})

	require.NoError(t, starter.StartInterrupt(ctx, chatWorkerTaskStartInput{
		ChatID: chat.ID, WorkerID: workerID, RunnerID: runnerID, HistoryVersion: interrupting.HistoryVersion,
		GenerationAttempt: interrupting.GenerationAttempt, Status: database.ChatStatusInterrupting,
	}))
	history, state := structuredHistory(t, f, chat.ID)
	var args []string
	for _, msg := range history {
		require.NotContains(t, string(msg.Content.RawMessage), rawFinalizerMarker)
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeToolCall {
				args = append(args, string(part.Args))
			}
		}
	}
	require.Equal(t, []string{"{}", "{}"}, args)
	require.Nil(t, state.Candidate)
	require.True(t, state.Closed)
	unresolved, _, err := unresolvedToolCallsFromHistory(history, nil)
	require.NoError(t, err)
	require.Empty(t, unresolved)
}

func TestFinalizerStreamCap(t *testing.T) {
	t.Parallel()
	for _, governing := range []bool{true, false} {
		t.Run(map[bool]string{true: "OpenRequest", false: "Ordinary"}[governing], func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newTaskTestFixture(t)
			initial := taskUserTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID)
			if governing {
				initial = structuredUserMessage(t, f, uuid.New())
			}
			chat, machine := createStructuredTestChat(t, f, initial)
			workerID, runnerID := uuid.New(), uuid.New()
			acquired := f.acquireChat(t, chat.ID, workerID, runnerID)
			starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
			attempt, err := starter.beginGenerationAttempt(ctx, machine, chatWorkerTaskStartInput{
				ChatID: chat.ID, WorkerID: workerID, RunnerID: runnerID, HistoryVersion: acquired.HistoryVersion, Status: database.ChatStatusRunning,
			})
			require.NoError(t, err)
			call := func(id, name, delta, args string) codersdk.ChatMessagePart {
				part := codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: id, ToolName: name, ArgsDelta: delta}
				if args != "" {
					part.Args = json.RawMessage(args)
				}
				return part
			}
			// Three 30 KiB chunks pass the 80 KiB envelope cap on the third.
			finalizerChunk, otherChunk := strings.Repeat("f", 30<<10), strings.Repeat("e", 30<<10)
			for _, chunks := range [][2]string{{`{"output":"` + finalizerChunk, `{"output":"` + otherChunk}, {finalizerChunk, otherChunk}, {finalizerChunk, otherChunk}, {`"}`, `"}`}} {
				attempt.publish(codersdk.ChatMessageRoleAssistant, call("c1", chatstructured.FinalizerToolName, chunks[0], ""))
				attempt.publish(codersdk.ChatMessageRoleAssistant, call("x1", "execute", chunks[1], ""))
			}
			full := `{"output":"` + strings.Repeat(finalizerChunk, 3) + `"}`
			attempt.publish(codersdk.ChatMessageRoleAssistant, call("c1", chatstructured.FinalizerToolName, "", full))

			current, err := f.db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			buffered, err := starter.opts.MessagePartBuffer.GetParts(messagepartbuffer.Key{ChatID: chat.ID, HistoryVersion: current.HistoryVersion, GenerationAttempt: attempt.number})
			require.NoError(t, err)
			streamed := map[string]int{}
			var final string
			for _, p := range buffered {
				streamed[p.MessagePart.ToolCallID] += len(p.MessagePart.ArgsDelta)
				if p.MessagePart.ToolCallID == "c1" && p.MessagePart.ArgsDelta == "" {
					final = string(p.MessagePart.Args)
				}
			}
			require.Equal(t, len(full), streamed["x1"])
			if !governing {
				require.Equal(t, len(full), streamed["c1"])
				require.Equal(t, full, final)
				return
			}
			require.LessOrEqual(t, streamed["c1"], 80<<10)
			require.Equal(t, "{}", final)

			// An interrupt after the cap persists the placeholder only.
			interrupting := f.forceExecutionState(t, chat.ID, database.ChatStatusInterrupting, false, sql.NullTime{})
			require.NoError(t, starter.StartInterrupt(ctx, chatWorkerTaskStartInput{
				ChatID: chat.ID, WorkerID: workerID, RunnerID: runnerID, HistoryVersion: interrupting.HistoryVersion,
				GenerationAttempt: interrupting.GenerationAttempt, Status: database.ChatStatusInterrupting,
			}))
			history, _ := structuredHistory(t, f, chat.ID)
			for _, msg := range history {
				require.NotContains(t, string(msg.Content.RawMessage), finalizerChunk[:64])
			}
		})
	}

	// Parts of other tools never read history.
	loads := 0
	gate := newFinalizerGate(func() (uuid.UUID, bool, error) { loads++; return uuid.New(), true, nil })
	_, ok := gate.forward(codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "x", ToolName: "execute", ArgsDelta: strings.Repeat("e", 90<<10)})
	require.True(t, ok)
	require.Zero(t, loads)
}
