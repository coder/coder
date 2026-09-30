// Package responses extracts recording facts from OpenAI Responses API
// traffic. It mirrors what the aibridge/intercept/responses interceptors
// record, reading raw JSON with gjson instead of SDK types so the same code
// serves SSE, complete-body, and WebSocket transports.
package responses

import (
	"strings"

	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/extract"
)

// RequestExtractor extracts facts from Responses API request bodies.
type RequestExtractor struct{}

var _ extract.RequestExtractor = RequestExtractor{}

// ExtractRequest reads the model, streaming flag, last user prompt, and
// function_call_output correlation from a request body. On an unexpected
// input shape it returns the facts it could read and an error describing
// the shape.
func (RequestExtractor) ExtractRequest(body []byte) (extract.RequestFacts, error) {
	if !gjson.ValidBytes(body) {
		return extract.RequestFacts{}, xerrors.New("request body is not valid JSON")
	}
	root := gjson.ParseBytes(body)
	input := root.Get("input")
	facts := extract.RequestFacts{
		Model:                 root.Get("model").String(),
		Streaming:             root.Get("stream").Bool(),
		CorrelatingToolCallID: correlatingToolCallID(input),
	}
	prompt, err := lastUserPrompt(input)
	facts.Prompt = prompt
	return facts, err
}

// correlatingToolCallID returns the call_id of a trailing
// function_call_output input item, or "".
func correlatingToolCallID(input gjson.Result) string {
	if !input.IsArray() {
		return ""
	}
	items := input.Array()
	if len(items) == 0 {
		return ""
	}
	last := items[len(items)-1]
	if last.Get("type").String() != "function_call_output" {
		return ""
	}
	return last.Get("call_id").String()
}

// lastUserPrompt returns the string input, or the input_text parts of the
// last input item when it has the user role, joined with newlines. It
// returns "" when the request carries no prompt, for example an agentic
// loop iteration that ends in a tool result.
func lastUserPrompt(input gjson.Result) (string, error) {
	if !input.Exists() || input.Type == gjson.Null {
		return "", nil
	}
	if input.Type == gjson.String {
		return input.Str, nil
	}
	if !input.IsArray() {
		return "", xerrors.Errorf("unexpected input type: %s", input.Type)
	}
	items := input.Array()
	if len(items) == 0 {
		return "", nil
	}
	last := items[len(items)-1]
	if last.Get("role").Str != "user" {
		return "", nil
	}

	content := last.Get("content")
	switch {
	case !content.Exists() || content.Type == gjson.Null:
		return "", nil
	case content.Type == gjson.String:
		return content.Str, nil
	case !content.IsArray():
		return "", xerrors.Errorf("unexpected input content type: %s", content.Type)
	}

	var parts []string
	for _, c := range content.Array() {
		// Skip non-text parts such as images or files.
		if c.Get("type").Str != "input_text" {
			continue
		}
		if text := c.Get("text"); text.Type == gjson.String {
			parts = append(parts, text.Str)
		}
	}
	return strings.Join(parts, "\n"), nil
}
