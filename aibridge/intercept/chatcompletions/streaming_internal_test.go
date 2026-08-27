package chatcompletions

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"github.com/coder/coder/v2/aibridge/recorder"
)

func TestStreamProcessorUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		chunks               []string
		wantPromptTokens     int64
		wantCompletionTokens int64
		wantTotalTokens      int64
	}{
		{
			name: "cumulative snapshots with trailing usage-less chunk",
			chunks: []string{
				`{"id":"chatcmpl-cumulative","choices":[{"index":0,"delta":{"content":"one"}}],"usage":{"prompt_tokens":6000,"completion_tokens":10,"total_tokens":6010}}`,
				`{"id":"chatcmpl-cumulative","choices":[{"index":0,"delta":{"content":" two"}}],"usage":{"prompt_tokens":6000,"completion_tokens":20,"total_tokens":6020}}`,
				`{"id":"chatcmpl-cumulative","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":6000,"completion_tokens":30,"total_tokens":6030}}`,
				`{"id":"chatcmpl-cumulative","choices":[]}`,
			},
			wantPromptTokens:     6000,
			wantCompletionTokens: 30,
			wantTotalTokens:      6030,
		},
		{
			name: "usage only on final chunk",
			chunks: []string{
				`{"id":"chatcmpl-final","choices":[{"index":0,"delta":{"content":"one"}}]}`,
				`{"id":"chatcmpl-final","choices":[{"index":0,"delta":{"content":" two"}}]}`,
				`{"id":"chatcmpl-final","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":6000,"completion_tokens":30,"total_tokens":6030}}`,
			},
			wantPromptTokens:     6000,
			wantCompletionTokens: 30,
			wantTotalTokens:      6030,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
			interceptor := NewStreamingInterceptor(uuid.New(), nil, intercept.Config{}, nil, nil, otel.Tracer("test"))
			processor := newStreamProcessor(t.Context(), logger, nil)

			var relayed []byte
			for _, rawChunk := range tt.chunks {
				var chunk openai.ChatCompletionChunk
				require.NoError(t, json.Unmarshal([]byte(rawChunk), &chunk))
				require.True(t, processor.process(chunk))
				if chunk.JSON.Usage.Valid() {
					var err error
					relayed, err = interceptor.marshalChunk(&chunk, interceptor.ID(), processor, openai.CompletionUsage{})
					require.NoError(t, err)
				}
			}

			var payload struct {
				Usage openai.CompletionUsage `json:"usage"`
			}
			relayed = bytes.TrimSuffix(bytes.TrimPrefix(relayed, []byte("data: ")), []byte("\n\n"))
			require.NoError(t, json.Unmarshal(relayed, &payload))
			assert.Equal(t, tt.wantPromptTokens, payload.Usage.PromptTokens)
			assert.Equal(t, tt.wantCompletionTokens, payload.Usage.CompletionTokens)
			assert.Equal(t, tt.wantTotalTokens, payload.Usage.TotalTokens)

			usage := processor.lastUsage
			assert.Equal(t, tt.wantPromptTokens, usage.PromptTokens)
			assert.Equal(t, tt.wantCompletionTokens, usage.CompletionTokens)
			assert.Equal(t, tt.wantTotalTokens, usage.TotalTokens)
		})
	}
}

func TestStreamingInterceptionRecordsLatestUsageWithZeroCompletionTokens(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-zero-completion\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"one\"}}],\"usage\":{\"prompt_tokens\":40,\"completion_tokens\":0,\"total_tokens\":40,\"prompt_tokens_details\":{\"cached_tokens\":5,\"cache_write_tokens\":3}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-zero-completion\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":50,\"completion_tokens\":0,\"total_tokens\":50,\"prompt_tokens_details\":{\"cached_tokens\":10,\"cache_write_tokens\":5}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(upstream.Close)

	req := &ChatCompletionNewParamsWrapper{
		ChatCompletionNewParams: openai.ChatCompletionNewParams{
			Model: "gpt-4",
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
			},
		},
		Stream: true,
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)
	interceptor := NewStreamingInterceptor(
		uuid.New(),
		req,
		intercept.Config{BaseURL: upstream.URL},
		intercept.BYOK{Secret: "test-key", Header: intercept.AuthHeaderAuthorization},
		httpReq.Header,
		otel.Tracer("test"),
	)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: false}).Leveled(slog.LevelDebug)
	rec := &testutil.MockRecorder{}
	interceptor.Setup(logger, rec, nil)

	w := httptest.NewRecorder()
	require.NoError(t, interceptor.ProcessRequest(w, httpReq))

	usages := rec.RecordedTokenUsages()
	require.Len(t, usages, 1)
	require.Equal(t, &recorder.TokenUsageRecord{
		InterceptionID:        interceptor.ID().String(),
		MsgID:                 "chatcmpl-zero-completion",
		Input:                 35,
		Output:                0,
		CacheReadInputTokens:  10,
		CacheWriteInputTokens: 5,
		CreatedAt:             usages[0].CreatedAt,
		ExtraTokenTypes: map[string]int64{
			"prompt_audio":                   0,
			"completion_accepted_prediction": 0,
			"completion_rejected_prediction": 0,
			"completion_audio":               0,
			"completion_reasoning":           0,
		},
	}, usages[0])
}

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
		})
	}
}
