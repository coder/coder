package chatcompletions

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

// Test that when the upstream provider returns an error before streaming starts,
// the error status code and body are correctly relayed to the client.
func TestStreamingInterception_RelaysUpstreamErrorToClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrStr string
		expectedBody   string
	}{
		{
			name:           "bad request error",
			statusCode:     http.StatusBadRequest,
			responseBody:   `{"error":{"message":"Invalid request","type":"invalid_request_error","code":"invalid_request"}}`,
			expectedErrStr: "Invalid request",
			expectedBody:   "invalid_request",
		},
		{
			name:           "rate limit error",
			statusCode:     http.StatusTooManyRequests,
			responseBody:   `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
			expectedErrStr: "Rate limit exceeded",
			expectedBody:   "rate_limit",
		},
		{
			name:           "internal server error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"error":{"message":"Internal server error","type":"server_error","code":"internal_error"}}`,
			expectedErrStr: "Internal server error",
			expectedBody:   "server_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup a mock server that returns an error immediately (before any streaming)
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("x-should-retry", "false")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			t.Cleanup(mockServer.Close)

			// Create interceptor with mock server URL
			cfg := intercept.Config{
				BaseURL: mockServer.URL,
			}
			cred := intercept.BYOK{Secret: "test-key", Header: intercept.AuthHeaderAuthorization}

			req := &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Model: "gpt-4",
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("hello"),
					},
				},
				Stream: true,
			}

			// Create test request
			w := httptest.NewRecorder()
			httpReq := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)

			tracer := otel.Tracer("test")
			interceptor := NewStreamingInterceptor(uuid.New(), req, cfg, cred, httpReq.Header, tracer)

			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
			interceptor.Setup(logger, &testutil.MockRecorder{}, nil)

			// Process the request
			err := interceptor.ProcessRequest(w, httpReq)

			// Verify error was returned
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErrStr)

			// Verify status code was written to response
			assert.Equal(t, tc.statusCode, w.Code, "expected status code to be relayed to client")

			// Verify error body contains expected error info
			body := w.Body.String()
			assert.Contains(t, body, tc.expectedBody, "expected error type in response body")
			assert.NotContains(t, body, "data: [DONE]", "direct JSON error response must not include SSE data")
		})
	}
}

// OpenAI-shaped SSE body for a successful streaming response.
const streamingSuccessBody = `data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}

data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]

`

func TestStreamingInterception_HandlesUpstreamSSEEdgeCases(t *testing.T) {
	t.Parallel()

	validChunkWithoutDone := `data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}

`
	largeContent := strings.Repeat("a", 70*1024)
	largeChunk := `data: {"id":"chatcmpl-01","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"` + largeContent + `"},"finish_reason":null}]}` + "\n\n"

	tests := []struct {
		name               string
		body               string
		expectErr          bool
		expectedStatusCode int
		expectedBody       string
		unexpectedBody     string
	}{
		{
			name:               "empty body",
			body:               "",
			expectErr:          true,
			expectedStatusCode: http.StatusBadGateway,
			expectedBody:       upstreamEmptyStreamMessage,
			unexpectedBody:     "unexpected end of JSON input",
		},
		{
			name:               "comment only event",
			body:               ": OPENROUTER PROCESSING\n\n",
			expectErr:          true,
			expectedStatusCode: http.StatusBadGateway,
			expectedBody:       upstreamEmptyStreamMessage,
			unexpectedBody:     "unexpected end of JSON input",
		},
		{
			name:               "comment only without final blank line",
			body:               ": OPENROUTER PROCESSING\n",
			expectErr:          true,
			expectedStatusCode: http.StatusBadGateway,
			expectedBody:       upstreamEmptyStreamMessage,
			unexpectedBody:     "unexpected end of JSON input",
		},
		{
			name:               "done marker only",
			body:               "data: [DONE]\n\n",
			expectErr:          true,
			expectedStatusCode: http.StatusBadGateway,
			expectedBody:       upstreamEmptyStreamMessage,
		},
		{
			name:               "malformed data event",
			body:               "data: {not-json}\n\n",
			expectErr:          true,
			expectedStatusCode: http.StatusBadGateway,
			expectedBody:       upstreamMalformedStreamMessage,
		},
		{
			name:               "valid chunk without done",
			body:               validChunkWithoutDone,
			expectedStatusCode: http.StatusOK,
			expectedBody:       "Hello",
		},
		{
			name:               "large data event",
			body:               largeChunk,
			expectedStatusCode: http.StatusOK,
			expectedBody:       largeContent,
		},
		{
			name:               "comments before valid chunks",
			body:               ": OPENROUTER PROCESSING\n\n" + streamingSuccessBody,
			expectedStatusCode: http.StatusOK,
			expectedBody:       "Hello",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(upstream.Close)

			cfg := intercept.Config{
				BaseURL: upstream.URL,
			}
			cred := intercept.BYOK{Secret: "test-key", Header: intercept.AuthHeaderAuthorization}

			params := &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Model: "gpt-4",
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("hello"),
					},
				},
				Stream: true,
			}

			interceptor := NewStreamingInterceptor(uuid.New(), params, cfg, cred, http.Header{}, otel.Tracer("streaming_test"))
			interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, nil)

			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			w := httptest.NewRecorder()
			err := interceptor.ProcessRequest(w, req)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			body := w.Body.String()
			assert.Equal(t, tc.expectedStatusCode, w.Code, "response status code")
			assert.Contains(t, body, tc.expectedBody, "response body")
			if tc.expectErr {
				assert.NotContains(t, body, "data: [DONE]", "direct JSON error response must not include SSE data")
			}
			if tc.unexpectedBody != "" {
				assert.NotContains(t, body, tc.unexpectedBody, "response body")
			}
		})
	}
}
