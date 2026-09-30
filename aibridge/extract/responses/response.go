package responses

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/coder/coder/v2/aibridge/extract"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// Responses API event and item type names.
const (
	eventCreated        = "response.created"
	eventInProgress     = "response.in_progress"
	eventQueued         = "response.queued"
	eventOutputItemDone = "response.output_item.done"
	eventCompleted      = "response.completed"
	eventIncomplete     = "response.incomplete"
	eventFailed         = "response.failed"
	eventError          = "error"

	itemFunctionCall   = "function_call"
	itemCustomToolCall = "custom_tool_call"
	itemReasoning      = "reasoning"
	itemMessage        = "message"
)

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

// ResponseExtraction accumulates facts for one Responses API response. It
// accepts SSE events, WebSocket frames (raw event JSON with an empty event
// type), and complete bodies. It is not safe for concurrent use.
//
// Token usage, tool calls, and thoughts come from the terminal response
// object (response.completed, response.incomplete, response.failed, or a
// complete body), as in the interceptor. When a stream ends without a
// terminal event, tool calls and thoughts fall back to the items seen in
// response.output_item.done events.
type ResponseExtraction struct {
	facts extract.ResponseFacts
	// doneItems are output items from response.output_item.done events.
	doneItems []gjson.Result
	// terminalOutput is the output of the terminal response object.
	terminalOutput []gjson.Result
	notes          extract.ParseNotes
}

var _ extract.ResponseExtraction = (*ResponseExtraction)(nil)

// NewResponseExtraction returns an empty extraction for one response.
func NewResponseExtraction() *ResponseExtraction {
	return &ResponseExtraction{}
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
		e.observeResponse(ev.Get("response"))
	case eventOutputItemDone:
		if item := ev.Get("item"); item.IsObject() {
			e.doneItems = append(e.doneItems, item)
		}
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

// OnBody observes a complete body. A 2xx body is a response object and is
// treated like a terminal event; a 4xx or 5xx body is an error envelope.
func (e *ResponseExtraction) OnBody(statusCode int, raw []byte) {
	if statusCode >= http.StatusBadRequest {
		e.httpError(statusCode, raw)
		return
	}
	if statusCode < 200 || statusCode > 299 {
		e.notes.Addf("skipped body: unexpected status %d", statusCode)
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

// Result returns the facts gathered so far.
func (e *ResponseExtraction) Result() extract.ResponseFacts {
	facts := e.facts
	if facts.Usage != nil {
		u := *facts.Usage
		u.Extra = maps.Clone(u.Extra)
		facts.Usage = &u
	}
	items := e.terminalOutput
	if len(items) == 0 {
		items = e.doneItems
	}
	facts.ToolCalls = toolCalls(items)
	facts.Thoughts = thoughts(items)
	facts.ParseNotes = e.notes.List()
	return facts
}

// parse validates raw and copies it, so no gjson result aliases the
// caller's buffer.
func (e *ResponseExtraction) parse(what string, raw []byte, limit int) (gjson.Result, bool) {
	if len(raw) > limit {
		e.notes.Addf("skipped %q: exceeds %d bytes", what, limit)
		return gjson.Result{}, false
	}
	if !gjson.ValidBytes(raw) {
		e.notes.Addf("skipped %q: invalid JSON (%d bytes)", what, len(raw))
		return gjson.Result{}, false
	}
	return gjson.Parse(string(raw)), true
}

// observeResponse records the response ID (first seen) and model (last
// seen) of a response object.
func (e *ResponseExtraction) observeResponse(r gjson.Result) {
	if !r.IsObject() {
		return
	}
	if id := r.Get("id").String(); id != "" && e.facts.ResponseID == "" {
		e.facts.ResponseID = id
	}
	if model := r.Get("model").String(); model != "" {
		e.facts.Model = model
	}
}

func (e *ResponseExtraction) finish(status extract.TerminalStatus, r gjson.Result) {
	if !r.IsObject() {
		e.notes.Addf("skipped terminal %q: no response object", status)
		return
	}
	e.observeResponse(r)
	e.facts.Terminal = extract.Terminal{Status: status}
	e.facts.ServiceTier = r.Get("service_tier").String()
	if u := r.Get("usage"); u.IsObject() {
		e.facts.Usage = tokenUsage(u)
	}
	if out := r.Get("output"); out.IsArray() {
		e.terminalOutput = out.Array()
	}

	switch status {
	case extract.TerminalIncomplete:
		e.facts.Terminal.Reason = r.Get("incomplete_details.reason").String()
	case extract.TerminalFailed:
		errObj := r.Get("error")
		code := errObj.Get("code").String()
		e.facts.Terminal.Reason = code
		e.facts.Err = providerError(0, errObj.Get("message").String(), errObj.Get("type").String(), code, "response failed")
	}
}

// streamError handles an "error" event. The fields may be top level or
// nested under "error", optionally with an HTTP "status" (WebSocket mode).
func (e *ResponseExtraction) streamError(ev gjson.Result) {
	obj := ev
	if nested := ev.Get("error"); nested.IsObject() {
		obj = nested
	}
	status := int(ev.Get("status").Int())
	if status < http.StatusBadRequest || status > 599 {
		status = 0
	}
	code := obj.Get("code").String()
	e.facts.Terminal = extract.Terminal{Status: extract.TerminalFailed, Reason: code}
	e.facts.Err = providerError(status, obj.Get("message").String(), obj.Get("type").String(), code, "upstream stream error")
}

// httpError handles a 4xx or 5xx body, normally {"error": {...}}.
func (e *ResponseExtraction) httpError(status int, raw []byte) {
	var errObj gjson.Result
	switch {
	case len(raw) == 0:
	case !gjson.ValidBytes(raw):
		e.notes.Addf("error body is not valid JSON (%d bytes)", len(raw))
	default:
		errObj = gjson.Get(string(raw), "error")
	}
	code := errObj.Get("code").String()
	e.facts.Terminal = extract.Terminal{Status: extract.TerminalFailed, Reason: code}
	e.facts.Err = providerError(status, errObj.Get("message").String(), errObj.Get("type").String(), code, http.StatusText(status))
}

// providerError builds the OpenAI-shaped error the interceptors return, so
// provider.OpenAI categorizes it by status code. Mid-stream errors carry no
// HTTP status unless the event reports one; status 0 categorizes as
// unknown, as the SDK stream errors the interceptor returns today do.
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

// tokenUsage applies the interceptor's math: input tokens include cache
// reads and writes, which are reported separately.
func tokenUsage(u gjson.Result) *extract.TokenUsage {
	input := u.Get("input_tokens").Int()
	cached := u.Get("input_tokens_details.cached_tokens").Int()
	cacheWrite := u.Get("input_tokens_details.cache_write_tokens").Int()
	return &extract.TokenUsage{
		Input:           max(0, input-cached-cacheWrite),
		Output:          u.Get("output_tokens").Int(),
		CacheReadInput:  cached,
		CacheWriteInput: cacheWrite,
		Extra: map[string]int64{
			"output_reasoning": u.Get("output_tokens_details.reasoning_tokens").Int(),
			"total_tokens":     u.Get("total_tokens").Int(),
		},
	}
}

func toolCalls(items []gjson.Result) []extract.ToolCall {
	var calls []extract.ToolCall
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
		calls = append(calls, extract.ToolCall{
			ItemID: item.Get("id").String(),
			CallID: item.Get("call_id").String(),
			Name:   name,
			Args:   args,
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

// thoughts returns reasoning summaries and assistant commentary messages
// (message items with "phase": "commentary").
func thoughts(items []gjson.Result) []extract.Thought {
	var out []extract.Thought
	for _, item := range items {
		switch item.Get("type").String() {
		case itemReasoning:
			for _, s := range item.Get("summary").Array() {
				if text := s.Get("text").String(); text != "" {
					out = append(out, extract.Thought{Content: text, Source: recorder.ThoughtSourceReasoningSummary})
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
					out = append(out, extract.Thought{Content: text, Source: recorder.ThoughtSourceCommentary})
				}
			}
		}
	}
	return out
}
