package chatd

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
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

// preload records the governing request the turn's prepared history already
// determined, so the gate never reads history itself. It has no effect once
// the gate decided.
func (g *finalizerGate) preload(requestID uuid.UUID) {
	g.once.Do(func() { g.requestID, g.governs = requestID, requestID != uuid.Nil })
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

// finalizerSeparateStepFeedback answers a finalizer call that shared its
// step with another tool call the exclusive policy does not cover.
const finalizerSeparateStepFeedback = chatstructured.FinalizerToolName + " must be called alone in its own step, without other tool calls. Call it again by itself."

// finalizerUnstorableFeedback answers a valid output whose candidate would
// not decode once stored, for example because of a huge number.
const finalizerUnstorableFeedback = "structured output is too large to store. Call " + chatstructured.FinalizerToolName + " again with a smaller output."

// structuredTurnFor returns the open structured output request of history's
// latest user turn with its compiled schema, or a zero state and nil schema
// when none is open. A schema that no longer compiles is a configuration
// error.
func structuredTurnFor(ctx context.Context, logger slog.Logger, chatID uuid.UUID, history []database.ChatMessage) (chatstructured.ActiveRequestState, *chatstructured.Schema, error) {
	state, open := openStructuredRequest(ctx, logger, chatID, history)
	if !open {
		return chatstructured.ActiveRequestState{}, nil, nil
	}
	schema, err := chatstructured.CompileSchema(state.Request.Schema)
	if err != nil {
		return state, nil, newStructuredConfigurationError(state, structuredInvalidSchemaReason)
	}
	return state, schema, nil
}

// finalizerBatchControls decides, on the server, the structured output
// control for an executed batch holding finalizer results, and returns it
// keyed by the first finalizer call ID. step holds the parts of the
// assistant row that emitted the batch. Only a lone finalizer call in a step
// without other tool calls or an earlier rejection can yield a candidate:
// the exact output bytes its arguments carry when they satisfy the schema.
// Any other batch records one rejection for the step unless screening
// already did, and a finalizer result that would acknowledge such a call is
// replaced in content with fixed feedback.
func finalizerBatchControls(schema *chatstructured.Schema, requestID uuid.UUID, step []codersdk.ChatMessagePart, content []fantasy.Content) (map[string][]codersdk.ChatMessagePart, error) {
	var finalizers []codersdk.ChatMessagePart
	siblings, rejected := 0, false
	for _, part := range step {
		switch {
		case isFinalizerCallPart(part):
			finalizers = append(finalizers, part)
		case part.Type == codersdk.ChatMessagePartTypeToolCall:
			siblings++
		case part.Type == codersdk.ChatMessagePartTypeStructuredOutputControl:
			control, err := chatstructured.DecodeControlPart(part)
			rejected = rejected || (err == nil && control.Kind == chatstructured.ControlRejection)
		}
	}
	lone := len(finalizers) == 1 && siblings == 0 && !rejected
	firstID := ""
	for i, block := range content {
		result, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok || result.ToolName != chatstructured.FinalizerToolName {
			continue
		}
		if firstID == "" {
			firstID = result.ToolCallID
		}
		if _, isError := result.Result.(fantasy.ToolResultOutputContentError); !lone && !isError {
			result.Result = fantasy.ToolResultOutputContentError{Error: xerrors.New(finalizerSeparateStepFeedback)}
			content[i] = result
		}
	}
	if firstID == "" || (!lone && rejected) {
		return map[string][]codersdk.ChatMessagePart{}, nil
	}
	control := chatstructured.Control{RequestID: requestID, Kind: chatstructured.ControlRejection}
	if lone {
		output, err := chatstructured.FinalizerOutput(finalizers[0].Args)
		if err == nil {
			_, err = schema.Validate(output)
		}
		if err == nil {
			part, err := chatstructured.EncodeControlPart(chatstructured.Control{RequestID: requestID, Kind: chatstructured.ControlCandidate, Value: output})
			if err == nil {
				// The succeeded receipt stores the same value, a few bytes larger.
				_, err = chatstructured.EncodeOutcomePart(codersdk.ChatStructuredOutput{RequestID: requestID, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: output})
			}
			if err == nil {
				return map[string][]codersdk.ChatMessagePart{firstID: {part}}, nil
			}
			// The runner acknowledged the output, so replace that result.
			replaceFinalizerResult(content, firstID, finalizerUnstorableFeedback)
		}
	}
	part, err := chatstructured.EncodeControlPart(control)
	if err != nil {
		return nil, xerrors.Errorf("encode structured output control: %w", err)
	}
	return map[string][]codersdk.ChatMessagePart{firstID: {part}}, nil
}

func replaceFinalizerResult(content []fantasy.Content, id, feedback string) {
	for i, block := range content {
		if result, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok && result.ToolCallID == id {
			result.Result = fantasy.ToolResultOutputContentError{Error: xerrors.New(feedback)}
			content[i] = result
		}
	}
}

// Configuration failures that close an open structured output request.
const (
	structuredStrictToolsReason   = "The model's strict tool schema setting cannot be used with structured output."
	structuredToolCollisionReason = "Another tool is named " + chatstructured.FinalizerToolName + ", which structured output reserves."
	structuredInvalidSchemaReason = "The stored structured output schema no longer compiles."
)

// structuredConfigurationError ends a turn whose open structured output
// request cannot be served as specified; finishGenerationError closes the
// request with a failed configuration_error receipt carrying reason. The
// reason is fixed text: it never includes schema values, tool arguments,
// provider responses or credentials.
type structuredConfigurationError struct {
	requestID    uuid.UUID
	requestRowID int64
	reason       string
}

func (e *structuredConfigurationError) Error() string {
	return "structured output configuration error: " + e.reason
}

// newStructuredConfigurationError returns a terminal generation error whose
// user-facing message is reason.
func newStructuredConfigurationError(state chatstructured.ActiveRequestState, reason string) error {
	return terminalGeneration(chaterror.WithClassification(
		&structuredConfigurationError{requestID: state.Request.RequestID, requestRowID: state.RequestRowID, reason: reason},
		chaterror.ClassifiedError{Message: reason, Kind: codersdk.ChatErrorKindConfig},
	))
}

// finalizerConfigurationReason returns why the finalizer cannot be offered
// as specified, or "". Strict tool schemas apply to every function tool in
// the pinned OpenAI Responses SDK, so the finalizer cannot be exempted; any
// other tool, dynamic tool, provider tool or alias with the finalizer's name
// would collide with it. Nothing is altered to make either case pass.
func finalizerConfigurationReason(options fantasy.ProviderOptions, tools []fantasy.AgentTool, dynamic map[string]bool, providerTools []chatloop.ProviderTool, aliases map[string]string) string {
	for _, data := range options {
		if responses, ok := data.(*fantasyopenai.ResponsesProviderOptions); ok && responses.StrictJSONSchema != nil && *responses.StrictJSONSchema {
			return structuredStrictToolsReason
		}
	}
	names := make([]string, 0, len(tools)+len(providerTools)+2*len(aliases))
	for _, tool := range tools {
		names = append(names, tool.Info().Name)
	}
	for _, tool := range providerTools {
		names = append(names, tool.Definition.GetName())
	}
	for from, to := range aliases {
		names = append(names, from, to)
	}
	if dynamic[chatstructured.FinalizerToolName] || slices.Contains(names, chatstructured.FinalizerToolName) {
		return structuredToolCollisionReason
	}
	return ""
}
