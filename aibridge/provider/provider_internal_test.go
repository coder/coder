package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/aibridge/config"
)

func TestProvider_TypeAndName(t *testing.T) {
	t.Parallel()

	bedrockCfg := config.AWSBedrock{
		Region:          "us-west-2",
		AccessKey:       "test-key",
		AccessKeySecret: "test-secret",
		Model:           "m",
		SmallFastModel:  "s",
	}

	tests := []struct {
		name       string
		provider   func(t testing.TB) Provider
		expectType string
		expectName string
	}{
		{
			name:       "anthropic_defaults",
			provider:   func(t testing.TB) Provider { return newTestAnthropic(t, config.Anthropic{}, nil) },
			expectType: config.ProviderAnthropic,
			expectName: config.ProviderAnthropic,
		},
		{
			name: "anthropic_custom_name",
			provider: func(t testing.TB) Provider {
				return newTestAnthropic(t, config.Anthropic{Name: "anthropic-custom"}, nil)
			},
			expectType: config.ProviderAnthropic,
			expectName: "anthropic-custom",
		},
		{
			name:       "bedrock_defaults",
			provider:   func(t testing.TB) Provider { return newTestBedrock(t, config.Anthropic{}, bedrockCfg) },
			expectType: config.ProviderBedrock,
			expectName: config.ProviderBedrock,
		},
		{
			name: "bedrock_custom_name",
			provider: func(t testing.TB) Provider {
				return newTestBedrock(t, config.Anthropic{Name: "bedrock-custom"}, bedrockCfg)
			},
			expectType: config.ProviderBedrock,
			expectName: "bedrock-custom",
		},
		{
			name:       "copilot_defaults",
			provider:   func(testing.TB) Provider { return NewCopilot(config.Copilot{}) },
			expectType: config.ProviderCopilot,
			expectName: config.ProviderCopilot,
		},
		{
			name:       "copilot_custom_name",
			provider:   func(testing.TB) Provider { return NewCopilot(config.Copilot{Name: "copilot-business"}) },
			expectType: config.ProviderCopilot,
			expectName: "copilot-business",
		},
		{
			name:       "openai_defaults",
			provider:   func(testing.TB) Provider { return NewOpenAI(config.OpenAI{}) },
			expectType: config.ProviderOpenAI,
			expectName: config.ProviderOpenAI,
		},
		{
			name:       "openai_custom_name",
			provider:   func(testing.TB) Provider { return NewOpenAI(config.OpenAI{Name: "openai-custom"}) },
			expectType: config.ProviderOpenAI,
			expectName: "openai-custom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := tc.provider(t)
			assert.Equal(t, tc.expectType, p.Type())
			assert.Equal(t, tc.expectName, p.Name())
		})
	}
}
