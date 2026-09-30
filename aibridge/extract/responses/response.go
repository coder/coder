package responses

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/extract"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// Responses API event and item type names.
const (
	eventCreated    = "response.created"
	eventInProgress = "response.in_progress"
	eventQueued     = "response.queued"
	eventCompleted  = "response.completed"
	eventIncomplete = "response.incomplete"
	eventFailed     = "response.failed"
	eventError      = "error"

	itemFunctionCall   = "function_call"
	itemCustomToolCall = "custom_tool_call"
	itemReasoning      = "reasoning"
	itemMessage        = "message"
)

// Relevant reports whether OnEvent acts on events of the given type, so
// callers can skip others, such as output deltas, without changing the
// records or the outcome. Pass the event JSON's "type" field when known:
// OnEvent prefers it over the transport event name. An empty type is
// relevant, since OnEvent then reads the type from the JSON.
func Relevant(eventType string) bool {
	switch eventType {
	case "", eventCreated, eventInProgress, eventQueued, eventCompleted, eventIncomplete, eventFailed, eventError:
		return true
	}
	return false
}

// hostedToolItems are output item types for tools the provider executes
// itself. They carry no uniform argument payload but are recorded for
// visibility, matching the interceptor.
var hostedToolItems = map[string]bool{
	"web_search_call":       true,
	"computer_call":         true,
	"local_shell_call":      true,
	"shell_call":            true,
	"apply_patch_call":      true,
	"code_interpreter_call": true,
	"mcp_call":              true,
	"file_search_call":      true,
	"image_generation_call": true,
}

// ResponseExtraction records one Responses API response as it is observed.
// It accepts SSE events, WebSocket frames (raw event JSON with an empty
// event type), and complete bodies.
//
// It is not safe for concurrent use: call its methods sequentially, feeding
// events in stream order.
//
// The prompt is recorded as soon as the response ID is known, so a failed
// or truncated stream still records it, as in the interceptor. Token usage,
// tool calls, and model thoughts are recorded once, from the terminal
// response object (response.completed, response.incomplete,
// response.failed, or a complete body), with the interceptor's field
// semantics. A stream that ends without a terminal event records no usage,
// tool calls, or thoughts.
type ResponseExtraction struct {
	// ctx scopes the recorder calls and logging of OnEvent and
	// ProcessBlocking, which have no context of their own.
	ctx            context.Context
	logger         slog.Logger
	rec            recorder.Recorder
	interceptionID string
	prompt         string

	outcome extract.Outcome
	// recorded is set once a terminal response object was recorded.
	recorded bool
	// settled is set once any terminal event, response object or error,
	// was observed. The first one decides the outcome: later errors are
	// parse notes, and a later terminal response is only recorded.
	settled bool
	notes   extract.ParseNotes
}

var _ extract.ResponseExtraction = (*ResponseExtraction)(nil)

// NewResponseExtraction returns an extraction that writes the records of one
// response to rec. ctx scopes the recorder calls and is usually the request
// context. prompt is the request's extract.RequestFacts.Prompt; an empty
// prompt is not recorded.
//
// Recorder calls run synchronously inside OnEvent and ProcessBlocking and
// are not bounded per call: rec, or ctx, must enforce a deadline, or a
// slow recorder stalls the caller.
func NewResponseExtraction(ctx context.Context, logger slog.Logger, rec recorder.Recorder, interceptionID, prompt string) *ResponseExtraction {
	if rec == nil {
		panic("responses: NewResponseExtraction requires a recorder")
	}
	if interceptionID == "" {
		panic("responses: NewResponseExtraction requires an interception ID")
	}
	return &ResponseExtraction{
		ctx:            ctx,
		logger:         logger,
		rec:            rec,
		interceptionID: interceptionID,
		prompt:         prompt,
		notes:          extract.NewParseNotes(logger),
	}
}

// OnEvent observes one streamed event. The JSON "type" field takes
// precedence over eventType, which is only a fallback.
func (e *ResponseExtraction) OnEvent(eventType string, raw []byte) {
	ev, ok := e.parse(eventType, raw, extract.MaxEventBytes)
	if !ok {
		return
	}
	typ := ev.Get("type").String()
	if typ == "" {
		typ = eventType
	}
	switch typ {
	case eventCreated, eventInProgress, eventQueued:
		e.observeResponseID(ev.Get("response"))
	case eventCompleted:
		e.finish(extract.TerminalCompleted, ev.Get("response"))
	case eventIncomplete:
		e.finish(extract.TerminalIncomplete, ev.Get("response"))
	case eventFailed:
		e.finish(extract.TerminalFailed, ev.Get("response"))
	case eventError:
		e.streamError(ev)
	}
}

// ProcessBlocking observes a complete, decoded body. A 2xx body is a
// response object and is treated like a terminal event; a 4xx or 5xx body
// is an error envelope.
func (e *ResponseExtraction) ProcessBlocking(statusCode int, raw []byte) {
	if statusCode >= http.StatusBadRequest {
		e.httpError(statusCode, raw)
		return
	}
	if statusCode < 200 || statusCode > 299 {
		e.notes.Addf(e.ctx, "skipped body: unexpected status %d", statusCode)
		return
	}
	r, ok := e.parse("body", raw, extract.MaxBodyBytes)
	if !ok {
		return
	}
	var status extract.TerminalStatus
	switch r.Get("status").String() {
	case string(extract.TerminalCompleted):
		status = extract.TerminalCompleted
	case string(extract.TerminalIncomplete):
		status = extract.TerminalIncomplete
	case string(extract.TerminalFailed):
		status = extract.TerminalFailed
	}
	e.finish(status, r)
}

// Outcome returns the response ID, terminal status, and provider error
// observed so far.
func (e *ResponseExtraction) Outcome() extract.Outcome {
	return e.outcome
}

// parse validates raw and copies it, so no gjson result aliases the
// caller's buffer.
func (e *ResponseExtraction) parse(what string, raw []byte, limit int) (gjson.Result, bool) {
	if len(raw) > limit {
		e.notes.Addf(e.ctx, "skipped %q: exceeds %d bytes", what, limit)
		return gjson.Result{}, false
	}
	if !gjson.ValidBytes(raw) {
		e.notes.Addf(e.ctx, "skipped %q: invalid JSON (%d bytes)", what, len(raw))
		return gjson.Result{}, false
	}
	return gjson.Parse(string(raw)), true
}

// observeResponseID keeps the first response ID seen and records the prompt
// against it.
func (e *ResponseExtraction) observeResponseID(r gjson.Result) {
	if e.outcome.ResponseID != "" || !r.IsObject() {
		return
	}
	id := r.Get("id").String()
	if id == "" {
		return
	}
	e.outcome.ResponseID = id
	if e.prompt == "" {
		return
	}
	if err := e.rec.RecordPromptUsage(e.ctx, &recorder.PromptUsageRecord{
		InterceptionID: e.interceptionID,
		MsgID:          id,
		Prompt:         e.prompt,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		e.logger.Warn(e.ctx, "failed to record prompt usage", slog.Error(err))
	}
}

// finish handles a terminal response object: it sets the outcome and
// records token usage, model thoughts, and tool usage.
func (e *ResponseExtraction) finish(status extract.TerminalStatus, r gjson.Result) {
	if !r.IsObject() {
		e.notes.Addf(e.ctx, "skipped terminal %q: no response object", status)
		return
	}
	if e.recorded {
		e.notes.Addf(e.ctx, "skipped terminal %q: response already recorded", status)
		return
	}
	e.recorded = true
	// A terminal response after an error event is still recorded, since
	// the usage was consumed, but the earlier error stays the outcome.
	erroredFirst := e.settled
	e.settled = true
	e.observeResponseID(r)

	if !erroredFirst {
		e.outcome.Terminal = extract.Terminal{Status: status}
		switch status {
		case extract.TerminalIncomplete:
			e.outcome.Terminal.Reason = r.Get("incomplete_details.reason").String()
		case extract.TerminalFailed:
			errObj := r.Get("error")
			code := errObj.Get("code").String()
			e.outcome.Terminal.Reason = code
			e.outcome.Err = providerError(failedStatus(code), errObj.Get("message").String(), errObj.Get("type").String(), code, "response failed")
		}
	}

	// Records use the terminal object's own ID and model, as the
	// interceptor does.
	msgID := r.Get("id").String()
	now := time.Now().UTC()
	if u := r.Get("usage"); u.IsObject() {
		e.recordTokenUsage(msgID, r, u, now)
	}
	output := r.Get("output").Array()
	for _, t := range thoughts(output) {
		if err := e.rec.RecordModelThought(e.ctx, &recorder.ModelThoughtRecord{
			InterceptionID: e.interceptionID,
			Content:        t.content,
			Metadata:       recorder.Metadata{"source": t.source},
			CreatedAt:      now,
		}); err != nil {
			e.logger.Warn(e.ctx, "failed to record model thought", slog.Error(err))
		}
	}
	for _, call := range toolCalls(output) {
		call.InterceptionID = e.interceptionID
		call.MsgID = msgID
		call.CreatedAt = now
		if err := e.rec.RecordToolUsage(e.ctx, call); err != nil {
			e.logger.Warn(e.ctx, "failed to record tool usage", slog.Error(err), slog.F("tool", call.Tool))
		}
	}
}

// streamError handles an "error" event. The fields may be top level or
// nested under "error", optionally with an HTTP "status" (WebSocket mode).
func (e *ResponseExtraction) streamError(ev gjson.Result) {
	if e.settled {
		e.notes.Addf(e.ctx, "skipped error event: response already ended")
		return
	}
	e.settled = true
	obj := ev
	if nested := ev.Get("error"); nested.IsObject() {
		obj = nested
	}
	status := int(ev.Get("status").Int())
	if status < http.StatusBadRequest || status > 599 {
		status = 0
	}
	code := obj.Get("code").String()
	e.outcome.Terminal = extract.Terminal{Status: extract.TerminalFailed, Reason: code}
	e.outcome.Err = providerError(status, obj.Get("message").String(), obj.Get("type").String(), code, "upstream stream error")
}

// httpError handles a 4xx or 5xx body, normally {"error": {...}}.
func (e *ResponseExtraction) httpError(status int, raw []byte) {
	if e.settled {
		e.notes.Addf(e.ctx, "skipped error body (status %d): response already ended", status)
		return
	}
	e.settled = true
	var errObj gjson.Result
	switch {
	case len(raw) == 0:
	case !gjson.ValidBytes(raw):
		e.notes.Addf(e.ctx, "error body is not valid JSON (%d bytes)", len(raw))
	default:
		errObj = gjson.Get(string(raw), "error")
	}
	code := errObj.Get("code").String()
	e.outcome.Terminal = extract.Terminal{Status: extract.TerminalFailed, Reason: code}
	e.outcome.Err = providerError(status, errObj.Get("message").String(), errObj.Get("type").String(), code, http.StatusText(status))
}

// failedStatus maps a response.failed error code, which carries no HTTP
// status, to the status the equivalent HTTP error would have, so the error
// categorizes by what failed. The WebSocket relay uses the same mapping.
// A missing code maps to 0, which categorizes as unknown.
func failedStatus(code string) int {
	switch code {
	case intercept.OpenAIErrCodeRateLimit:
		return http.StatusTooManyRequests
	case intercept.OpenAIErrCodeServer:
		return http.StatusInternalServerError
	case "":
		return 0
	default:
		return http.StatusBadRequest
	}
}

// providerError builds the OpenAI-shaped error the interceptors return, so
// provider.OpenAI categorizes it by status code. Mid-stream "error" events
// carry no HTTP status unless the event reports one; status 0 categorizes
// as unknown, as the SDK stream errors the interceptor returns today do.
func providerError(status int, msg, errType, code, fallbackMsg string) *intercept.ResponseError {
	if msg == "" {
		msg = fallbackMsg
	}
	if msg == "" {
		msg = "upstream error"
	}
	if errType == "" {
		errType = intercept.OpenAIErrTypeError
	}
	return intercept.NewResponseError(msg, errType, code, status, 0)
}

// recordTokenUsage applies the interceptor's math: input tokens include
// cache reads and writes, which are reported separately.
func (e *ResponseExtraction) recordTokenUsage(msgID string, r, u gjson.Result, now time.Time) {
	input := u.Get("input_tokens").Int()
	cached := u.Get("input_tokens_details.cached_tokens").Int()
	cacheWrite := u.Get("input_tokens_details.cache_write_tokens").Int()
	var metadata recorder.Metadata
	if tier := r.Get("service_tier").String(); tier != "" {
		metadata = recorder.Metadata{recorder.MetadataKeyServiceTier: tier}
	}
	if err := e.rec.RecordTokenUsage(e.ctx, &recorder.TokenUsageRecord{
		InterceptionID:        e.interceptionID,
		MsgID:                 msgID,
		ProviderModel:         r.Get("model").String(),
		Input:                 max(0, input-cached-cacheWrite),
		Output:                u.Get("output_tokens").Int(),
		CacheReadInputTokens:  cached,
		CacheWriteInputTokens: cacheWrite,
		ExtraTokenTypes: map[string]int64{
			"output_reasoning": u.Get("output_tokens_details.reasoning_tokens").Int(),
			"total_tokens":     u.Get("total_tokens").Int(),
		},
		Metadata:  metadata,
		CreatedAt: now,
	}); err != nil {
		e.logger.Warn(e.ctx, "failed to record token usage", slog.Error(err))
	}
}

// toolCalls returns tool usage records for the tool call output items,
// without interception, message, or time fields.
func toolCalls(items []gjson.Result) []*recorder.ToolUsageRecord {
	var calls []*recorder.ToolUsageRecord
	for _, item := range items {
		typ := item.Get("type").String()
		var args recorder.ToolArgs
		switch {
		case typ == itemFunctionCall:
			args = functionCallArgs(item.Get("arguments"))
		case typ == itemCustomToolCall:
			args = item.Get("input").String()
		case hostedToolItems[typ]:
		default:
			continue
		}
		// Hosted tools usually have no name, so fall back to the type.
		name := item.Get("name").String()
		if name == "" {
			name = typ
		}
		calls = append(calls, &recorder.ToolUsageRecord{
			// ToolCallID (call_id) is empty for hosted tools the provider
			// executes itself.
			ItemID:     item.Get("id").String(),
			ToolCallID: item.Get("call_id").String(),
			Tool:       name,
			Args:       args,
			Injected:   false,
		})
	}
	return calls
}

// functionCallArgs decodes JSON-string arguments, keeping the trimmed string
// when it is not valid JSON. Non-string arguments yield "", as in the
// interceptor.
func functionCallArgs(v gjson.Result) recorder.ToolArgs {
	if v.Type != gjson.String {
		return ""
	}
	trimmed := strings.TrimSpace(v.Str)
	if trimmed == "" {
		return trimmed
	}
	var args recorder.ToolArgs
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return trimmed
	}
	return args
}

type thought struct {
	content string
	// source is one of the recorder.ThoughtSource* constants.
	source string
}

// thoughts returns reasoning summaries and assistant commentary messages
// (message items with "phase": "commentary").
func thoughts(items []gjson.Result) []thought {
	var out []thought
	for _, item := range items {
		switch item.Get("type").String() {
		case itemReasoning:
			for _, s := range item.Get("summary").Array() {
				if text := s.Get("text").String(); text != "" {
					out = append(out, thought{content: text, source: recorder.ThoughtSourceReasoningSummary})
				}
			}
		case itemMessage:
			if item.Get("role").String() != "assistant" || item.Get("phase").String() != "commentary" {
				continue
			}
			for _, part := range item.Get("content").Array() {
				if part.Get("type").String() != "output_text" {
					continue
				}
				if text := part.Get("text").String(); text != "" {
					out = append(out, thought{content: text, source: recorder.ThoughtSourceCommentary})
				}
			}
		}
	}
	return out
}
