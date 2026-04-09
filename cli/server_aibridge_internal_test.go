package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/testutil"
)

func TestReadAIBridgeProvidersFromEnv(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"HOME=/home/frodo",
		})
		require.NoError(t, err)
		require.Empty(t, providers)
	})

	t.Run("SingleProvider", func(t *testing.T) {
		t.Parallel()
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-zdr",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-ant-xxx",
			"CODER_AIBRIDGE_PROVIDER_0_BASE_URL=https://api.anthropic.com/",
		})
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, aibridge.ProviderAnthropic, providers[0].Type)
		assert.Equal(t, "anthropic-zdr", providers[0].Name)
		assert.Equal(t, "sk-ant-xxx", providers[0].Key)
		assert.Equal(t, "https://api.anthropic.com/", providers[0].BaseURL)
	})

	t.Run("MultipleProvidersSameType", func(t *testing.T) {
		t.Parallel()
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-us",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-ant-us",
			"CODER_AIBRIDGE_PROVIDER_1_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_1_NAME=anthropic-eu",
			"CODER_AIBRIDGE_PROVIDER_1_KEY=sk-ant-eu",
			"CODER_AIBRIDGE_PROVIDER_1_BASE_URL=https://eu.api.anthropic.com/",
		})
		require.NoError(t, err)
		require.Len(t, providers, 2)
		assert.Equal(t, "anthropic-us", providers[0].Name)
		assert.Equal(t, "anthropic-eu", providers[1].Name)
		assert.Equal(t, "https://eu.api.anthropic.com/", providers[1].BaseURL)
	})

	t.Run("DefaultName", func(t *testing.T) {
		t.Parallel()
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-xxx",
		})
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, aibridge.ProviderOpenAI, providers[0].Name)
	})

	t.Run("MixedTypes", func(t *testing.T) {
		t.Parallel()
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-main",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-ant",
			"CODER_AIBRIDGE_PROVIDER_1_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_1_KEY=sk-oai",
			"CODER_AIBRIDGE_PROVIDER_2_TYPE=copilot",
			"CODER_AIBRIDGE_PROVIDER_2_NAME=copilot-custom",
			"CODER_AIBRIDGE_PROVIDER_2_BASE_URL=https://custom.copilot.com",
		})
		require.NoError(t, err)
		require.Len(t, providers, 3)
		assert.Equal(t, aibridge.ProviderAnthropic, providers[0].Type)
		assert.Equal(t, aibridge.ProviderOpenAI, providers[1].Type)
		assert.Equal(t, aibridge.ProviderCopilot, providers[2].Type)
		assert.Equal(t, "copilot-custom", providers[2].Name)
	})

	t.Run("BedrockFields", func(t *testing.T) {
		t.Parallel()
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_0_NAME=anthropic-bedrock",
			"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_REGION=us-west-2",
			"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY=AKID",
			"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_ACCESS_KEY_SECRET=secret",
			"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_MODEL=anthropic.claude-3-sonnet",
			"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_SMALL_FAST_MODEL=anthropic.claude-3-haiku",
			"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_BASE_URL=https://bedrock.us-west-2.amazonaws.com",
		})
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "us-west-2", providers[0].BedrockRegion)
		assert.Equal(t, "AKID", providers[0].BedrockAccessKey)
		assert.Equal(t, "secret", providers[0].BedrockAccessKeySecret)
		assert.Equal(t, "anthropic.claude-3-sonnet", providers[0].BedrockModel)
		assert.Equal(t, "anthropic.claude-3-haiku", providers[0].BedrockSmallFastModel)
		assert.Equal(t, "https://bedrock.us-west-2.amazonaws.com", providers[0].BedrockBaseURL)
	})

	t.Run("OutOfOrderIndices", func(t *testing.T) {
		t.Parallel()
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_1_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_1_KEY=sk-ant",
			"CODER_AIBRIDGE_PROVIDER_1_NAME=second",
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-oai",
			"CODER_AIBRIDGE_PROVIDER_0_NAME=first",
		})
		require.NoError(t, err)
		require.Len(t, providers, 2)
		assert.Equal(t, "first", providers[0].Name)
		assert.Equal(t, aibridge.ProviderOpenAI, providers[0].Type)
		assert.Equal(t, "second", providers[1].Name)
		assert.Equal(t, aibridge.ProviderAnthropic, providers[1].Type)
	})

	t.Run("MultiDigitIndices", func(t *testing.T) {
		t.Parallel()
		// Indices 0, 1, 2, ..., 10 — verifies that 10 sorts after 2,
		// not between 1 and 2 as a lexicographic sort would do.
		env := []string{}
		for i := range 11 {
			env = append(env,
				fmt.Sprintf("CODER_AIBRIDGE_PROVIDER_%d_TYPE=openai", i),
				fmt.Sprintf("CODER_AIBRIDGE_PROVIDER_%d_KEY=sk-%d", i, i),
				fmt.Sprintf("CODER_AIBRIDGE_PROVIDER_%d_NAME=p%d", i, i),
			)
		}
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), env)
		require.NoError(t, err)
		require.Len(t, providers, 11)
		for i, p := range providers {
			assert.Equal(t, fmt.Sprintf("p%d", i), p.Name, "provider at index %d", i)
		}
	})

	t.Run("SkippedIndex", func(t *testing.T) {
		t.Parallel()
		_, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_2_TYPE=anthropic",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skipped")
	})

	t.Run("InvalidKey", func(t *testing.T) {
		t.Parallel()
		_, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_XXX_TYPE=openai",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse number")
	})

	t.Run("MissingType", func(t *testing.T) {
		t.Parallel()
		_, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_NAME=my-provider",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-xxx",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TYPE is required")
	})

	t.Run("InvalidType", func(t *testing.T) {
		t.Parallel()
		_, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=gemini",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown TYPE")
	})

	t.Run("DuplicateExplicitNames", func(t *testing.T) {
		t.Parallel()
		_, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_0_NAME=my-provider",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-1",
			"CODER_AIBRIDGE_PROVIDER_1_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_1_NAME=my-provider",
			"CODER_AIBRIDGE_PROVIDER_1_KEY=sk-2",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate NAME")
	})

	t.Run("DuplicateDefaultNames", func(t *testing.T) {
		t.Parallel()
		_, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-1",
			"CODER_AIBRIDGE_PROVIDER_1_TYPE=anthropic",
			"CODER_AIBRIDGE_PROVIDER_1_KEY=sk-2",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate NAME")
	})

	t.Run("BedrockFieldsOnNonAnthropic", func(t *testing.T) {
		t.Parallel()
		_, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-xxx",
			"CODER_AIBRIDGE_PROVIDER_0_BEDROCK_REGION=us-west-2",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BEDROCK_* fields are only supported with TYPE")
	})

	t.Run("IgnoresUnrelatedEnvVars", func(t *testing.T) {
		t.Parallel()
		providers, err := ReadAIBridgeProvidersFromEnv(slogtest.Make(t, nil), []string{
			"CODER_AIBRIDGE_OPENAI_KEY=should-be-ignored",
			"CODER_AIBRIDGE_ANTHROPIC_KEY=also-ignored",
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-xxx",
			"SOME_OTHER_VAR=hello",
		})
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "sk-xxx", providers[0].Key)
	})

	t.Run("UnknownFieldWarnsButSucceeds", func(t *testing.T) {
		t.Parallel()
		// A typo like TPYE instead of TYPE should not prevent startup;
		// the function logs a warning and continues.
		sink := testutil.NewFakeSink(t)
		providers, err := ReadAIBridgeProvidersFromEnv(sink.Logger(), []string{
			"CODER_AIBRIDGE_PROVIDER_0_TYPE=openai",
			"CODER_AIBRIDGE_PROVIDER_0_KEY=sk-xxx",
			"CODER_AIBRIDGE_PROVIDER_0_TPYE=openai",
		})
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "sk-xxx", providers[0].Key)

		warnings := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Message == "ignoring unknown aibridge provider field (check for typos)"
		})
		require.Len(t, warnings, 1)
		require.Len(t, warnings[0].Fields, 1)
		assert.Equal(t, "CODER_AIBRIDGE_PROVIDER_0_TPYE", warnings[0].Fields[0].Value)
	})
}
