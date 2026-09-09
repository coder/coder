package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestBuildProviderFromProtoSetsAPIDumpDir(t *testing.T) {
	t.Parallel()

	const dumpDir = "/tmp/coder-aibridge-dumps"

	tests := []struct {
		name         string
		provider     *proto.AIProvider
		expectedType string
	}{
		{
			name: "OpenAI",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeOpenai),
				Name:    "openai",
				BaseUrl: "https://api.openai.com/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "Anthropic",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeAnthropic),
				Name:    "anthropic",
				BaseUrl: "https://api.anthropic.com/",
			},
			expectedType: aibridge.ProviderAnthropic,
		},
		{
			name: "Copilot",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeCopilot),
				Name:    "copilot",
				BaseUrl: "https://api.githubcopilot.com/",
			},
			expectedType: aibridge.ProviderCopilot,
		},
		{
			name: "Azure",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeAzure),
				Name:    "azure",
				BaseUrl: "https://example.openai.azure.com/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "Google",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeGoogle),
				Name:    "google",
				BaseUrl: "https://generativelanguage.googleapis.com/v1beta/openai/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "OpenAICompat",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeOpenaiCompat),
				Name:    "openai-compat",
				BaseUrl: "https://compat.example.com/v1/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "OpenRouter",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeOpenrouter),
				Name:    "openrouter",
				BaseUrl: "https://openrouter.ai/api/v1/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "Vercel",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeVercel),
				Name:    "vercel",
				BaseUrl: "https://api.v0.dev/v1/",
			},
			expectedType: aibridge.ProviderOpenAI,
		},
		{
			name: "Bedrock",
			provider: &proto.AIProvider{
				Enabled: true,
				Type:    string(database.AIProviderTypeBedrock),
				Name:    "bedrock",
				BaseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com/",
				Bedrock: &proto.AIProviderKindBedrock{
					Region:          "us-east-1",
					AccessKey:       "AKID",
					AccessKeySecret: "secret",
					Model:           "anthropic.claude-3-5-sonnet-20241022-v2:0",
					SmallFastModel:  "anthropic.claude-3-5-haiku-20241022-v1:0",
				},
			},
			expectedType: aibridge.ProviderAnthropic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider, err := buildProvider(t.Context(), protoToProviderSpec(tt.provider), codersdk.AIBridgeConfig{
				AllowBYOK:  serpent.Bool(true),
				APIDumpDir: serpent.String(dumpDir),
			}, nil)
			require.NoError(t, err)
			assert.Equal(t, dumpDir, provider.APIDumpDir())
			assert.Equal(t, tt.expectedType, provider.Type())
		})
	}
}

func TestBuildProviderFromProtoBedrockWithoutSettings(t *testing.T) {
	t.Parallel()

	_, err := buildProvider(t.Context(), protoToProviderSpec(&proto.AIProvider{
		Enabled: true,
		Type:    string(database.AIProviderTypeBedrock),
		Name:    "bedrock-no-settings",
		BaseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com/",
	}), codersdk.AIBridgeConfig{
		AllowBYOK: serpent.Bool(true),
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bedrock provider has no bedrock credentials configured")
}
