package responses_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/extract"
	"github.com/coder/coder/v2/aibridge/extract/responses"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
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
// same records and outcome from both.
func TestResponseExtractionTransports(t *testing.T) {
	t.Parallel()

	wantOutcome := extract.Outcome{ResponseID: "resp_ws", Terminal: extract.Terminal{Status: extract.TerminalCompleted}}
	wantPrompts := []recorder.PromptUsageRecord{{InterceptionID: interceptionID, MsgID: "resp_ws", Prompt: "hi"}}
	wantTokens := []recorder.TokenUsageRecord{{
		InterceptionID:        interceptionID,
		MsgID:                 "resp_ws",
		ProviderModel:         "gpt-5-2025",
		Input:                 30,
		Output:                20,
		CacheReadInputTokens:  60,
		CacheWriteInputTokens: 10,
		ExtraTokenTypes:       map[string]int64{"output_reasoning": 5, "total_tokens": 120},
		Metadata:              recorder.Metadata{recorder.MetadataKeyServiceTier: "default"},
	}}
	wantTools := []recorder.ToolUsageRecord{{
		InterceptionID: interceptionID, MsgID: "resp_ws", ItemID: "fc_1", ToolCallID: "call_1", Tool: "add",
		Args: map[string]any{"a": float64(1)},
	}}
	wantThoughts := []recorder.ModelThoughtRecord{{
		InterceptionID: interceptionID, Content: "thinking",
		Metadata: recorder.Metadata{"source": recorder.ThoughtSourceReasoningSummary},
	}}
	check := func(t *testing.T, h harness) {
		t.Helper()
		require.Equal(t, wantOutcome, h.ext.Outcome())
		require.Equal(t, wantPrompts, testutil.WithoutCreatedAt(h.rec.RecordedPromptUsages()))
		require.Equal(t, wantTokens, testutil.WithoutCreatedAt(h.rec.RecordedTokenUsages()))
		require.Equal(t, wantTools, testutil.WithoutCreatedAt(h.rec.RecordedToolUsages()))
		require.Equal(t, wantThoughts, testutil.WithoutCreatedAt(h.rec.RecordedModelThoughts()))
		require.Empty(t, h.notes.list())
	}

	t.Run("websocket_frames", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, "hi")
		for _, frame := range wsFrames {
			h.ext.OnEvent("", []byte(frame))
		}
		check(t, h)
	})

	t.Run("sse_bytes", func(t *testing.T) {
		t.Parallel()
		// CRLF line endings, comments, and a data field split over two
		// lines.
		var stream strings.Builder
		_, _ = stream.WriteString(": keep-alive\r\n\r\n")
		for _, frame := range wsFrames {
			// Split after the first comma, which sits between JSON tokens,
			// so the newline that joins data lines is only whitespace.
			i := strings.Index(frame, ",") + 1
			_, _ = fmt.Fprintf(&stream, "event: ignored\r\ndata: %s\r\ndata:%s\r\n\r\n", frame[:i], frame[i:])
		}
		h := newHarness(t, "hi")
		sse := extract.NewSSEStream(t.Context(), h.logger, h.ext)
		for _, b := range []byte(stream.String()) {
			_, _ = sse.Write([]byte{b})
		}
		require.NoError(t, sse.Close())
		check(t, h)
	})
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
			feed:     blocking(400, `{"error":{"message":"too long","type":"invalid_request_error","code":"context_length_exceeded"}}`),
			wantType: recorder.ErrorTypeBadRequest, todayErr: sdkErr(400), wantMsg: "too long", wantCode: "context_length_exceeded",
		},
		{
			name:     "http_401",
			feed:     blocking(401, `{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`),
			wantType: recorder.ErrorTypeUnauthorized, todayErr: sdkErr(401), wantMsg: "bad key", wantCode: "invalid_api_key",
		},
		{
			name:     "http_429",
			feed:     blocking(429, `{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limit_exceeded"}}`),
			wantType: recorder.ErrorTypeRateLimited, todayErr: sdkErr(429), wantMsg: "slow down", wantCode: "rate_limit_exceeded",
		},
		{
			name:     "http_503",
			feed:     blocking(503, `{"error":{"message":"overloaded","type":"server_error"}}`),
			wantType: recorder.ErrorTypeOverloaded, todayErr: sdkErr(503), wantMsg: "overloaded",
		},
		{
			name:     "http_500_not_json",
			feed:     blocking(500, `<html>oops</html>`),
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

			h := newHarness(t, "")
			tc.feed(h.ext)
			out := h.ext.Outcome()

			require.Equal(t, extract.TerminalFailed, out.Terminal.Status)
			require.Equal(t, tc.wantCode, out.Terminal.Reason)
			var respErr *intercept.ResponseError
			require.ErrorAs(t, out.Err, &respErr)
			require.Equal(t, tc.wantMsg, respErr.ErrorObject.Message)
			require.Equal(t, tc.wantCode, respErr.ErrorObject.Code)
			require.Equal(t, tc.wantNotes, len(h.notes.list()) > 0, "notes: %v", h.notes.list())

			gotType, _ := interceptionerror.Categorize((*provider.OpenAI)(nil), out.Err)
			require.Equal(t, tc.wantType, gotType)
			if tc.todayErr != nil {
				todayType, _ := interceptionerror.Categorize((*provider.OpenAI)(nil), tc.todayErr)
				require.Equal(t, todayType, gotType, "categorized differently from the interceptor path")
			}
		})
	}
}

// TestResponseExtractionFailOpen checks that malformed, oversized, and
// truncated input records what could be read and logs parse notes, never a
// provider error.
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
	// decodeAndProcess is what a caller does with a blocking body: a decode
	// error is a parse note, and the body is still processed.
	decodeAndProcess := func(h harness, enc string, raw []byte) {
		decoded, err := extract.DecodeBody(enc, raw)
		if err != nil {
			h.notes.add(err.Error())
		}
		h.ext.ProcessBlocking(http.StatusOK, decoded)
	}

	cases := []struct {
		name string
		run  func(h harness)
		// wantUsage is whether the completed response was still recorded.
		wantUsage bool
		wantID    string
		// clean is set for the one valid input, which must add no notes.
		clean bool
	}{
		{
			name: "malformed_event",
			run: func(h harness) {
				h.ext.OnEvent("response.output_text.delta", []byte(`{ "wrong format`))
				h.ext.OnEvent("", completed)
			},
			wantUsage: true, wantID: "resp_ws",
		},
		{
			name: "oversized_frame",
			run: func(h harness) {
				h.ext.OnEvent("", []byte(oversized))
				h.ext.OnEvent("", completed)
			},
			wantUsage: true, wantID: "resp_ws",
		},
		{
			name: "oversized_sse_event",
			run: func(h harness) {
				sse := extract.NewSSEStream(t.Context(), h.logger, h.ext)
				// The second data line belongs to the skipped event and must
				// not be dispatched.
				smuggled := `{"type":"response.created","response":{"id":"resp_smuggled"}}`
				_, _ = sse.Write([]byte("event: big\ndata: " + oversized + "\ndata: " + smuggled + "\n\n"))
				_, _ = sse.Write([]byte("data: " + string(completed) + "\n\n"))
				_ = sse.Close()
			},
			wantUsage: true, wantID: "resp_ws",
		},
		{
			// A stream cut mid-event keeps the response ID and records the
			// prompt, but nothing from the missing terminal event.
			name: "truncated_stream",
			run: func(h harness) {
				sse := extract.NewSSEStream(t.Context(), h.logger, h.ext)
				for _, frame := range wsFrames[:3] {
					_, _ = sse.Write([]byte("data: " + frame + "\n\n"))
				}
				_, _ = sse.Write([]byte("data: " + string(completed[:40])))
				_ = sse.Close()
				require.Equal(t, extract.TerminalNone, h.ext.Outcome().Terminal.Status)
				require.Len(t, h.rec.RecordedPromptUsages(), 1)
				require.Empty(t, h.rec.RecordedToolUsages())
			},
			wantID: "resp_ws",
		},
		{
			name: "oversized_body",
			run: func(h harness) {
				decodeAndProcess(h, "", []byte(`{"pad":"`+strings.Repeat("x", extract.MaxBodyBytes)+`"}`))
			},
		},
		{
			name: "gzip_body",
			run: func(h harness) {
				decodeAndProcess(h, "gzip", gzipped([]byte(`{"id":"resp_gz","status":"completed","usage":{"input_tokens":1}}`)))
			},
			wantUsage: true, wantID: "resp_gz", clean: true,
		},
		{
			name: "invalid_gzip_body",
			run: func(h harness) {
				decodeAndProcess(h, "gzip", completed)
			},
		},
		{
			name: "truncated_gzip_body",
			run: func(h harness) {
				body := gzipped(completed)
				decodeAndProcess(h, "gzip", body[:len(body)/2])
			},
		},
		{
			name: "oversized_gzip_body",
			run: func(h harness) {
				decodeAndProcess(h, "gzip", gzipped([]byte(`{"pad":"`+strings.Repeat("x", extract.MaxBodyBytes)+`"}`)))
			},
		},
		{
			name: "unsupported_encoding",
			run: func(h harness) {
				decodeAndProcess(h, "br", completed)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, "hi")
			tc.run(h)
			out := h.ext.Outcome()
			require.Equal(t, tc.clean, len(h.notes.list()) == 0, "notes: %v", h.notes.list())
			require.NoError(t, out.Err)
			require.Equal(t, tc.wantUsage, len(h.rec.RecordedTokenUsages()) == 1)
			require.Equal(t, tc.wantID, out.ResponseID)
		})
	}
}

// TestResponseExtractionIncomplete checks that an incomplete response keeps
// its reason and records its usage.
func TestResponseExtractionIncomplete(t *testing.T) {
	t.Parallel()

	h := newHarness(t, "")
	h.ext.OnEvent("response.incomplete", []byte(`{"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete",`+
		`"incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}}`))
	out := h.ext.Outcome()
	require.Equal(t, extract.Terminal{Status: extract.TerminalIncomplete, Reason: "max_output_tokens"}, out.Terminal)
	require.NoError(t, out.Err)
	tokens := h.rec.RecordedTokenUsages()
	require.Len(t, tokens, 1)
	require.EqualValues(t, 7, tokens[0].Output)
}

const interceptionID = "intc_1"

// harness is one extraction wired to an in-memory recorder and a logger
// that captures parse notes.
type harness struct {
	ext    *responses.ResponseExtraction
	rec    *testutil.MockRecorder
	logger slog.Logger
	notes  *noteSink
}

func newHarness(t *testing.T, prompt string) harness {
	t.Helper()
	notes := &noteSink{}
	logger := slog.Make(notes)
	rec := &testutil.MockRecorder{}
	return harness{
		ext:    responses.NewResponseExtraction(t.Context(), logger, rec, interceptionID, prompt),
		rec:    rec,
		logger: logger,
		notes:  notes,
	}
}

// noteSink captures parse notes logged by the extractor.
type noteSink struct {
	mu    sync.Mutex
	notes []string
}

func (s *noteSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = append(s.notes, e.Message)
}

func (*noteSink) Sync() {}

func (s *noteSink) add(note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = append(s.notes, note)
}

func (s *noteSink) list() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.notes)
}

func blocking(status int, raw string) func(*responses.ResponseExtraction) {
	return func(ext *responses.ResponseExtraction) { ext.ProcessBlocking(status, []byte(raw)) }
}

func event(raw string) func(*responses.ResponseExtraction) {
	return func(ext *responses.ResponseExtraction) { ext.OnEvent("", []byte(raw)) }
}
