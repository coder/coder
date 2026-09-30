package responses_test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/extract"
	"github.com/coder/coder/v2/aibridge/extract/responses"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// wsFrames is a Responses stream as WebSocket frames: raw event JSON with no
// SSE framing.
var wsFrames = []string{
	`{"type":"response.created","response":{"id":"resp_ws","model":"gpt-5","status":"in_progress","output":[]}}`,
	`{"type":"response.output_text.delta","item_id":"msg_1","delta":"Hel"}`,
	`{"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"add","arguments":"{\"a\":1}"}}`,
	`{"type":"response.completed","response":{"id":"resp_ws","model":"gpt-5-2025","status":"completed","service_tier":"default",` +
		`"output":[{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"thinking"}]},` +
		`{"id":"fc_1","type":"function_call","call_id":"call_1","name":"add","arguments":"{\"a\":1}"}],` +
		`"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":60,"cache_write_tokens":10},"output_tokens":20,"output_tokens_details":{"reasoning_tokens":5},"total_tokens":120}}}`,
}

// TestResponseExtractionTransports feeds the same events as WebSocket
// frames and as an SSE stream split into single bytes, and requires the
// same facts from both.
func TestResponseExtractionTransports(t *testing.T) {
	t.Parallel()

	want := extract.ResponseFacts{
		ResponseID:  "resp_ws",
		Model:       "gpt-5-2025",
		ServiceTier: "default",
		Usage: &extract.TokenUsage{
			Input:           30,
			Output:          20,
			CacheReadInput:  60,
			CacheWriteInput: 10,
			Extra:           map[string]int64{"output_reasoning": 5, "total_tokens": 120},
		},
		ToolCalls: []extract.ToolCall{{ItemID: "fc_1", CallID: "call_1", Name: "add", Args: map[string]any{"a": float64(1)}}},
		Thoughts:  []extract.Thought{{Content: "thinking", Source: recorder.ThoughtSourceReasoningSummary}},
		Terminal:  extract.Terminal{Status: extract.TerminalCompleted},
	}

	ws := responses.NewResponseExtraction()
	for _, frame := range wsFrames {
		ws.OnEvent("", []byte(frame))
	}
	require.Equal(t, want, ws.Result())

	// CRLF line endings, comments, and a data field split over two lines.
	var stream strings.Builder
	_, _ = stream.WriteString(": keep-alive\r\n\r\n")
	for _, frame := range wsFrames {
		// Split after the first comma, which sits between JSON tokens, so
		// the newline that joins data lines is only whitespace.
		i := strings.Index(frame, ",") + 1
		_, _ = fmt.Fprintf(&stream, "event: ignored\r\ndata: %s\r\ndata:%s\r\n\r\n", frame[:i], frame[i:])
	}
	sse := extract.NewSSEStream(responses.NewResponseExtraction())
	for _, b := range []byte(stream.String()) {
		_, _ = sse.Write([]byte{b})
	}
	require.Equal(t, want, sse.Result())
}

// TestResponseExtractionTerminalErrors checks that provider-reported
// failures become provider-shaped errors that the OpenAI provider
// categorizes as the interceptor path does today.
func TestResponseExtractionTerminalErrors(t *testing.T) {
	t.Parallel()

	// sdkErr is the error the interceptor returns today for an HTTP error
	// response.
	sdkErr := func(status int) error {
		req, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", nil)
		return &openai.Error{StatusCode: status, Request: req, Response: &http.Response{StatusCode: status, Header: http.Header{}}}
	}

	cases := []struct {
		name      string
		feed      func(ext *responses.ResponseExtraction)
		wantType  recorder.ErrorType
		todayErr  error // when set, wantType must match today's category
		wantMsg   string
		wantCode  string
		wantNotes bool
	}{
		{
			name:     "http_400",
			feed:     body(400, `{"error":{"message":"too long","type":"invalid_request_error","code":"context_length_exceeded"}}`),
			wantType: recorder.ErrorTypeBadRequest, todayErr: sdkErr(400), wantMsg: "too long", wantCode: "context_length_exceeded",
		},
		{
			name:     "http_401",
			feed:     body(401, `{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`),
			wantType: recorder.ErrorTypeUnauthorized, todayErr: sdkErr(401), wantMsg: "bad key", wantCode: "invalid_api_key",
		},
		{
			name:     "http_429",
			feed:     body(429, `{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limit_exceeded"}}`),
			wantType: recorder.ErrorTypeRateLimited, todayErr: sdkErr(429), wantMsg: "slow down", wantCode: "rate_limit_exceeded",
		},
		{
			name:     "http_503",
			feed:     body(503, `{"error":{"message":"overloaded","type":"server_error"}}`),
			wantType: recorder.ErrorTypeOverloaded, todayErr: sdkErr(503), wantMsg: "overloaded",
		},
		{
			name:     "http_500_not_json",
			feed:     body(500, `<html>oops</html>`),
			wantType: recorder.ErrorTypeServerError, todayErr: sdkErr(500), wantMsg: "Internal Server Error", wantNotes: true,
		},
		{
			// Statusless mid-stream errors categorize as unknown, like the
			// SDK stream errors the interceptor returns today.
			name:     "sse_error_event",
			feed:     event(`{"type":"error","code":"ERR_SOMETHING","message":"Something went wrong","param":null}`),
			wantType: recorder.ErrorTypeUnknown, wantMsg: "Something went wrong", wantCode: "ERR_SOMETHING",
		},
		{
			name:     "nested_error_event",
			feed:     event(`{"type":"error","error":{"type":"invalid_request_error","code":null,"message":"Conversation not found."}}`),
			wantType: recorder.ErrorTypeUnknown, wantMsg: "Conversation not found.",
		},
		{
			name:     "websocket_error_with_status",
			feed:     event(`{"type":"error","status":429,"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down"}}`),
			wantType: recorder.ErrorTypeRateLimited, wantMsg: "slow down", wantCode: "rate_limit_exceeded",
		},
		{
			name:     "response_failed",
			feed:     event(`{"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"server_error","message":"The model failed."},"output":[]}}`),
			wantType: recorder.ErrorTypeUnknown, wantMsg: "The model failed.", wantCode: "server_error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := responses.NewResponseExtraction()
			tc.feed(ext)
			facts := ext.Result()

			require.Equal(t, extract.TerminalFailed, facts.Terminal.Status)
			require.Equal(t, tc.wantCode, facts.Terminal.Reason)
			var respErr *intercept.ResponseError
			require.ErrorAs(t, facts.Err, &respErr)
			require.Equal(t, tc.wantMsg, respErr.ErrorObject.Message)
			require.Equal(t, tc.wantCode, respErr.ErrorObject.Code)
			require.Equal(t, tc.wantNotes, len(facts.ParseNotes) > 0, "notes: %v", facts.ParseNotes)

			gotType, _ := interceptionerror.Categorize((*provider.OpenAI)(nil), facts.Err)
			require.Equal(t, tc.wantType, gotType)
			if tc.todayErr != nil {
				todayType, _ := interceptionerror.Categorize((*provider.OpenAI)(nil), tc.todayErr)
				require.Equal(t, todayType, gotType, "categorized differently from the interceptor path")
			}
		})
	}
}

// TestResponseExtractionFailOpen checks that malformed, oversized, and
// truncated input yields partial facts and parse notes, never a provider
// error.
func TestResponseExtractionFailOpen(t *testing.T) {
	t.Parallel()

	completed := []byte(wsFrames[len(wsFrames)-1])
	gzipped := func(b []byte) []byte {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(b)
		_ = zw.Close()
		return buf.Bytes()
	}
	oversized := `{"pad":"` + strings.Repeat("x", extract.MaxEventBytes) + `"}`

	cases := []struct {
		name string
		run  func() extract.ResponseFacts
		// wantUsage is whether the completed response was still extracted.
		wantUsage bool
		wantID    string
		// clean is set for the one valid input, which must add no notes.
		clean bool
	}{
		{
			name: "malformed_event",
			run: func() extract.ResponseFacts {
				ext := responses.NewResponseExtraction()
				ext.OnEvent("response.output_text.delta", []byte(`{ "wrong format`))
				ext.OnEvent("", completed)
				return ext.Result()
			},
			wantUsage: true, wantID: "resp_ws",
		},
		{
			name: "oversized_frame",
			run: func() extract.ResponseFacts {
				ext := responses.NewResponseExtraction()
				ext.OnEvent("", []byte(oversized))
				ext.OnEvent("", completed)
				return ext.Result()
			},
			wantUsage: true, wantID: "resp_ws",
		},
		{
			name: "oversized_sse_event",
			run: func() extract.ResponseFacts {
				sse := extract.NewSSEStream(responses.NewResponseExtraction())
				// The second data line belongs to the skipped event and must
				// not be dispatched.
				smuggled := `{"type":"response.created","response":{"id":"resp_smuggled"}}`
				_, _ = sse.Write([]byte("event: big\ndata: " + oversized + "\ndata: " + smuggled + "\n\n"))
				_, _ = sse.Write([]byte("data: " + string(completed) + "\n\n"))
				return sse.Result()
			},
			wantUsage: true, wantID: "resp_ws",
		},
		{
			// A stream cut mid-event keeps what the earlier events said,
			// including tool calls from finished output items.
			name: "truncated_stream",
			run: func() extract.ResponseFacts {
				sse := extract.NewSSEStream(responses.NewResponseExtraction())
				for _, frame := range wsFrames[:3] {
					_, _ = sse.Write([]byte("data: " + frame + "\n\n"))
				}
				_, _ = sse.Write([]byte("data: " + string(completed[:40])))
				facts := sse.Result()
				require.Len(t, facts.ToolCalls, 1)
				require.Equal(t, extract.TerminalNone, facts.Terminal.Status)
				return facts
			},
			wantID: "resp_ws",
		},
		{
			name: "oversized_body",
			run: func() extract.ResponseFacts {
				big := []byte(`{"pad":"` + strings.Repeat("x", extract.MaxBodyBytes) + `"}`)
				return extract.FromBody(responses.NewResponseExtraction(), http.StatusOK, "", bytes.NewReader(big))
			},
		},
		{
			name: "gzip_body",
			run: func() extract.ResponseFacts {
				body := gzipped([]byte(`{"id":"resp_gz","status":"completed","usage":{"input_tokens":1}}`))
				return extract.FromBody(responses.NewResponseExtraction(), http.StatusOK, "gzip", bytes.NewReader(body))
			},
			wantUsage: true, wantID: "resp_gz", clean: true,
		},
		{
			name: "invalid_gzip_body",
			run: func() extract.ResponseFacts {
				return extract.FromBody(responses.NewResponseExtraction(), http.StatusOK, "gzip", bytes.NewReader(completed))
			},
		},
		{
			name: "truncated_gzip_body",
			run: func() extract.ResponseFacts {
				body := gzipped(completed)
				return extract.FromBody(responses.NewResponseExtraction(), http.StatusOK, "gzip", bytes.NewReader(body[:len(body)/2]))
			},
		},
		{
			name: "unsupported_encoding",
			run: func() extract.ResponseFacts {
				return extract.FromBody(responses.NewResponseExtraction(), http.StatusOK, "br", bytes.NewReader(completed))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			facts := tc.run()
			require.Equal(t, tc.clean, len(facts.ParseNotes) == 0, "notes: %v", facts.ParseNotes)
			require.NoError(t, facts.Err)
			require.Equal(t, tc.wantUsage, facts.Usage != nil)
			require.Equal(t, tc.wantID, facts.ResponseID)
		})
	}
}

// TestResponseExtractionIncomplete checks that an incomplete response keeps
// its reason and usage.
func TestResponseExtractionIncomplete(t *testing.T) {
	t.Parallel()

	ext := responses.NewResponseExtraction()
	ext.OnEvent("response.incomplete", []byte(`{"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete",`+
		`"incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}}`))
	facts := ext.Result()
	require.Equal(t, extract.Terminal{Status: extract.TerminalIncomplete, Reason: "max_output_tokens"}, facts.Terminal)
	require.NotNil(t, facts.Usage)
	require.EqualValues(t, 7, facts.Usage.Output)
	require.NoError(t, facts.Err)
}

func body(status int, raw string) func(*responses.ResponseExtraction) {
	return func(ext *responses.ResponseExtraction) {
		_ = extract.FromBody(ext, status, "", strings.NewReader(raw))
	}
}

func event(raw string) func(*responses.ResponseExtraction) {
	return func(ext *responses.ResponseExtraction) { ext.OnEvent("", []byte(raw)) }
}
