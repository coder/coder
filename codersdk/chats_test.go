package codersdk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestChatModelProviderOptions_MarshalJSON_UsesPlainProviderPayload(t *testing.T) {
	t.Parallel()

	sendReasoning := true
	effort := "high"

	raw, err := json.Marshal(codersdk.ChatModelProviderOptions{
		Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
			SendReasoning: &sendReasoning,
			Effort:        &effort,
		},
	})
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"type":"anthropic.options"`)
	require.NotContains(t, string(raw), `"data":`)
	require.Contains(t, string(raw), `"send_reasoning":true`)
	require.Contains(t, string(raw), `"effort":"high"`)
}

func TestChatModelProviderOptions_UnmarshalJSON_ParsesPlainProviderPayloads(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"anthropic": {
			"send_reasoning": true,
			"effort": "high"
		}
	}`)

	var decoded codersdk.ChatModelProviderOptions
	err := json.Unmarshal(raw, &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Anthropic)
	require.NotNil(t, decoded.Anthropic.SendReasoning)
	require.True(t, *decoded.Anthropic.SendReasoning)
	require.NotNil(t, decoded.Anthropic.Effort)
	require.Equal(
		t,
		"high",
		*decoded.Anthropic.Effort,
	)
}

func TestChatUsageLimitExceededFrom(t *testing.T) {
	t.Parallel()

	t.Run("ExtractsTyped409", func(t *testing.T) {
		t.Parallel()

		want := codersdk.ChatUsageLimitExceededResponse{
			Response:    codersdk.Response{Message: "Chat usage limit exceeded."},
			SpentMicros: 123,
			LimitMicros: 456,
			ResetsAt:    time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC),
		}

		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "/api/experimental/chats", r.URL.Path)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusConflict)
			require.NoError(t, json.NewEncoder(rw).Encode(want))
		}))
		defer srv.Close()

		serverURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		client := codersdk.New(serverURL)
		_, err = client.CreateChat(context.Background(), codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
		})
		require.Error(t, err)

		sdkErr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
		require.Equal(t, want.Message, sdkErr.Message)

		limitErr := codersdk.ChatUsageLimitExceededFrom(err)
		require.NotNil(t, limitErr)
		require.Equal(t, want, *limitErr)
	})

	t.Run("ReturnsNilForNonLimitErrors", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, codersdk.ChatUsageLimitExceededFrom(codersdk.NewError(http.StatusConflict, codersdk.Response{Message: "plain conflict"})))

		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusBadRequest)
			require.NoError(t, json.NewEncoder(rw).Encode(codersdk.Response{Message: "Invalid request."}))
		}))
		defer srv.Close()

		serverURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		client := codersdk.New(serverURL)
		_, err = client.CreateChat(context.Background(), codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello",
			}},
		})
		require.Error(t, err)

		sdkErr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Nil(t, codersdk.ChatUsageLimitExceededFrom(err))
	})
}

func TestChatMessagePart_StripInternal(t *testing.T) {
	t.Parallel()

	t.Run("StripsProviderMetadata", func(t *testing.T) {
		t.Parallel()
		part := codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeToolCall,
			ToolCallID:       "call-1",
			ToolName:         "some_tool",
			Args:             json.RawMessage(`{"key":"value"}`),
			ProviderMetadata: json.RawMessage(`{"type":"ephemeral"}`),
		}
		part.StripInternal()
		assert.Nil(t, part.ProviderMetadata)
		// Public fields preserved.
		assert.Equal(t, codersdk.ChatMessagePartTypeToolCall, part.Type)
		assert.Equal(t, "call-1", part.ToolCallID)
		assert.Equal(t, "some_tool", part.ToolName)
		assert.JSONEq(t, `{"key":"value"}`, string(part.Args))
	})

	t.Run("StripsFileDataWhenFileIDSet", func(t *testing.T) {
		t.Parallel()
		id := uuid.New()
		part := codersdk.ChatMessagePart{
			Type:      codersdk.ChatMessagePartTypeFile,
			FileID:    uuid.NullUUID{UUID: id, Valid: true},
			MediaType: "image/png",
			Data:      []byte("binary-payload"),
		}
		part.StripInternal()
		assert.Nil(t, part.Data)
		assert.Equal(t, id, part.FileID.UUID)
		assert.Equal(t, "image/png", part.MediaType)
	})

	t.Run("PreservesDataWhenNoFileID", func(t *testing.T) {
		t.Parallel()
		part := codersdk.ChatMessagePart{
			Type:      codersdk.ChatMessagePartTypeFile,
			MediaType: "image/png",
			Data:      []byte("inline-data"),
		}
		part.StripInternal()
		assert.Equal(t, []byte("inline-data"), part.Data)
	})

	t.Run("NoopOnCleanPart", func(t *testing.T) {
		t.Parallel()
		part := codersdk.ChatMessageText("hello")
		part.StripInternal()
		assert.Equal(t, "hello", part.Text)
		assert.Equal(t, codersdk.ChatMessagePartTypeText, part.Type)
	})
}

func TestModelCostConfig_LegacyNumericJSON(t *testing.T) {
	t.Parallel()

	var decoded codersdk.ModelCostConfig
	err := json.Unmarshal([]byte("{\"input_price_per_million_tokens\": 1.5}"), &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.InputPricePerMillionTokens)
	require.True(t, decoded.InputPricePerMillionTokens.Equal(decimal.RequireFromString("1.5")))
}

func TestModelCostConfig_QuotedDecimalJSON(t *testing.T) {
	t.Parallel()

	var decoded codersdk.ModelCostConfig
	err := json.Unmarshal([]byte("{\"input_price_per_million_tokens\": \"1.5\"}"), &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.InputPricePerMillionTokens)
	require.True(t, decoded.InputPricePerMillionTokens.Equal(decimal.RequireFromString("1.5")))
}

func TestModelCostConfig_NilVsZero(t *testing.T) {
	t.Parallel()

	zero := decimal.Zero
	raw, err := json.Marshal(struct {
		Nil  codersdk.ModelCostConfig `json:"nil"`
		Zero codersdk.ModelCostConfig `json:"zero"`
	}{
		Nil:  codersdk.ModelCostConfig{},
		Zero: codersdk.ModelCostConfig{InputPricePerMillionTokens: &zero},
	})
	require.NoError(t, err)
	require.Contains(t, string(raw), "\"zero\":{\"input_price_per_million_tokens\":\"0\"}")
	require.Contains(t, string(raw), "\"nil\":{}")
}

func TestChatModelCallConfig_UnmarshalLegacyPricing(t *testing.T) {
	t.Parallel()

	var decoded codersdk.ChatModelCallConfig
	err := json.Unmarshal([]byte("{\"input_price_per_million_tokens\": 1.5}"), &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Cost)
	require.NotNil(t, decoded.Cost.InputPricePerMillionTokens)
	require.True(t, decoded.Cost.InputPricePerMillionTokens.Equal(decimal.RequireFromString("1.5")))
}

func TestChatCostSummary_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := codersdk.ChatCostSummary{
		TotalCostMicros: 123,
	}
	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded codersdk.ChatCostSummary
	err = json.Unmarshal(raw, &decoded)
	require.NoError(t, err)
	require.Equal(t, original.TotalCostMicros, decoded.TotalCostMicros)
}
