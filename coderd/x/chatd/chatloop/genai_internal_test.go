package chatloop

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

func TestGenAIProviderNameCoversEveryProviderType(t *testing.T) {
	t.Parallel()

	// Adding a provider type fails this test until the type is mapped to
	// a well-known gen_ai.provider.name or listed as a pass-through.
	want := map[database.AIProviderType]string{
		database.AIProviderTypeOpenai:       "openai",
		database.AIProviderTypeAnthropic:    "anthropic",
		database.AIProviderTypeAzure:        "azure.ai.openai",
		database.AIProviderTypeBedrock:      "aws.bedrock",
		database.AIProviderTypeGoogle:       "gcp.gemini",
		database.AIProviderTypeOpenaiCompat: "openai-compat",
		database.AIProviderTypeOpenrouter:   "openrouter",
		database.AIProviderTypeVercel:       "vercel",
		database.AIProviderTypeCopilot:      "copilot",
	}
	require.ElementsMatch(t, database.AllAIProviderTypeValues(), slices.Collect(maps.Keys(want)))
	for providerType, name := range want {
		require.Equal(t, name, genAIProviderName(StageModel{ProviderType: string(providerType)}), providerType)
	}
}

func TestUsageAttributesInputTokens(t *testing.T) {
	t.Parallel()

	usage := fantasy.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 3, CacheCreationTokens: 2}
	inputTokens := func(model StageModel) attribute.KeyValue {
		for _, attr := range usageAttributes(model, usage) {
			if attr.Key == semconv.GenAIUsageInputTokensKey {
				return attr
			}
		}
		t.Fatal("no input token attribute")
		return attribute.KeyValue{}
	}

	// Anthropic reports cache reads and writes apart from input tokens.
	require.Equal(t, int64(15), inputTokens(StageModel{Provider: "anthropic"}).Value.AsInt64())
	// Google's prompt token count already includes cached content.
	require.Equal(t, int64(10), inputTokens(StageModel{Provider: "google", ProviderType: "google"}).Value.AsInt64())
}

func TestErrorTypeKeepsTransportResetClassification(t *testing.T) {
	t.Parallel()

	require.Equal(t, "canceled", errorType(context.Canceled))
	reset := errors.Join(chaterror.ErrProviderTransportReset, context.Canceled)
	require.Equal(t, string(chaterror.Classify(reset).Kind), errorType(reset))
	require.NotEqual(t, "canceled", errorType(reset))
}
