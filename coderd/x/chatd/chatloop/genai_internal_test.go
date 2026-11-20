package chatloop

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

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

func TestErrorTypeKeepsTransportResetClassification(t *testing.T) {
	t.Parallel()

	require.Equal(t, "canceled", errorType(context.Canceled))
	reset := errors.Join(chaterror.ErrProviderTransportReset, context.Canceled)
	require.Equal(t, string(chaterror.Classify(reset).Kind), errorType(reset))
	require.NotEqual(t, "canceled", errorType(reset))
}
