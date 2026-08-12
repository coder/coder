package coderd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// decodeAIModelPrices decodes a prices document the same way the handler does,
// so a case can be written as the JSON an operator would send.
func decodeAIModelPrices(t *testing.T, body string) ([]codersdk.AIModelPriceUpsert, []map[string]json.RawMessage) {
	t.Helper()

	var typed codersdk.UpsertAIModelPricesRequest
	require.NoError(t, json.Unmarshal([]byte(body), &typed))

	var raw struct {
		Prices []map[string]json.RawMessage `json:"prices"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &raw))

	return typed.Prices, raw.Prices
}

func TestValidateAIModelPrices(t *testing.T) {
	t.Parallel()

	const allPrices = `"input_price": 100, "output_price": 200, "cache_read_price": 300, "cache_write_price": 400`

	tests := []struct {
		name string
		body string
		// want is every validation error, in order. Nil means the document is
		// accepted.
		want []codersdk.ValidationError
	}{
		{
			name: "EmptyRequest",
			body: `{"prices":[]}`,
			want: []codersdk.ValidationError{
				{Field: "prices", Detail: "At least one model price is required."},
			},
		},
		{
			name: "MissingProvider",
			body: `{"prices":[{"model":"my-model",` + allPrices + `}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0].provider", Detail: "Provider is required. Supported providers: anthropic, azure, bedrock, copilot, google, openai, openrouter, vercel."},
			},
		},
		{
			name: "UnsupportedProvider",
			body: `{"prices":[{"provider":"unknown-provider","model":"my-model",` + allPrices + `}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0].provider", Detail: `Provider "unknown-provider" is not supported. Supported providers: anthropic, azure, bedrock, copilot, google, openai, openrouter, vercel.`},
			},
		},
		{
			// openai-compat is a generic passthrough, so a price cannot be
			// attributed to the model behind it.
			name: "OpenAICompatRejected",
			body: `{"prices":[{"provider":"openai-compat","model":"my-model",` + allPrices + `}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0].provider", Detail: `Provider "openai-compat" is not supported. Supported providers: anthropic, azure, bedrock, copilot, google, openai, openrouter, vercel.`},
			},
		},
		{
			name: "MissingModel",
			body: `{"prices":[{"provider":"anthropic","model":"",` + allPrices + `}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0].model", Detail: "Model is required."},
			},
		},
		{
			// The price book is re-applied on every restart, so this price
			// would not survive one.
			name: "ModelInPriceBook",
			body: `{"prices":[{"provider":"anthropic","model":"claude-opus-5",` + allPrices + `}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0]", Detail: "anthropic/claude-opus-5 is priced by Coder's default price book. Overriding a default price is not supported."},
			},
		},
		{
			name: "NegativePrice",
			body: `{"prices":[{"provider":"anthropic","model":"my-model","input_price":-1,"output_price":200,"cache_read_price":null,"cache_write_price":null}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0].input_price", Detail: "Price must not be negative."},
			},
		},
		{
			// Some keys present and some absent would clear the absent ones.
			name: "MissingSomePriceKeys",
			body: `{"prices":[{"provider":"anthropic","model":"my-model","input_price":100}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0].output_price", Detail: "Price is required. Use 'null' for a price that is not known."},
				{Field: "prices[0].cache_read_price", Detail: "Price is required. Use 'null' for a price that is not known."},
				{Field: "prices[0].cache_write_price", Detail: "Price is required. Use 'null' for a price that is not known."},
			},
		},
		{
			// Reported once rather than four times, since no price was given at
			// all.
			name: "MissingAllPriceKeys",
			body: `{"prices":[{"provider":"anthropic","model":"my-model"}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0]", Detail: "At least one price must be set. Use 0 to declare a model free."},
			},
		},
		{
			name: "AllPricesNull",
			body: `{"prices":[{"provider":"anthropic","model":"my-model","input_price":null,"output_price":null,"cache_read_price":null,"cache_write_price":null}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0]", Detail: "At least one price must be set. Use 0 to declare a model free."},
			},
		},
		{
			name: "DuplicateEntry",
			body: `{"prices":[{"provider":"anthropic","model":"my-model",` + allPrices + `},` +
				`{"provider":"anthropic","model":"my-model",` + allPrices + `}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[1]", Detail: "anthropic/my-model appears more than once."},
			},
		},
		{
			// Model names may carry a "/", as openrouter IDs do.
			name: "SeparatorInAModelNameIsAccepted",
			body: `{"prices":[{"provider":"openrouter","model":"anthropic/my-model",` + allPrices + `}]}`,
			want: nil,
		},
		{
			// The two entries share a provider/model concatenation, so keying
			// on the pair is what keeps them apart.
			name: "SeparatorDoesNotCollideWithAProviderName",
			body: `{"prices":[{"provider":"openrouter","model":"anthropic/my-model",` + allPrices + `},` +
				`{"provider":"openrouter/anthropic","model":"my-model",` + allPrices + `}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[1].provider", Detail: `Provider "openrouter/anthropic" is not supported. Supported providers: anthropic, azure, bedrock, copilot, google, openai, openrouter, vercel.`},
			},
		},
		{
			// A model repeated under a different provider is a different row.
			name: "SameModelDifferentProvider",
			body: `{"prices":[{"provider":"anthropic","model":"my-model",` + allPrices + `},` +
				`{"provider":"openai","model":"my-model",` + allPrices + `}]}`,
			want: nil,
		},
		{
			// Provider comes first, and the absent price keys are reported once.
			name: "ReportsProviderBeforePrices",
			body: `{"prices":[{"provider":"unknown-provider","model":"my-model"}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0].provider", Detail: `Provider "unknown-provider" is not supported. Supported providers: anthropic, azure, bedrock, copilot, google, openai, openrouter, vercel.`},
				{Field: "prices[0]", Detail: "At least one price must be set. Use 0 to declare a model free."},
			},
		},
		{
			// Every entry is reported, not just the first bad one.
			name: "ReportsEveryEntry",
			body: `{"prices":[{"provider":"unknown-provider","model":"a",` + allPrices + `},` +
				`{"provider":"anthropic","model":"",` + allPrices + `}]}`,
			want: []codersdk.ValidationError{
				{Field: "prices[0].provider", Detail: `Provider "unknown-provider" is not supported. Supported providers: anthropic, azure, bedrock, copilot, google, openai, openrouter, vercel.`},
				{Field: "prices[1].model", Detail: "Model is required."},
			},
		},
		{
			name: "Valid",
			body: `{"prices":[{"provider":"anthropic","model":"my-model",` + allPrices + `}]}`,
			want: nil,
		},
		{
			name: "ValidWithNullPrices",
			body: `{"prices":[{"provider":"anthropic","model":"my-model","input_price":100,"output_price":null,"cache_read_price":null,"cache_write_price":null}]}`,
			want: nil,
		},
		{
			name: "ValidWithZeroPrice",
			body: `{"prices":[{"provider":"anthropic","model":"my-model","input_price":0,"output_price":0,"cache_read_price":null,"cache_write_price":null}]}`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			requested, raw := decodeAIModelPrices(t, tt.body)
			require.Equal(t, tt.want, validateAIModelPrices(requested, raw))
		})
	}
}
