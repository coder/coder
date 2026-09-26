package chatd

import (
	"context"
	"encoding/json"
	"sync"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
)

// finalizerPlaceholderArgs replaces rejected or interrupted finalizer
// arguments. It lacks the output property, so it can never pass
// CheckFinalizerArguments.
var finalizerPlaceholderArgs = json.RawMessage(`{}`)

// finalizerArgumentMaxBytes is the finalizer envelope byte cap.
const finalizerArgumentMaxBytes = 80 << 10

// finalizerGate screens tool calls named chatstructured.FinalizerToolName
// while the chat has an open structured output request. Such a call is
// persisted with its original arguments only after they passed the envelope
// screen in an attempt that finished normally; every other governing call is
// persisted with the placeholder and resolved by an error result in the same
// commit. A persisted governing call with other arguments therefore came from
// a screened, eligible attempt, and a placeholder call is always resolved.
// History is read once, and only when a finalizer named call appears.
type finalizerGate struct {
	load      func() (uuid.UUID, bool, error)
	once      sync.Once
	requestID uuid.UUID
	governs   bool
	err       error

	mu sync.Mutex
	// streamed counts buffered argument delta bytes per governing call.
	streamed map[string]int
}

func newFinalizerGate(load func() (uuid.UUID, bool, error)) *finalizerGate {
	return &finalizerGate{load: load, streamed: make(map[string]int)}
}

// openRequestGate reads the chat's history to decide whether finalizer calls
// govern an open structured output request.
func openRequestGate(ctx context.Context, logger slog.Logger, store database.Store, chatID uuid.UUID) *finalizerGate {
	return newFinalizerGate(func() (uuid.UUID, bool, error) {
		history, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
		if err != nil {
			return uuid.Nil, false, xerrors.Errorf("load history for structured output request: %w", err)
		}
		state, open := openStructuredRequest(ctx, logger, chatID, history)
		return state.Request.RequestID, open, nil
	})
}

func (g *finalizerGate) governing() (uuid.UUID, bool, error) {
	g.once.Do(func() { g.requestID, g.governs, g.err = g.load() })
	return g.requestID, g.governs, g.err
}

func isFinalizerCallPart(part codersdk.ChatMessagePart) bool {
	return part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName == chatstructured.FinalizerToolName && !part.ProviderExecuted
}

// screen checks the original argument bytes of each governing finalizer
// call in a completed step. A rejected call keeps its place with the
// placeholder arguments and gains an error result with fixed feedback; the
// step then carries one rejection control part for its assistant row,
// however many calls it rejected. It returns the rejected call IDs.
func (g *finalizerGate) screen(content []fantasy.Content, finish fantasy.FinishReason) ([]fantasy.Content, map[string]bool, []codersdk.ChatMessagePart, error) {
	var requestID uuid.UUID
	rejected := make(map[string]bool)
	out := content
	for i, block := range content {
		call, ok := fantasy.AsContentType[fantasy.ToolCallContent](block)
		if !ok || call.ToolName != chatstructured.FinalizerToolName || call.ProviderExecuted {
			continue
		}
		id, governs, err := g.governing()
		if err != nil || !governs {
			return content, nil, nil, err
		}
		requestID = id
		err = chatstructured.ErrFinalizerCallIncomplete
		if finish == fantasy.FinishReasonStop || finish == fantasy.FinishReasonToolCalls {
			err = chatstructured.ScreenFinalizerArguments([]byte(call.Input))
		}
		if err == nil {
			continue
		}
		if len(rejected) == 0 {
			out = append([]fantasy.Content(nil), content...)
		}
		rejected[call.ToolCallID] = true
		call.Input = string(finalizerPlaceholderArgs)
		out[i] = call
		out = append(out, fantasy.ToolResultContent{
			ToolCallID: call.ToolCallID, ToolName: call.ToolName,
			Result: fantasy.ToolResultOutputContentError{Error: xerrors.New(chatstructured.FinalizerFeedback(err))},
		})
	}
	if len(rejected) == 0 {
		return content, nil, nil, nil
	}
	control, err := chatstructured.EncodeControlPart(chatstructured.Control{RequestID: requestID, Kind: chatstructured.ControlRejection})
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("encode structured output rejection: %w", err)
	}
	return out, rejected, []codersdk.ChatMessagePart{control}, nil
}

// forward returns the part to buffer, or false to drop it. Argument deltas
// of a governing finalizer call stop once their total passes the envelope
// byte cap, and its complete part then carries the placeholder, so buffer
// memory and pubsub stay bounded. Completion screening rejects the call as
// too large. When history cannot be read the cap applies anyway, and
// screening fails the step.
func (g *finalizerGate) forward(part codersdk.ChatMessagePart) (codersdk.ChatMessagePart, bool) {
	if !isFinalizerCallPart(part) {
		return part, true
	}
	if _, governs, err := g.governing(); !governs && err == nil {
		return part, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.streamed[part.ToolCallID] += len(part.ArgsDelta)
	if g.streamed[part.ToolCallID] <= finalizerArgumentMaxBytes && len(part.Args) <= finalizerArgumentMaxBytes {
		return part, true
	}
	if part.ArgsDelta != "" {
		return part, false
	}
	part.Args = finalizerPlaceholderArgs
	return part, true
}
