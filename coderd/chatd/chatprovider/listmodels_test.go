package chatprovider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/testutil"
)

// newOpenAIModelServer returns an httptest.Server that responds to
// GET /v1/models (and GET /models for custom base URLs that omit
// the /v1 prefix) with a valid OpenAI-compatible model list.
func newOpenAIModelServer(t *testing.T, models []string, statusCode int) *httptest.Server {
	t.Helper()

	type openAIModel struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}

	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			http.Error(w, `{"error":{"message":"unauthorized","type":"auth_error","code":"invalid_api_key"}}`, statusCode)
			return
		}

		data := make([]openAIModel, 0, len(models))
		for _, m := range models {
			data = append(data, openAIModel{
				ID:      m,
				Object:  "model",
				Created: 1700000000,
				OwnedBy: "test",
			})
		}
		resp := map[string]any{
			"object": "list",
			"data":   data,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// The OpenAI SDK appends "models" to the base URL. When the
	// base URL includes /v1 (default), the request hits /v1/models.
	// When the base URL is the server root, it hits /models.
	mux.Handle("GET /v1/models", handler)
	mux.Handle("GET /models", handler)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// newAnthropicModelServer returns an httptest.Server that responds
// to GET /v1/models with a valid Anthropic model list.
func newAnthropicModelServer(t *testing.T, models []string) *httptest.Server {
	t.Helper()

	type anthropicModel struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		CreatedAt   string `json:"created_at"`
		Type        string `json:"type"`
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		data := make([]anthropicModel, 0, len(models))
		for _, m := range models {
			data = append(data, anthropicModel{
				ID:          m,
				DisplayName: m,
				CreatedAt:   "2025-01-01T00:00:00Z",
				Type:        "model",
			})
		}

		firstID := ""
		lastID := ""
		if len(models) > 0 {
			firstID = models[0]
			lastID = models[len(models)-1]
		}

		resp := map[string]any{
			"data":     data,
			"has_more": false,
			"first_id": firstID,
			"last_id":  lastID,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// newGoogleModelServer returns an httptest.Server that responds to
// GET /v1beta/models with a valid Google genai model list.
func newGoogleModelServer(t *testing.T, models []string) *httptest.Server {
	t.Helper()

	type googleModel struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1beta/models", func(w http.ResponseWriter, r *http.Request) {
		data := make([]googleModel, 0, len(models))
		for _, m := range models {
			data = append(data, googleModel{
				Name:        m,
				DisplayName: m,
			})
		}

		resp := map[string]any{
			"models": data,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestListProviderModels(t *testing.T) {
	t.Parallel()

	t.Run("OpenAI_Success", func(t *testing.T) {
		t.Parallel()

		ts := newOpenAIModelServer(t, []string{"gpt-4", "gpt-3.5-turbo", "gpt-4o"}, http.StatusOK)

		ctx := testutil.Context(t, testutil.WaitShort)
		models, err := chatprovider.ListProviderModels(ctx, "openai", "test-key", ts.URL+"/v1")
		require.NoError(t, err)
		require.Equal(t, []string{"gpt-3.5-turbo", "gpt-4", "gpt-4o"}, models)
	})

	t.Run("OpenAI_AuthFailure", func(t *testing.T) {
		t.Parallel()

		ts := newOpenAIModelServer(t, nil, http.StatusUnauthorized)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := chatprovider.ListProviderModels(ctx, "openai", "bad-key", ts.URL+"/v1")
		require.Error(t, err)
	})

	t.Run("Anthropic_Success", func(t *testing.T) {
		t.Parallel()

		ts := newAnthropicModelServer(t, []string{"claude-sonnet-4-20250514", "claude-haiku-3-20240307"})

		ctx := testutil.Context(t, testutil.WaitShort)
		models, err := chatprovider.ListProviderModels(ctx, "anthropic", "test-key", ts.URL)
		require.NoError(t, err)
		require.Equal(t, []string{"claude-haiku-3-20240307", "claude-sonnet-4-20250514"}, models)
	})

	t.Run("Google_Success", func(t *testing.T) {
		t.Parallel()

		ts := newGoogleModelServer(t, []string{"models/gemini-2.5-flash", "models/gemini-2.0-pro"})

		ctx := testutil.Context(t, testutil.WaitShort)
		models, err := chatprovider.ListProviderModels(ctx, "google", "test-key", ts.URL)
		require.NoError(t, err)
		require.Equal(t, []string{"models/gemini-2.0-pro", "models/gemini-2.5-flash"}, models)
	})

	t.Run("OpenAICompat_RequiresBaseURL", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := chatprovider.ListProviderModels(ctx, "openai-compat", "test-key", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "base URL is required")
	})

	t.Run("Azure_NotSupported", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := chatprovider.ListProviderModels(ctx, "azure", "test-key", "")
		require.ErrorIs(t, err, chatprovider.ErrModelListingNotSupported)
	})

	t.Run("Bedrock_NotSupported", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := chatprovider.ListProviderModels(ctx, "bedrock", "", "")
		require.ErrorIs(t, err, chatprovider.ErrModelListingNotSupported)
	})

	t.Run("UnsupportedProvider", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := chatprovider.ListProviderModels(ctx, "nonexistent-provider", "key", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported provider")
	})

	t.Run("EmptyProvider", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := chatprovider.ListProviderModels(ctx, "", "key", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported provider")
	})

	t.Run("OpenAI_CustomBaseURL", func(t *testing.T) {
		t.Parallel()

		ts := newOpenAIModelServer(t, []string{"custom-model-b", "custom-model-a"}, http.StatusOK)

		ctx := testutil.Context(t, testutil.WaitShort)
		// Pass the custom base URL. The function should use it
		// instead of the default OpenAI URL.
		models, err := chatprovider.ListProviderModels(ctx, "openai", "test-key", ts.URL+"/v1")
		require.NoError(t, err)
		require.Equal(t, []string{"custom-model-a", "custom-model-b"}, models)
	})
}
