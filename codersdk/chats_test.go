package codersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
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
		TotalCostMicros: decimal.RequireFromString("123.45"),
	}
	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded codersdk.ChatCostSummary
	err = json.Unmarshal(raw, &decoded)
	require.NoError(t, err)
	require.True(t, original.TotalCostMicros.Equal(decoded.TotalCostMicros))
}
