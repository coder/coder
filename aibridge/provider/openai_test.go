package provider //nolint:testpackage // tests unexported internals

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/sync/errgroup"

	"cdr.dev/slog/v3"
	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/intercept"
	"github.com/coder/aibridge/internal/testutil"
)

const (
	chatCompletionResponse = `{"id":"chatcmpl-123","object":"chat.completion","created":1677652288,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}}`
	responsesAPIResponse   = `{"id":"resp-123","object":"response","created_at":1677652288,"model":"gpt-5","output":[],"usage":{"input_tokens":5,"output_tokens":10,"total_tokens":15}}`
)

type message struct {
	Role    string
	Content string
}

type providerStrategy interface {
	DefaultModel() string
	formatMessages(messages []message) []any
	buildRequestBody(model string, messages []any, stream bool) map[string]any
}
type responsesProvider struct{}

func (*responsesProvider) DefaultModel() string {
	return "gpt-5"
}

func (*responsesProvider) formatMessages(messages []message) []any {
	formatted := make([]any, 0, len(messages))
	for _, msg := range messages {
		formatted = append(formatted, map[string]any{
			"type":    "message",
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	return formatted
}

func (*responsesProvider) buildRequestBody(model string, messages []any, stream bool) map[string]any {
	return map[string]any{
		"model":  model,
		"input":  messages,
		"stream": stream,
	}
}

type chatCompletionsProvider struct{}

func (*chatCompletionsProvider) DefaultModel() string {
	return "gpt-4"
}

func (*chatCompletionsProvider) formatMessages(messages []message) []any {
	formatted := make([]any, 0, len(messages))
	for _, msg := range messages {
		formatted = append(formatted, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	return formatted
}

func (*chatCompletionsProvider) buildRequestBody(model string, messages []any, stream bool) map[string]any {
	return map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	}
}

func generateConversation(provider providerStrategy, targetSize int, numMessages int) []any {
	if targetSize <= 0 {
		return nil
	}
	if numMessages < 1 {
		numMessages = 1
	}

	roles := []string{"user", "assistant"}
	messages := make([]message, numMessages)
	for i := range messages {
		messages[i].Role = roles[i%2]
	}
	// Ensure last message is from user (required for LLM APIs).
	if messages[len(messages)-1].Role != "user" {
		messages[len(messages)-1].Role = "user"
	}

	overhead := measureJSONSize(provider.formatMessages(messages))

	bytesPerMessage := targetSize - overhead
	if bytesPerMessage < 0 {
		bytesPerMessage = 0
	}

	perMessage := bytesPerMessage / len(messages)
	remainder := bytesPerMessage % len(messages)

	for i := range messages {
		size := perMessage
		if i == len(messages)-1 {
			size += remainder
		}
		messages[i].Content = strings.Repeat("x", size)
	}

	return provider.formatMessages(messages)
}

func measureJSONSize(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(data)
}

// generateChatCompletionsPayload creates a JSON payload with the specified number of messages.
// Messages alternate between user and assistant roles to simulate a conversation.
func generateChatCompletionsPayload(payloadSize int, messageCount int, stream bool) []byte {
	provider := &chatCompletionsProvider{}
	messages := generateConversation(provider, payloadSize, messageCount)

	body := provider.buildRequestBody(provider.DefaultModel(), messages, stream)
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return bodyBytes
}

// generateResponsesPayload creates a JSON payload for the responses API with the specified number of input items.
// Input items alternate between user and assistant roles to simulate a conversation.
func generateResponsesPayload(payloadSize int, inputCount int, stream bool) []byte {
	provider := &responsesProvider{}
	inputs := generateConversation(provider, payloadSize, inputCount)

	body := provider.buildRequestBody(provider.DefaultModel(), inputs, stream)
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return bodyBytes
}

func TestOpenAI_TypeAndName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        config.OpenAI
		expectType string
		expectName string
	}{
		{
			name:       "defaults",
			cfg:        config.OpenAI{},
			expectType: config.ProviderOpenAI,
			expectName: config.ProviderOpenAI,
		},
		{
			name:       "custom_name",
			cfg:        config.OpenAI{Name: "openai-custom"},
			expectType: config.ProviderOpenAI,
			expectName: "openai-custom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := NewOpenAI(tc.cfg)
			assert.Equal(t, tc.expectType, p.Type())
			assert.Equal(t, tc.expectName, p.Name())
		})
	}
}

func TestOpenAI_CreateInterceptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		route              string
		requestBody        string
		responseBody       string
		setHeaders         map[string]string
		wantAuthorization  string
		wantCredentialKind intercept.CredentialKind
		wantCredentialHint string
	}{
		{
			name:               "ChatCompletions_BYOK",
			route:              routeChatCompletions,
			requestBody:        `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}], "stream": false}`,
			responseBody:       chatCompletionResponse,
			setHeaders:         map[string]string{"Authorization": "Bearer user-token"},
			wantAuthorization:  "Bearer user-token",
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...en",
		},
		{
			name:               "ChatCompletions_Centralized",
			route:              routeChatCompletions,
			requestBody:        `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}], "stream": false}`,
			responseBody:       chatCompletionResponse,
			setHeaders:         map[string]string{},
			wantAuthorization:  "Bearer centralized-key",
			wantCredentialKind: intercept.CredentialKindCentralized,
			wantCredentialHint: "ce...ey",
		},
		{
			name:               "Responses_BYOK",
			route:              routeResponses,
			requestBody:        `{"model": "gpt-5", "input": "hello", "stream": false}`,
			responseBody:       responsesAPIResponse,
			setHeaders:         map[string]string{"Authorization": "Bearer user-token"},
			wantAuthorization:  "Bearer user-token",
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...en",
		},
		{
			name:               "Responses_Centralized",
			route:              routeResponses,
			requestBody:        `{"model": "gpt-5", "input": "hello", "stream": false}`,
			responseBody:       responsesAPIResponse,
			setHeaders:         map[string]string{},
			wantAuthorization:  "Bearer centralized-key",
			wantCredentialKind: intercept.CredentialKindCentralized,
			wantCredentialHint: "ce...ey",
		},
		// X-Api-Key should not appear in production since clients use Authorization,
		// but ensure it is stripped if it does arrive.
		{
			name:         "ChatCompletions_BYOK_XApiKeyStripped",
			route:        routeChatCompletions,
			requestBody:  `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}], "stream": false}`,
			responseBody: chatCompletionResponse,
			setHeaders: map[string]string{
				"Authorization": "Bearer user-token",
				"X-Api-Key":     "some-key",
			},
			wantAuthorization:  "Bearer user-token",
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...en",
		},
		{
			name:         "Responses_BYOK_XApiKeyStripped",
			route:        routeResponses,
			requestBody:  `{"model": "gpt-5", "input": "hello", "stream": false}`,
			responseBody: responsesAPIResponse,
			setHeaders: map[string]string{
				"Authorization": "Bearer user-token",
				"X-Api-Key":     "some-key",
			},
			wantAuthorization:  "Bearer user-token",
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...en",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var receivedHeaders http.Header

			mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(tc.responseBody))
				require.NoError(t, err)
			}))
			t.Cleanup(mockUpstream.Close)

			provider := NewOpenAI(config.OpenAI{
				BaseURL: mockUpstream.URL,
				Key:     "centralized-key",
			})

			req := httptest.NewRequest(http.MethodPost, provider.RoutePrefix()+tc.route, bytes.NewBufferString(tc.requestBody))
			for k, v := range tc.setHeaders {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()

			interceptor, err := provider.CreateInterceptor(w, req, testTracer)
			require.NoError(t, err)
			require.NotNil(t, interceptor)

			cred := interceptor.Credential()
			assert.Equal(t, tc.wantCredentialKind, cred.Kind, "credential kind mismatch")
			assert.Equal(t, tc.wantCredentialHint, cred.Hint, "credential hint mismatch")

			logger := slog.Make()
			interceptor.Setup(logger, &testutil.MockRecorder{}, nil)

			processReq := httptest.NewRequest(http.MethodPost, provider.RoutePrefix()+tc.route, nil)
			err = interceptor.ProcessRequest(w, processReq)
			require.NoError(t, err)

			assert.Equal(t, tc.wantAuthorization, receivedHeaders.Get("Authorization"))
			assert.Empty(t, receivedHeaders.Get("X-Api-Key"), "X-Api-Key must not be set upstream")
		})
	}
}

func TestOpenAI_InjectAuthHeader(t *testing.T) {
	t.Parallel()

	provider := NewOpenAI(config.OpenAI{Key: "centralized-key"})

	tests := []struct {
		name              string
		presetHeaders     map[string]string
		wantAuthorization string
	}{
		{
			name:              "when no Authorization header is provided, inject centralized key",
			presetHeaders:     map[string]string{},
			wantAuthorization: "Bearer centralized-key",
		},
		{
			name:              "when Authorization header is provided, do not overwrite it",
			presetHeaders:     map[string]string{"Authorization": "Bearer user-token"},
			wantAuthorization: "Bearer user-token",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := http.Header{}
			for k, v := range tc.presetHeaders {
				headers.Set(k, v)
			}

			provider.InjectAuthHeader(&headers)

			assert.Equal(t, tc.wantAuthorization, headers.Get("Authorization"))
		})
	}
}

func BenchmarkOpenAI_CreateInterceptor_ChatCompletions(b *testing.B) {
	provider := NewOpenAI(config.OpenAI{
		BaseURL: "https://api.openai.com/v1/",
		Key:     "test-key",
	})

	tracer := noop.NewTracerProvider().Tracer("test")
	messagesPerRequest := 50
	requestCount := 100
	maxConcurrentRequests := 10
	payloadSizes := []int{2000, 10000, 50000, 100000, 2000000}
	for _, payloadSize := range payloadSizes {
		for _, stream := range []bool{true, false} {
			payload := generateChatCompletionsPayload(payloadSize, messagesPerRequest, stream)
			name := fmt.Sprintf("stream=%t/payloadSize=%d/requests=%d", stream, payloadSize, requestCount)

			b.Run(name, func(b *testing.B) {
				b.ResetTimer()
				for range b.N {
					eg := errgroup.Group{}
					eg.SetLimit(maxConcurrentRequests)
					for i := 0; i < requestCount; i++ {
						eg.Go(func() error {
							req := httptest.NewRequest(http.MethodPost, routeChatCompletions, bytes.NewReader(payload))
							w := httptest.NewRecorder()
							_, err := provider.CreateInterceptor(w, req, tracer)
							if err != nil {
								return err
							}
							return nil
						})
					}
				}
			})
		}
	}
}

func BenchmarkOpenAI_CreateInterceptor_Responses(b *testing.B) {
	provider := NewOpenAI(config.OpenAI{
		BaseURL: "https://api.openai.com/v1/",
		Key:     "test-key",
	})

	tracer := noop.NewTracerProvider().Tracer("test")
	messagesPerRequest := 50
	requestCount := 100
	maxConcurrentRequests := 10
	// payloadSizes := []int{2000, 10000, 50000, 100000, 2000000}
	payloadSizes := []int{2000000}
	for _, payloadSize := range payloadSizes {
		for _, stream := range []bool{true, false} {
			payload := generateResponsesPayload(payloadSize, messagesPerRequest, stream)
			name := fmt.Sprintf("stream=%t/payloadSize=%d/requests=%d", stream, payloadSize, requestCount)

			b.Run(name, func(b *testing.B) {
				b.ResetTimer()
				for range b.N {
					eg := errgroup.Group{}
					eg.SetLimit(maxConcurrentRequests)
					for i := 0; i < requestCount; i++ {
						eg.Go(func() error {
							req := httptest.NewRequest(http.MethodPost, routeResponses, bytes.NewReader(payload))
							w := httptest.NewRecorder()
							interceptor, err := provider.CreateInterceptor(w, req, tracer)
							if err != nil {
								return err
							}
							err = interceptor.ProcessRequest(w, req)
							if err != nil {
								return err
							}
							return nil
						})
					}
				}
			})
		}
	}
}
