package chatprovider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
)

// TestModelFromConfigHeaders verifies that custom headers passed to
// ModelFromConfig arrive on every outgoing HTTP request to the LLM
// provider.
func TestModelFromConfigHeaders(t *testing.T) {
	t.Parallel()

	t.Run("HeadersPresent", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		headers := map[string]string{
			"X-Coder-Chat-Id": chatID.String(),
		}

		received := captureHeaders(t, headers)
		assert.Equal(t, chatID.String(), received.Get("X-Coder-Chat-Id"))
	})

	t.Run("NilHeaders", func(t *testing.T) {
		t.Parallel()

		received := captureHeaders(t, nil)
		assert.Empty(t, received.Get("X-Coder-Chat-Id"))
	})
}

// captureHeaders creates an OpenAI-compatible test server, calls
// ModelFromConfig with the given headers, invokes Generate, and
// returns the HTTP headers that arrived at the server.
func captureHeaders(t *testing.T, headers map[string]string) http.Header {
	t.Helper()

	var mu sync.Mutex
	var captured http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured = r.Header.Clone()
		mu.Unlock()

		// Return a minimal valid OpenAI Responses API response.
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":      fmt.Sprintf("resp_%s", uuid.New().String()[:8]),
			"object":  "response",
			"created": time.Now().Unix(),
			"model":   "gpt-4o-mini",
			"output": []map[string]interface{}{
				{
					"id":   uuid.New().String(),
					"type": "message",
					"role": "assistant",
					"content": []map[string]interface{}{
						{
							"type": "output_text",
							"text": "hello",
						},
					},
				},
			},
			"usage": map[string]int{
				"input_tokens":  5,
				"output_tokens": 1,
				"total_tokens":  6,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	keys := chatprovider.ProviderAPIKeys{
		OpenAI:            "test-key",
		BaseURLByProvider: map[string]string{"openai": srv.URL},
	}

	model, err := chatprovider.ModelFromConfig("openai", "gpt-4o-mini", keys, headers)
	require.NoError(t, err)

	maxTokens := int64(10)
	_, err = model.Generate(t.Context(), fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "hi"},
				},
			},
		},
		MaxOutputTokens: &maxTokens,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, captured, "server should have received a request")
	return captured
}
