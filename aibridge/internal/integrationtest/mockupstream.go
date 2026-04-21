package integrationtest

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/coder/aibridge/fixtures"
	"github.com/coder/aibridge/intercept/eventstream"
)

// upstreamResponse defines a single response that mockUpstream will replay
// for one incoming request. Use [newFixtureResponse] or [newFixtureToolResponse] to
// construct one from a parsed txtar archive.
type upstreamResponse struct {
	Streaming []byte // returned when the request has "stream": true.
	Blocking  []byte // returned for non-streaming requests.

	// OnRequest, if non-nil, is called with the incoming request and body
	// before the response is sent. Use it for per-request assertions.
	OnRequest func(r *http.Request, body []byte)
}

// newFixtureResponse creates an upstreamResponse from a parsed fixture archive.
// It reads whichever of 'streaming' and 'non-streaming' sections exist;
// not every fixture has both (e.g. error fixtures may only define one).
func newFixtureResponse(fix fixtures.Fixture) upstreamResponse {
	var resp upstreamResponse
	if fix.Has(fixtures.SectionStreaming) {
		resp.Streaming = fix.Streaming()
	}
	if fix.Has(fixtures.SectionNonStreaming) {
		resp.Blocking = fix.NonStreaming()
	}
	return resp
}

// newFixtureToolResponse creates an upstreamResponse from the tool-call fixture files.
// It reads whichever of 'streaming/tool-call' and 'non-streaming/tool-call'
// sections exist.
func newFixtureToolResponse(fix fixtures.Fixture) upstreamResponse {
	var resp upstreamResponse
	if fix.Has(fixtures.SectionStreamingToolCall) {
		resp.Streaming = fix.StreamingToolCall()
	}
	if fix.Has(fixtures.SectionNonStreamToolCall) {
		resp.Blocking = fix.NonStreamingToolCall()
	}
	return resp
}

// receivedRequest captures the details of a single request handled by mockUpstream.
type receivedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// mockUpstream replays txtar fixture responses, validates incoming request
// bodies, and counts calls. It stands in for a real AI provider API
// (Anthropic, OpenAI) during integration tests.
type mockUpstream struct {
	*httptest.Server

	// Calls is incremented atomically on every request.
	Calls atomic.Uint32

	// StatusCode overrides the HTTP status for non-streaming responses.
	// Zero means 200.
	StatusCode int

	// AllowOverflow disables the strict call-count check. When true,
	// requests beyond the last response repeat that response, and the
	// cleanup assertion only verifies that at least len(responses)
	// requests were made. This is useful for error-response tests where
	// the bridge may retry.
	AllowOverflow bool

	mu       sync.Mutex
	requests []receivedRequest

	t         *testing.T
	responses []upstreamResponse
}

// receivedRequests returns a copy of all requests received so far.
func (ms *mockUpstream) receivedRequests() []receivedRequest {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return append([]receivedRequest(nil), ms.requests...)
}

// newMockUpstream creates a started httptest.Server that replays fixture
// responses. Responses are returned in order: first call → first response.
// The test fails if the number of requests doesn't match the number of
// responses (when AllowOverflow is not set, default).
//
//	srv := newMockUpstream(ctx, t, newFixtureResponse(fix))                        // simple
//	srv := newMockUpstream(ctx, t, newFixtureResponse(fix), newFixtureToolResponse(fix)) // multi-turn
func newMockUpstream(ctx context.Context, t *testing.T, responses ...upstreamResponse) *mockUpstream {
	t.Helper()
	require.NotEmpty(t, responses, "at least one upstreamResponse required")

	ms := &mockUpstream{
		t:         t,
		responses: responses,
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(ms.handle))
	srv.Config.BaseContext = func(_ net.Listener) context.Context { return ctx }
	srv.Start()

	t.Cleanup(func() {
		srv.Close()

		// Verify the number of requests matches expectations.
		calls := int(ms.Calls.Load())
		if ms.AllowOverflow {
			require.LessOrEqual(t, len(ms.responses), calls, "too few requests, got: %v, want at least: %v", calls, len(ms.responses))
		} else {
			require.Equal(t, len(ms.responses), calls, "unexpected number of requests, got: %v, want: %v", calls, len(ms.responses))
		}
	})

	ms.Server = srv
	return ms
}

func (ms *mockUpstream) handle(w http.ResponseWriter, r *http.Request) {
	call := int(ms.Calls.Add(1) - 1)

	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	require.NoError(ms.t, err)

	ms.mu.Lock()
	ms.requests = append(ms.requests, receivedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Header: r.Header.Clone(),
		Body:   append([]byte(nil), body...),
	})
	ms.mu.Unlock()

	validateRequest(ms.t, call, r.URL.Path, body)

	resp := ms.responseForCall(call)
	if resp.OnRequest != nil {
		resp.OnRequest(r, body)
	}

	if isStreaming(body, r.URL.Path) {
		require.NotEmpty(ms.t, resp.Streaming, "response #%d: Streaming body is empty (fixture missing streaming response?)", call+1)
		if isRawHTTPResponse(resp.Streaming) {
			ms.writeRawHTTPResponse(w, r, resp.Streaming)
			return
		}
		ms.writeSSE(w, resp.Streaming)
		return
	}

	require.NotEmpty(ms.t, resp.Blocking, "response #%d: Blocking body is empty (fixture missing non-streaming response?)", call+1)
	if isRawHTTPResponse(resp.Blocking) {
		ms.writeRawHTTPResponse(w, r, resp.Blocking)
		return
	}

	status := cmp.Or(ms.StatusCode, http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(resp.Blocking)
}

func (ms *mockUpstream) responseForCall(call int) upstreamResponse {
	if call >= len(ms.responses) {
		if ms.AllowOverflow {
			return ms.responses[len(ms.responses)-1]
		}
		ms.t.Fatalf("unexpected number of calls: %v, got only %v responses", call, len(ms.responses))
	}
	return ms.responses[call]
}

func isStreaming(body []byte, urlPath string) bool {
	// The Anthropic SDK's Bedrock middleware extracts "stream"
	// from the JSON body and encodes them in the URL path instead.
	// See: https://github.com/anthropics/anthropic-sdk-go/blob/4d669338f2041f3c60640b6dd317c4895dc71cd4/bedrock/bedrock.go#L247-L248
	return gjson.GetBytes(body, "stream").Bool() || strings.HasSuffix(urlPath, "invoke-with-response-stream")
}

func (ms *mockUpstream) writeSSE(w http.ResponseWriter, data []byte) {
	ms.t.Helper()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Write line-by-line to simulate SSE events arriving incrementally
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		_, err := fmt.Fprintf(w, "%s\n", scanner.Text())
		if eventstream.IsConnError(err) {
			return // client disconnected, stop writing
		}
		require.NoError(ms.t, err)
		flusher.Flush()
	}
	require.NoError(ms.t, scanner.Err())
}

// isRawHTTPResponse returns true if data starts with "HTTP/", indicating
// it contains a complete HTTP response (status line + headers + body) rather
// than just a response body.
func isRawHTTPResponse(data []byte) bool {
	return bytes.HasPrefix(data, []byte("HTTP/"))
}

// writeRawHTTPResponse parses data as a complete HTTP response and replays it,
// copying the status code, headers, and body to w. This supports error fixtures
// that contain full HTTP responses (e.g. "HTTP/2.0 400 Bad Request\r\n...").
func (ms *mockUpstream) writeRawHTTPResponse(w http.ResponseWriter, r *http.Request, data []byte) {
	ms.t.Helper()

	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), r)
	require.NoError(ms.t, err)
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	require.NoError(ms.t, err)
}

// validateRequest dispatches to provider-specific validators based on URL path
// and fails the test immediately if the request body is invalid.
func validateRequest(t *testing.T, call int, path string, body []byte) {
	t.Helper()

	msgAndArgs := []any{fmt.Sprintf("request #%d validation failed\n\nBody:\n%s", call+1, body)}
	switch {
	case strings.Contains(path, "/chat/completions"):
		validateOpenAIChatCompletion(t, body, msgAndArgs...)
	case strings.Contains(path, "/responses"):
		validateOpenAIResponses(t, body, msgAndArgs...)
	case strings.Contains(path, "/messages"):
		validateAnthropicMessages(t, body, msgAndArgs...)
	}
}

// validateOpenAIChatCompletion validates that an OpenAI chat completion request
// has all required fields.
// See https://platform.openai.com/docs/api-reference/chat/create.
func validateOpenAIChatCompletion(t *testing.T, body []byte, msgAndArgs ...any) {
	t.Helper()

	var req openai.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal(body, &req), msgAndArgs...)
	require.NotEmpty(t, req.Model, "model is required", msgAndArgs)
	require.NotEmpty(t, req.Messages, "messages is required", msgAndArgs)
}

// validateOpenAIResponses validates that an OpenAI responses request
// has all required fields.
// See https://platform.openai.com/docs/api-reference/responses/create.
func validateOpenAIResponses(t *testing.T, body []byte, msgAndArgs ...any) {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m), msgAndArgs...)
	require.NotEmpty(t, m["model"], "model is required", msgAndArgs)
	require.Contains(t, m, "input", msgAndArgs...)
}

// validateAnthropicMessages validates that an Anthropic messages request
// has all required fields.
// See https://github.com/anthropics/anthropic-sdk-go.
func validateAnthropicMessages(t *testing.T, body []byte, msgAndArgs ...any) {
	t.Helper()

	var req anthropic.MessageNewParams
	require.NoError(t, json.Unmarshal(body, &req), msgAndArgs...)
	require.NotEmpty(t, req.Model, "model is required", msgAndArgs)
	require.NotEmpty(t, req.Messages, "messages is required", msgAndArgs)
	require.NotZero(t, req.MaxTokens, "max_tokens is required", msgAndArgs)
}
