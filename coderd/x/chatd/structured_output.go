package chatd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"slices"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
)

var (
	// ErrStructuredOutputMode rejects a structured output request for a
	// chat in plan mode, with a mode, or with a parent.
	ErrStructuredOutputMode = xerrors.New("structured output requires a root chat outside plan mode")
	// ErrStructuredOutputBusy rejects a structured output request for a
	// chat that cannot start its turn at once.
	ErrStructuredOutputBusy = chatstate.ErrStructuredOutputBusy
	// ErrStructuredOutputPending rejects turning on plan mode while the
	// chat has a pending structured output result.
	ErrStructuredOutputPending = xerrors.New("plan mode cannot be enabled while a structured output result is pending")
)

// ParseResponseFormat parses the raw response_format member of a chat
// request strictly, with exact, unique keys at both object levels, and
// compiles the schema. An ordinary request (omitted, JSON null or
// {"type":"text"}) returns nil. Otherwise it returns the request to store.
// enabled reports whether the chat-structured-output experiment is on. A
// rejection names the field and never echoes schema values.
func ParseResponseFormat(raw json.RawMessage, enabled bool) (json.RawMessage, *codersdk.ValidationError) {
	if raw = bytes.TrimSpace(raw); len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	reject := func(field, detail string) (json.RawMessage, *codersdk.ValidationError) {
		return nil, &codersdk.ValidationError{Field: field, Detail: detail}
	}
	format, ok := jsonObjectMembers(raw, "type", "json_schema")
	var typ string
	typeErr := json.Unmarshal(format["type"], &typ)
	if ok && len(format) == 1 && typeErr == nil && typ == string(codersdk.ChatResponseFormatTypeText) {
		return nil, nil
	}
	switch {
	case !enabled:
		return reject("response_format", "Structured output is disabled. Enable the chat-structured-output experiment.")
	case !ok:
		return reject("response_format", "Must be a JSON object with unique keys, holding only type and json_schema.")
	case typeErr == nil && typ == string(codersdk.ChatResponseFormatTypeText):
		return reject("response_format", "json_schema requires type json_schema.")
	case typeErr != nil || typ != string(codersdk.ChatResponseFormatTypeJSONSchema):
		return reject("response_format.type", `Must be "text" or "json_schema".`)
	}
	spec, ok := jsonObjectMembers(format["json_schema"], "name", "description", "schema")
	if !ok {
		return reject("response_format.json_schema", "Must be an object with unique keys, holding only name, description and schema.")
	}
	var request chatstructured.Request
	if json.Unmarshal(spec["name"], &request.Name) != nil || !chatstructured.ValidRequestName(request.Name) {
		return reject("response_format.json_schema.name", "Must match ^[A-Za-z0-9_-]{1,64}$.")
	}
	if raw, present := spec["description"]; present {
		// ParseOutputValue rejects invalid UTF-8, unpaired surrogate escapes
		// and U+0000, which json.Unmarshal would silently replace.
		_, err := chatstructured.ParseOutputValue(raw)
		if err != nil || raw[0] != '"' || json.Unmarshal(raw, &request.Description) != nil || len(request.Description) > 1024 {
			return reject("response_format.json_schema.description", "Must be a string of at most 1024 bytes of valid UTF-8.")
		}
	}
	request.Schema = bytes.TrimSpace(spec["schema"])
	if len(request.Schema) == 0 || request.Schema[0] != '{' {
		return reject("response_format.json_schema.schema", "Must be a JSON Schema object.")
	}
	if _, err := chatstructured.CompileSchema(request.Schema); err != nil {
		return reject("response_format.json_schema.schema", "Must be a supported JSON Schema within the size and complexity limits.")
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return reject("response_format.json_schema", "Cannot be stored.")
	}
	return encoded, nil
}

// jsonObjectMembers splits a JSON object into its raw members, reporting
// false for any other value, a key outside allowed, or a repeated key, which
// lenient decoding would silently resolve.
func jsonObjectMembers(raw json.RawMessage, allowed ...string) (map[string]json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	members := make(map[string]json.RawMessage)
	for dec.More() {
		tok, err := dec.Token()
		key, isKey := tok.(string)
		if _, seen := members[key]; err != nil || !isKey || seen || !slices.Contains(allowed, key) {
			return nil, false
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, false
		}
		members[key] = value
	}
	if _, err := dec.Token(); err != nil {
		return nil, false
	}
	// Exactly one object: any token after it, even a closing one, fails.
	_, err := dec.Token()
	return members, errors.Is(err, io.EOF)
}

// checkStructuredOutputSend applies the structured output rules of a send
// under the chat lock: a request needs a root chat outside plan mode, and
// plan mode cannot turn on while a result is pending. chatstate refuses a
// request for a busy chat.
func checkStructuredOutputSend(ctx context.Context, store database.Store, chat database.Chat, opts SendMessageOptions) error {
	if opts.StructuredOutputRequest != nil {
		if chat.PlanMode.Valid || chat.Mode.Valid || chat.ParentChatID.Valid || (opts.PlanMode != nil && opts.PlanMode.Valid) {
			return ErrStructuredOutputMode
		}
		return nil
	}
	if opts.PlanMode == nil || !opts.PlanMode.Valid {
		return nil
	}
	pending, err := HasPendingStructuredOutput(ctx, store, chat.ID)
	if err != nil {
		return err
	}
	if pending {
		return ErrStructuredOutputPending
	}
	return nil
}

// HasPendingStructuredOutput reports whether the chat has a pending
// structured output result.
func HasPendingStructuredOutput(ctx context.Context, store database.Store, chatID uuid.UUID) (bool, error) {
	_, err := store.GetPendingChatStructuredOutput(ctx, chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, xerrors.Errorf("get pending structured output: %w", err)
	}
	return true, nil
}

// structuredTurn is the pending structured output request that a turn
// serves, with its compiled schema.
type structuredTurn struct {
	pending database.ChatStructuredOutput
	schema  *chatstructured.Schema
}

// loadStructuredTurn returns the chat's pending structured output request,
// or nil. A stored schema that no longer compiles is a configuration error.
func loadStructuredTurn(ctx context.Context, store database.Store, chatID uuid.UUID) (*structuredTurn, error) {
	pending, err := store.GetPendingChatStructuredOutput(ctx, chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get pending structured output: %w", err)
	}
	var request chatstructured.Request
	if err := json.Unmarshal(pending.Request, &request); err != nil {
		return nil, newStructuredConfigurationError(pending, structuredInvalidSchemaReason)
	}
	schema, err := chatstructured.CompileSchema(request.Schema)
	if err != nil {
		return nil, newStructuredConfigurationError(pending, structuredInvalidSchemaReason)
	}
	return &structuredTurn{pending: pending, schema: schema}, nil
}

// Configuration failures that fail a pending structured output request.
const (
	structuredStrictToolsReason   = "The model's strict tool schema setting cannot be used with structured output."
	structuredToolCollisionReason = "Another tool is named " + chatstructured.FinalizerToolName + ", which structured output reserves."
	structuredInvalidSchemaReason = "The stored structured output schema no longer compiles."
)

// structuredConfigurationError ends a turn whose pending structured output
// request cannot be served; finishGenerationError fails the request with
// configuration_error in the same transaction. The reason is fixed text.
type structuredConfigurationError struct {
	pending database.ChatStructuredOutput
	reason  string
}

func (e *structuredConfigurationError) Error() string {
	return "structured output configuration error: " + e.reason
}

func newStructuredConfigurationError(pending database.ChatStructuredOutput, reason string) error {
	return terminalGeneration(chaterror.WithClassification(
		&structuredConfigurationError{pending: pending, reason: reason},
		chaterror.ClassifiedError{Message: reason, Kind: codersdk.ChatErrorKindConfig},
	))
}

// finalizerConfigurationReason returns why the finalizer cannot be offered,
// or "". Strict tool schemas apply to every function tool in the pinned
// OpenAI Responses SDK, so the finalizer cannot be exempted. Any other tool,
// dynamic tool, provider tool or alias with the finalizer's name collides.
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

// finalizerPlaceholderArgs replaces rejected finalizer arguments. It lacks
// the output property, so it never passes the finalizer runner.
var finalizerPlaceholderArgs = `{}`

// screenFinalizerCalls checks the original argument bytes of each finalizer
// call in a completed step before it is stored: the envelope caps and the
// finish reason (a cut call is incomplete). A rejected call keeps its place
// with placeholder arguments and one error result with fixed feedback, so it
// is never dispatched. It returns the number of rejected calls.
func screenFinalizerCalls(content []fantasy.Content, finish fantasy.FinishReason) ([]fantasy.Content, int) {
	out, rejected := content, 0
	for i, block := range content {
		call, ok := fantasy.AsContentType[fantasy.ToolCallContent](block)
		if !ok || call.ToolName != chatstructured.FinalizerToolName || call.ProviderExecuted {
			continue
		}
		err := chatstructured.ErrFinalizerCallIncomplete
		if finish == fantasy.FinishReasonStop || finish == fantasy.FinishReasonToolCalls {
			err = chatstructured.ScreenFinalizerArguments([]byte(call.Input))
		}
		if err == nil {
			continue
		}
		if rejected == 0 {
			out = slices.Clone(content)
		}
		rejected++
		call.Input = finalizerPlaceholderArgs
		out[i] = call
		feedback := fantasy.ToolResultOutputContentError{Error: xerrors.New(chatstructured.FinalizerFeedback(err))}
		// chatloop already answers a call that the output token limit cut;
		// that result takes the feedback, so the call keeps one result.
		answered := false
		for j, other := range out {
			if result, ok := fantasy.AsContentType[fantasy.ToolResultContent](other); ok && result.ToolCallID == call.ToolCallID {
				result.Result, answered = feedback, true
				out[j] = result
			}
		}
		if !answered {
			out = append(out, fantasy.ToolResultContent{ToolCallID: call.ToolCallID, ToolName: call.ToolName, Result: feedback})
		}
	}
	return out, rejected
}

// structuredStepUpdate is what one committed step changes on the pending
// structured output result: the value of an accepted finalizer call and
// the number of rejected calls.
type structuredStepUpdate struct {
	pending  database.ChatStructuredOutput
	value    json.RawMessage
	rejected int
}

// finalizerStepUpdate reads the finalizer calls of an executed batch. The
// exclusive policy rejects a finalizer call that shares its batch, so a
// successful result means a lone call whose arguments the runner validated.
func finalizerStepUpdate(structured *structuredTurn, calls []fantasy.ToolCallContent, content []fantasy.Content) (*structuredStepUpdate, error) {
	if structured == nil {
		return nil, nil
	}
	update := &structuredStepUpdate{pending: structured.pending}
	for _, block := range content {
		result, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok || result.ToolName != chatstructured.FinalizerToolName {
			continue
		}
		if _, failed := result.Result.(fantasy.ToolResultOutputContentError); failed {
			update.rejected++
			continue
		}
		i := slices.IndexFunc(calls, func(call fantasy.ToolCallContent) bool { return call.ToolCallID == result.ToolCallID })
		if i < 0 {
			return nil, xerrors.New("finalizer result has no call")
		}
		value, err := chatstructured.FinalizerOutput([]byte(calls[i].Input))
		if err != nil {
			return nil, xerrors.Errorf("read accepted finalizer output: %w", err)
		}
		update.value = value
	}
	if update.value == nil && update.rejected == 0 {
		return nil, nil
	}
	return update, nil
}

// apply writes the update to the pending result in the commit transaction.
// A value replaces the previous one. Without any value, the third rejected
// call fails the request as validation_exhausted.
func (u *structuredStepUpdate) apply(ctx context.Context, store database.Store) error {
	if u == nil {
		return nil
	}
	current, err := store.GetPendingChatStructuredOutput(ctx, u.pending.ChatID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && current.MessageID != u.pending.MessageID {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get pending structured output: %w", err)
	}
	current.RejectedCalls += int32(u.rejected) //nolint:gosec // A step holds few calls.
	if u.value != nil {
		current.Value = pqtype.NullRawMessage{RawMessage: u.value, Valid: true}
	}
	status, code := database.ChatStructuredOutputStatusPending, codersdk.ChatStructuredOutputErrorCode("")
	if !current.Value.Valid && current.RejectedCalls >= chatstate.RejectedCallLimit {
		status, code = database.ChatStructuredOutputStatusFailed, codersdk.ChatStructuredOutputErrorCodeValidationExhausted
	}
	return chatstate.UpdateStructuredOutput(ctx, store, current, status, code)
}
