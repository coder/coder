package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestReadAIBridgeProvidersFromEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		env         []string
		expected    []codersdk.AIBridgeProviderConfig
		errContains string
	}{
		{
			name: "Empty",
			env:  []string{"HOME=/home/frodo"},
		},
		{
			name: "SingleProvider",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-zdr",
				"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-ant-xxx",
				"CODER_AIBRIDGE_PROVIDER_0_BASE_URL=https://api.anthropic.com/",
				"CODER_AIBRIDGE_PROVIDER_0_DUMP_DIR=/tmp/aibridge-dump",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{
					Type:    aibridge.ProviderAnthropic,
					Name:    "anthropic-zdr",
					Keys:    []string{"sk-ant-xxx"},
					BaseURL: "https://api.anthropic.com/",
					DumpDir: "/tmp/aibridge-dump",
				},
			},
		},
		{
			name: "MultipleProvidersSameType",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-us",
				"CODER_AIBRIDGE_PROVIDER_1_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_1_NAME=anthropic-eu",
				"CODER_AIBRIDGE_PROVIDER_1_BASE_URL=https://eu.api.anthropic.com/",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: "anthropic-us"},
				{Type: aibridge.ProviderAnthropic, Name: "anthropic-eu", BaseURL: "https://eu.api.anthropic.com/"},
			},
		},
		{
			name: "DefaultName",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI},
			},
		},
		{
			name: "MixedTypes",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-main",
				"CODER_AIBRIDGE_PROVIDER_1_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_2_TYPE=copilot",
				"CODER_AIBRIDGE_PROVIDER_2_NAME=copilot-custom",
				"CODER_AIBRIDGE_PROVIDER_2_BASE_URL=https://custom.copilot.com",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: "anthropic-main"},
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI},
				{Type: aibridge.ProviderCopilot, Name: "copilot-custom", BaseURL: "https://custom.copilot.com"},
			},
		},
		{
			name: "BedrockFields",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-bedrock",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_REGION=us-west-2",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY=AKID",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY_SECRET=secret",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_MODEL=anthropic.claude-3-sonnet",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_SMALL_FAST_MODEL=anthropic.claude-3-haiku",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_BASE_URL=https://bedrock.us-west-2.amazonaws.com",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{
					Type:                    aibridge.ProviderAnthropic,
					Name:                    "anthropic-bedrock",
					BedrockRegion:           "us-west-2",
					BedrockAccessKeys:       []string{"AKID"},
					BedrockAccessKeySecrets: []string{"secret"},
					BedrockModel:            "anthropic.claude-3-sonnet",
					BedrockSmallFastModel:   "anthropic.claude-3-haiku",
					BedrockBaseURL:          "https://bedrock.us-west-2.amazonaws.com",
				},
			},
		},
		{
			name: "OutOfOrderIndices",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_1_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_1_NAME=second",
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_0_NAME=first",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: "first"},
				{Type: aibridge.ProviderAnthropic, Name: "second"},
			},
		},
		{
			name:        "SkippedIndex",
			env:         []string{"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai", "CODER_AIBRIDGE_PROVIDER_2_TYPE=anthropic"},
			errContains: "skipped",
		},
		{
			name:        "InvalidKey",
			env:         []string{"CODER_AIBRIDGE_PROVIDER_XXX_TYPE=openai"},
			errContains: "parse number",
		},
		{
			name:        "MissingType",
			env:         []string{"CODER_AIBRIDGE_PROVIDER_0_NAME=my-provider", "CODER_AIBRIDGE_PROVIDER_0_KEY=sk-xxx"},
			errContains: "TYPE is required",
		},
		{
			name:        "InvalidType",
			env:         []string{"CODER_AIBRIDGE_PROVIDER_0_TYPE=gemini"},
			errContains: "unknown TYPE",
		},
		{
			name: "DuplicateExplicitNames",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_NAME=my-provider",
				"CODER_AIBRIDGE_PROVIDER_1_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_1_NAME=my-provider",
			},
			errContains: "duplicate NAME",
		},
		{
			name:        "DuplicateDefaultNames",
			env:         []string{"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic", "CODER_AIBRIDGE_PROVIDER_1_TYPE=anthropic"},
			errContains: "duplicate NAME",
		},
		{
			name:        "BedrockFieldsOnNonAnthropic",
			env:         []string{"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai", "CODER_AIBRIDGE_PROVIDER_0_BEDROCK_REGION=us-west-2"},
			errContains: "BEDROCK_* fields are only supported with TYPE",
		},
		{
			name: "IgnoresUnrelatedEnvVars",
			env: []string{
				"CODER_AIBRIDGE_OPENAI_KEY=should-be-ignored",
				"CODER_AIBRIDGE_ANTHROPIC_KEY=also-ignored",
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-xxx",
				"SOME_OTHER_VAR=hello",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI, Keys: []string{"sk-xxx"}},
			},
		},
		{
			// KEYS, BEDROCK_ACCESS_KEYS, and BEDROCK_ACCESS_KEY_SECRETS
			// are plural aliases for their singular counterparts.
			name: "PluralKeyAliases",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_KEYS=sk-ant-xxx",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEYS=AKID",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY_SECRETS=secret",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{
					Type:                    aibridge.ProviderAnthropic,
					Name:                    aibridge.ProviderAnthropic,
					Keys:                    []string{"sk-ant-xxx"},
					BedrockAccessKeys:       []string{"AKID"},
					BedrockAccessKeySecrets: []string{"secret"},
				},
			},
		},
		{
			name: "ConflictKeyAndKeys",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-single",
				"CODER_AIBRIDGE_PROVIDER_0_KEYS=sk-multi",
			},
			errContains: "KEY and KEYS are mutually exclusive",
		},
		{
			name: "ConflictBedrockAccessKeyAndKeys",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY=AKID1",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEYS=AKID2",
			},
			errContains: "BEDROCK_ACCESS_KEY and BEDROCK_ACCESS_KEYS are mutually exclusive",
		},
		{
			name: "ConflictBedrockSecretAndSecrets",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY_SECRET=s1",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY_SECRETS=s2",
			},
			errContains: "BEDROCK_ACCESS_KEY_SECRET and BEDROCK_ACCESS_KEY_SECRETS are mutually exclusive",
		},
		{
			name: "CopilotRejectsKey",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=copilot",
				"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-xxx",
			},
			errContains: "KEY/KEYS are not supported for TYPE",
		},
		{
			name: "CopilotRejectsKeys",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=copilot",
				"CODER_AIBRIDGE_PROVIDER_0_KEYS=sk-a,sk-b",
			},
			errContains: "KEY/KEYS are not supported for TYPE",
		},
		{
			name: "MultipleKeysCommaSeparated",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_0_KEYS=sk-a,sk-b,sk-c",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI, Keys: []string{"sk-a", "sk-b", "sk-c"}},
			},
		},
		{
			name: "KeysWhitespaceTrimmed",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_0_KEYS= sk-a , sk-b ",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI, Keys: []string{"sk-a", "sk-b"}},
			},
		},
		{
			name: "KeysEmptyAfterTrim",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_0_KEYS=sk-a,,sk-b",
			},
			errContains: "key at index 1 is empty",
		},
		{
			name: "KeysDuplicate",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_0_KEYS=sk-a,sk-b,sk-a",
			},
			errContains: "duplicate key at index 2",
		},
		{
			name: "KeysTooMany",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
				"CODER_AIBRIDGE_PROVIDER_0_KEYS=sk-1,sk-2,sk-3,sk-4,sk-5,sk-6",
			},
			errContains: "too many keys (6), maximum is 5",
		},
		{
			name: "BedrockMultipleKeys",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_REGION=us-west-2",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEYS=AKID1,AKID2",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY_SECRETS=secret1,secret2",
			},
			expected: []codersdk.AIBridgeProviderConfig{
				{
					Type:                    aibridge.ProviderAnthropic,
					Name:                    aibridge.ProviderAnthropic,
					BedrockRegion:           "us-west-2",
					BedrockAccessKeys:       []string{"AKID1", "AKID2"},
					BedrockAccessKeySecrets: []string{"secret1", "secret2"},
				},
			},
		},
		{
			name: "BedrockKeyCountMismatch",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEYS=AKID1,AKID2",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY_SECRET=secret1",
			},
			errContains: "BEDROCK_ACCESS_KEYS count (2) must match BEDROCK_ACCESS_KEY_SECRETS count (1)",
		},
		{
			name: "BedrockKeysTooMany",
			env: []string{
				"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEYS=AKID1,AKID2,AKID3,AKID4,AKID5,AKID6",
				"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY_SECRETS=s1,s2,s3,s4,s5,s6",
			},
			errContains: "too many keys (6), maximum is 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), tt.env)
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, providers)
		})
	}

	// Cases below need special setup that doesn't fit the table above.

	t.Run("MultiDigitIndices", func(t *testing.T) {
		t.Parallel()
		// Indices 0, 1, 2, ..., 10 — verifies that 10 sorts after 2,
		// not between 1 and 2 as a lexicographic sort would do.
		var env []string
		var expected []codersdk.AIBridgeProviderConfig
		for i := range 11 {
			env = append(env,
				fmt.Sprintf("CODER_AIBRIDGE_PROVIDER_%d_TYPE=openai", i),
				fmt.Sprintf("CODER_AIBRIDGE_PROVIDER_%d_KEY=sk-%d", i, i),
				fmt.Sprintf("CODER_AIBRIDGE_PROVIDER_%d_NAME=p%d", i, i),
			)
			expected = append(expected, codersdk.AIBridgeProviderConfig{
				Type: aibridge.ProviderOpenAI,
				Name: fmt.Sprintf("p%d", i),
				Keys: []string{fmt.Sprintf("sk-%d", i)},
			})
		}
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), env)
		require.NoError(t, err)
		require.Equal(t, expected, providers)
	})

	t.Run("UnknownFieldWarnsButSucceeds", func(t *testing.T) {
		t.Parallel()
		// A typo like TPYE instead of TYPE should not prevent startup;
		// the function logs a warning and continues.
		sink := testutil.NewFakeSink(t)
		providers, err := ReadAIBridgeProvidersFromEnv(sink.Logger(), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_0_TPYE=openai",
		})
		require.NoError(t, err)
		require.Equal(t, []codersdk.AIBridgeProviderConfig{
			{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI},
		}, providers)

		warnings := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Message == "ignoring unknown aibridge provider field (check for typos)"
		})
		require.Len(t, warnings, 1)
		require.Len(t, warnings[0].Fields, 1)
		assert.Equal(t, "CODER_AIBRIDGE_PROVIDER_0_TPYE", warnings[0].Fields[0].Value)
	})
}
