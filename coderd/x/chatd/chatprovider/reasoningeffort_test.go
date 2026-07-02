package chatprovider_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

func TestNormalizeGlobalReasoningEffort(t *testing.T) {
	t.Parallel()

	require.Nil(t, chatprovider.NormalizeGlobalReasoningEffort(nil))
	require.Nil(t, chatprovider.NormalizeGlobalReasoningEffort(ptr.Ref("")))
	require.Nil(t, chatprovider.NormalizeGlobalReasoningEffort(ptr.Ref("extreme")))

	got := chatprovider.NormalizeGlobalReasoningEffort(ptr.Ref(" HIGH "))
	require.NotNil(t, got)
	require.Equal(t, "high", *got)
}

func TestResolveReasoningEffort(t *testing.T) {
	t.Parallel()

	config := func(defaultEffort, maxEffort string) *codersdk.ChatModelReasoningEffortConfig {
		cfg := &codersdk.ChatModelReasoningEffortConfig{}
		if defaultEffort != "" {
			cfg.Default = ptr.Ref(defaultEffort)
		}
		if maxEffort != "" {
			cfg.Max = ptr.Ref(maxEffort)
		}
		return cfg
	}

	tests := []struct {
		name      string
		provider  string
		requested *string
		config    *codersdk.ChatModelReasoningEffortConfig
		want      *string
	}{
		{
			name:      "NilConfigIgnoresRequested",
			provider:  "openai",
			requested: ptr.Ref("high"),
			config:    nil,
			want:      nil,
		},
		{
			name:     "DefaultUsedWhenNoRequested",
			provider: "openai",
			config:   config("medium", "high"),
			want:     ptr.Ref("medium"),
		},
		{
			name:      "RequestedWinsOverDefault",
			provider:  "openai",
			requested: ptr.Ref("high"),
			config:    config("medium", "high"),
			want:      ptr.Ref("high"),
		},
		{
			name:      "RequestedClampedToMax",
			provider:  "openai",
			requested: ptr.Ref("xhigh"),
			config:    config("low", "medium"),
			want:      ptr.Ref("medium"),
		},
		{
			name:      "InvalidRequestedFallsBackToDefault",
			provider:  "openai",
			requested: ptr.Ref("extreme"),
			config:    config("low", "high"),
			want:      ptr.Ref("low"),
		},
		{
			name:     "EmptyConfigReturnsNil",
			provider: "openai",
			config:   &codersdk.ChatModelReasoningEffortConfig{},
			want:     nil,
		},
		{
			name:      "BelowProviderMinimumClampsUp",
			provider:  "anthropic",
			requested: ptr.Ref("minimal"),
			config:    config("medium", "max"),
			want:      ptr.Ref("low"),
		},
		{
			name:      "AboveProviderMaximumSnapsDown",
			provider:  "openrouter",
			requested: ptr.Ref("max"),
			config:    config("medium", "max"),
			want:      ptr.Ref("high"),
		},
		{
			name:      "AnthropicMaxSupported",
			provider:  "anthropic",
			requested: ptr.Ref("max"),
			config:    config("medium", "max"),
			want:      ptr.Ref("max"),
		},
		{
			name:      "BedrockSharesAnthropicSet",
			provider:  "bedrock",
			requested: ptr.Ref("xhigh"),
			config:    config("medium", "xhigh"),
			want:      ptr.Ref("xhigh"),
		},
		{
			name:      "AzureSharesOpenAISet",
			provider:  "azure",
			requested: ptr.Ref("minimal"),
			config:    config("medium", "xhigh"),
			want:      ptr.Ref("minimal"),
		},
		{
			name:      "OpenAISnapsMaxToXHigh",
			provider:  "openai",
			requested: ptr.Ref("max"),
			config:    config("medium", "max"),
			want:      ptr.Ref("xhigh"),
		},
		{
			name:      "VercelNoneSupported",
			provider:  "vercel",
			requested: ptr.Ref("none"),
			config:    config("medium", "xhigh"),
			want:      ptr.Ref("none"),
		},
		{
			name:      "GoogleUnsupportedReturnsNil",
			provider:  "google",
			requested: ptr.Ref("high"),
			config:    config("medium", "high"),
			want:      nil,
		},
		{
			name:      "UnknownProviderReturnsNil",
			provider:  "copilot",
			requested: ptr.Ref("high"),
			config:    config("medium", "high"),
			want:      nil,
		},
		{
			name:      "RequestedNormalized",
			provider:  "openai",
			requested: ptr.Ref(" HIGH "),
			config:    config("medium", "xhigh"),
			want:      ptr.Ref("high"),
		},
		{
			name:      "MaxOnlyConfigClampsRequested",
			provider:  "openai",
			requested: ptr.Ref("xhigh"),
			config:    config("", "medium"),
			want:      ptr.Ref("medium"),
		},
		{
			name:     "MaxOnlyConfigWithoutRequestedReturnsNil",
			provider: "openai",
			config:   config("", "medium"),
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatprovider.ResolveReasoningEffort(tt.provider, tt.requested, tt.config)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}
