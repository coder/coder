package chatcompletions_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/intercept"
	"github.com/coder/aibridge/intercept/chatcompletions"
	"github.com/coder/aibridge/internal/testutil"
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
			expectedErrStr: strconv.Itoa(http.StatusBadRequest),
			expectedBody:   "invalid_request",
		},
		{
			name:           "rate limit error",
			statusCode:     http.StatusTooManyRequests,
			responseBody:   `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
			expectedErrStr: strconv.Itoa(http.StatusTooManyRequests),
			expectedBody:   "rate_limit",
		},
		{
			name:           "internal server error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"error":{"message":"Internal server error","type":"server_error","code":"internal_error"}}`,
			expectedErrStr: strconv.Itoa(http.StatusInternalServerError),
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
			cfg := config.OpenAI{
				BaseURL: mockServer.URL,
				Key:     "test-key",
			}

			req := &chatcompletions.ChatCompletionNewParamsWrapper{
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
			interceptor := chatcompletions.NewStreamingInterceptor(uuid.New(), req, config.ProviderOpenAI, cfg, httpReq.Header, "Authorization", tracer, intercept.CredentialInfo{})

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
		})
	}
}
