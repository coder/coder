package googleopenai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Gemini's OpenAI-compatible endpoint emits thought text inline in the
// regular content field, marked with extra_content.google.thought and
// wrapped in <thought>...</thought>. OpenAI-compatible clients expect
// reasoning in the reasoning_content field instead, so these helpers
// rewrite responses at the transport boundary.
const (
	thoughtOpenMarker  = "<thought>"
	thoughtCloseMarker = "</thought>"
)

// RewriteThoughtResponse rewrites a Gemini OpenAI-compatible chat
// completions response so thought output surfaces as reasoning_content.
// Streaming bodies are rewritten incrementally; JSON bodies are rewritten
// in place. Responses without thought output pass through unchanged.
func RewriteThoughtResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil || resp.StatusCode != http.StatusOK {
		return
	}
	contentType := resp.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "text/event-stream"):
		resp.Body = &thoughtStreamBody{
			reader:    bufio.NewReader(resp.Body),
			closer:    resp.Body,
			inThought: map[int]bool{},
		}
	case strings.HasPrefix(contentType, "application/json"):
		body, err := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if err != nil || closeErr != nil {
			// The client re-reads the replaced body, so surface read
			// failures there instead of swallowing them here.
			resp.Body = io.NopCloser(&errorReader{err: errors.Join(err, closeErr)})
			return
		}
		rewritten := RewriteThoughtCompletion(body)
		resp.Body = io.NopCloser(bytes.NewReader(rewritten))
		resp.ContentLength = int64(len(rewritten))
		resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	}
}

type errorReader struct{ err error }

func (r *errorReader) Read([]byte) (int, error) { return 0, r.err }

// RewriteThoughtCompletion rewrites a non-streaming chat completion,
// splitting <thought>-marked message content into reasoning_content and
// the visible answer.
func RewriteThoughtCompletion(body []byte) []byte {
	choices := gjson.GetBytes(body, "choices")
	if !choices.IsArray() {
		return body
	}
	out := body
	for index, choice := range choices.Array() {
		content := choice.Get("message.content")
		// The thought metadata gates the rewrite so an answer that
		// legitimately begins with the marker text is left alone; Gemini sets
		// it on every message that carries thought output.
		if content.Type != gjson.String || !strings.HasPrefix(content.Str, thoughtOpenMarker) ||
			!choice.Get("message.extra_content.google.thought").Bool() {
			continue
		}
		reasoning := content.Str[len(thoughtOpenMarker):]
		answer := ""
		if markerIndex := strings.Index(reasoning, thoughtCloseMarker); markerIndex >= 0 {
			answer = reasoning[markerIndex+len(thoughtCloseMarker):]
			reasoning = reasoning[:markerIndex]
		}
		prefix := "choices." + strconv.Itoa(index) + ".message."
		updated, err := sjson.SetBytes(out, prefix+"reasoning_content", reasoning)
		if err != nil {
			return body
		}
		updated, err = sjson.SetBytes(updated, prefix+"content", answer)
		if err != nil {
			return body
		}
		out = updated
	}
	return out
}

// thoughtStreamBody rewrites SSE chat completion chunks line by line as the
// client reads them, preserving streaming latency.
type thoughtStreamBody struct {
	reader  *bufio.Reader
	closer  io.Closer
	pending []byte
	err     error
	// inThought tracks, per choice index, whether the previous delta was
	// thought output, so the <thought> and </thought> markers can be
	// stripped at the transitions.
	inThought map[int]bool
}

func (b *thoughtStreamBody) Read(p []byte) (int, error) {
	for len(b.pending) == 0 {
		if b.err != nil {
			return 0, b.err
		}
		line, err := b.reader.ReadBytes('\n')
		if len(line) > 0 {
			b.pending = b.rewriteLine(line)
		}
		b.err = err
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

func (b *thoughtStreamBody) Close() error {
	return b.closer.Close()
}

var streamDataPrefix = []byte("data: ")

func (b *thoughtStreamBody) rewriteLine(line []byte) []byte {
	payload := bytes.TrimPrefix(line, streamDataPrefix)
	if len(payload) == len(line) || !bytes.HasPrefix(payload, []byte("{")) {
		return line
	}
	suffixLength := len(payload) - len(bytes.TrimRight(payload, "\r\n"))
	suffix := payload[len(payload)-suffixLength:]
	payload = payload[:len(payload)-suffixLength]

	choices := gjson.GetBytes(payload, "choices")
	if !choices.IsArray() {
		return line
	}
	if errPayload := functionCallFilterErrorPayload(choices); errPayload != nil {
		return assembleDataLine(errPayload, suffix)
	}
	out := payload
	for index, choice := range choices.Array() {
		delta := choice.Get("delta")
		if !delta.Exists() {
			continue
		}
		deltaPath := "choices." + strconv.Itoa(index) + ".delta"
		content := delta.Get("content")
		if delta.Get("extra_content.google.thought").Bool() {
			text := content.Str
			if !b.inThought[index] {
				text = strings.TrimPrefix(text, thoughtOpenMarker)
				b.inThought[index] = true
			}
			updated, err := sjson.SetBytes(out, deltaPath+".reasoning_content", text)
			if err != nil {
				return line
			}
			updated, err = sjson.DeleteBytes(updated, deltaPath+".content")
			if err != nil {
				return line
			}
			out = updated
			continue
		}
		if !b.inThought[index] {
			continue
		}
		if content.Type == gjson.String && content.Str != "" {
			b.inThought[index] = false
			if strings.HasPrefix(content.Str, thoughtCloseMarker) {
				updated, err := sjson.SetBytes(out, deltaPath+".content", strings.TrimPrefix(content.Str, thoughtCloseMarker))
				if err != nil {
					return line
				}
				out = updated
			}
		} else if delta.Get("tool_calls").Exists() {
			b.inThought[index] = false
		}
	}
	return assembleDataLine(out, suffix)
}

func assembleDataLine(payload, suffix []byte) []byte {
	rewritten := make([]byte, 0, len(streamDataPrefix)+len(payload)+len(suffix))
	rewritten = append(rewritten, streamDataPrefix...)
	rewritten = append(rewritten, payload...)
	rewritten = append(rewritten, suffix...)
	return rewritten
}

// functionCallFilterFinishReasonPrefix marks Gemini's server-side
// function-call filter on the OpenAI-compatible endpoint, observed as
// finish_reason "function_call_filter: MALFORMED_FUNCTION_CALL": the
// rejected call is dropped and the stream ends cleanly with no answer
// output, so clients would otherwise treat the empty step as a normal
// completion.
const functionCallFilterFinishReasonPrefix = "function_call_filter"

type streamErrorEvent struct {
	Error streamErrorDetail `json:"error"`
}

type streamErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// functionCallFilterErrorPayload converts a chunk carrying a
// function-call-filter finish reason into an OpenAI-style SSE error
// event so the stream fails loudly and the step can be retried.
func functionCallFilterErrorPayload(choices gjson.Result) []byte {
	for _, choice := range choices.Array() {
		reason := choice.Get("finish_reason")
		if reason.Type != gjson.String || !strings.HasPrefix(reason.Str, functionCallFilterFinishReasonPrefix) {
			continue
		}
		payload, err := json.Marshal(streamErrorEvent{Error: streamErrorDetail{
			Message: "gemini dropped the model's generated function call (finish_reason " + strconv.Quote(reason.Str) + ")",
			Type:    "invalid_response_error",
			Code:    "malformed_function_call",
		}})
		if err != nil {
			return nil
		}
		return payload
	}
	return nil
}
