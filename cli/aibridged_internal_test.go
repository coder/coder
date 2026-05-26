//go:build !slim

package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// buildFromEnv exercises the same env-config-in/providers-out path that
// production uses on boot: SeedAIProvidersFromEnv writes the env-derived
// rows to the database, and BuildProviders reads them back as runtime
// [aibridge.Provider] instances. This keeps the existing TestBuildProviders
// table intact while reflecting the post-refactor flow where the database
// is the single source of truth.
func buildFromEnv(t *testing.T, cfg codersdk.AIBridgeConfig) ([]aibridge.Provider, error) {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	if err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, logger); err != nil {
		return nil, err
	}
	return BuildProviders(ctx, db, cfg, logger)
}

func TestBuildProviders(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfig", func(t *testing.T) {
		t.Parallel()
		providers, err := buildFromEnv(t, codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		assert.Empty(t, providers)
	})

	t.Run("LegacyOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := buildFromEnv(t, cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
		assert.Len(t, names, 2)
	})

	const dumpBase = "/tmp/aibridge-dump"

	t.Run("IndexedOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			APIDumpDir: serpent.String(dumpBase),
			Providers: []codersdk.AIProviderConfig{
				{
					Type: aibridge.ProviderAnthropic,
					Name: "anthropic-zdr",
					Keys: []string{"sk-zdr"},
				},
				{
					Type:    aibridge.ProviderOpenAI,
					Name:    "openai-azure",
					Keys:    []string{"sk-azure"},
					BaseURL: "https://azure.openai.com",
				},
			},
		}

		providers, err := buildFromEnv(t, cfg)
		require.NoError(t, err)
		require.Len(t, providers, 2)

		byName := make(map[string]aibridge.Provider, len(providers))
		for _, p := range providers {
			byName[p.Name()] = p
		}
		require.Contains(t, byName, "anthropic-zdr")
		require.Contains(t, byName, "openai-azure")
		assert.Equal(t, filepath.Join(dumpBase, "anthropic-zdr"), byName["anthropic-zdr"].APIDumpDir())
		assert.Equal(t, filepath.Join(dumpBase, "openai-azure"), byName["openai-azure"].APIDumpDir())
	})

	t.Run("LegacyOpenAIConflictsWithIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI, Keys: []string{"sk-indexed"}},
			},
		}
		cfg.LegacyOpenAI.Key = serpent.String("sk-legacy")

		_, err := buildFromEnv(t, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with the legacy env var")
	})

	t.Run("LegacyAnthropicConflictsWithIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: aibridge.ProviderAnthropic, Keys: []string{"sk-indexed"}},
			},
		}
		cfg.LegacyAnthropic.Key = serpent.String("sk-legacy")

		_, err := buildFromEnv(t, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with the legacy env var")
	})

	t.Run("MixedLegacyAndIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: "anthropic-zdr", Keys: []string{"sk-zdr"}},
			},
		}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := buildFromEnv(t, cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
		assert.Contains(t, names, "anthropic-zdr")
	})

	t.Run("LegacyAnthropicWithBedrock", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")
		cfg.LegacyBedrock.Region = serpent.String("us-west-2")
		cfg.LegacyBedrock.AccessKey = serpent.String("AKID")
		cfg.LegacyBedrock.AccessKeySecret = serpent.String("secret")

		providers, err := buildFromEnv(t, cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Equal(t, []string{aibridge.ProviderAnthropic}, names)
	})

	t.Run("LegacyBedrockWithoutAnthropicKey", func(t *testing.T) {
		t.Parallel()
		// Bedrock credentials alone should be enough to create an
		// Anthropic provider — no CODER_AIBRIDGE_ANTHROPIC_KEY needed.
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyBedrock.Region = serpent.String("us-west-2")
		cfg.LegacyBedrock.AccessKey = serpent.String("AKID")
		cfg.LegacyBedrock.AccessKeySecret = serpent.String("secret")

		providers, err := buildFromEnv(t, cfg)
		require.NoError(t, err)
		require.Len(t, providers, 1)

		p := providers[0]
		assert.Equal(t, aibridge.ProviderAnthropic, p.Type())
		assert.Equal(t, aibridge.ProviderAnthropic, p.Name())
	})

	t.Run("UnknownType", func(t *testing.T) {
		t.Parallel()
		// Unknown provider types are dropped by the seed step (logged
		// and skipped) so one misconfigured row cannot stop the daemon
		// from starting. The end state is "no providers", not an error.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: "gemini", Name: "gemini-pro"},
			},
		}

		providers, err := buildFromEnv(t, cfg)
		require.NoError(t, err)
		assert.Empty(t, providers)
	})

	t.Run("CopilotVariants", func(t *testing.T) {
		t.Parallel()
		// Copilot providers can target any of the three GitHub
		// Copilot API hosts via an explicit BASE_URL. The dump
		// directory comes from the top-level base + provider name.
		cfg := codersdk.AIBridgeConfig{
			APIDumpDir: serpent.String(dumpBase),
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderCopilot, Name: aibridge.ProviderCopilot},
				{Type: aibridge.ProviderCopilot, Name: agplaibridge.ProviderCopilotBusiness, BaseURL: "https://" + agplaibridge.HostCopilotBusiness},
				{Type: aibridge.ProviderCopilot, Name: agplaibridge.ProviderCopilotEnterprise, BaseURL: "https://" + agplaibridge.HostCopilotEnterprise},
			},
		}

		providers, err := buildFromEnv(t, cfg)
		require.NoError(t, err)
		require.Len(t, providers, 3)

		byName := make(map[string]aibridge.Provider, len(providers))
		for _, p := range providers {
			byName[p.Name()] = p
		}
		require.Contains(t, byName, aibridge.ProviderCopilot)
		require.Contains(t, byName, agplaibridge.ProviderCopilotBusiness)
		require.Contains(t, byName, agplaibridge.ProviderCopilotEnterprise)
		assert.Equal(t, filepath.Join(dumpBase, aibridge.ProviderCopilot), byName[aibridge.ProviderCopilot].APIDumpDir())
		assert.Equal(t, "https://"+agplaibridge.HostCopilotBusiness, byName[agplaibridge.ProviderCopilotBusiness].BaseURL())
		assert.Equal(t, "https://"+agplaibridge.HostCopilotEnterprise, byName[agplaibridge.ProviderCopilotEnterprise].BaseURL())
	})

	t.Run("ChatGPTProvider", func(t *testing.T) {
		t.Parallel()
		// ChatGPT is an OpenAI-compatible provider with a custom
		// base URL. Admins configure it as an indexed openai provider.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: agplaibridge.ProviderChatGPT, Keys: []string{"sk-chatgpt"}, BaseURL: agplaibridge.BaseURLChatGPT},
			},
		}

		providers, err := buildFromEnv(t, cfg)
		require.NoError(t, err)
		require.Len(t, providers, 1)

		assert.Equal(t, agplaibridge.ProviderChatGPT, providers[0].Name())
		assert.Equal(t, agplaibridge.BaseURLChatGPT, providers[0].BaseURL())
	})

	t.Run("NativeAnthropicDefaultBaseURL", func(t *testing.T) {
		t.Parallel()
		row := database.AIProvider{
			Type:    database.AiProviderTypeAnthropic,
			Name:    aibridge.ProviderAnthropic,
			BaseUrl: "https://api.anthropic.com/",
		}
		assert.Nil(t, bedrockConfigFromRow(row, codersdk.AIProviderSettings{}))
	})

	t.Run("NativeAnthropicCustomBaseURL", func(t *testing.T) {
		t.Parallel()
		row := database.AIProvider{
			Type:    database.AiProviderTypeAnthropic,
			Name:    "anthropic-proxy",
			BaseUrl: "https://internal-proxy.example.com/anthropic/",
		}
		assert.Nil(t, bedrockConfigFromRow(row, codersdk.AIProviderSettings{}))
	})

	t.Run("BedrockSettingsPresent", func(t *testing.T) {
		t.Parallel()
		accessKey := "AKID"
		secret := "secret"
		model := "anthropic.claude-3-5-sonnet-20241022-v2:0"
		smallModel := "anthropic.claude-3-5-haiku-20241022-v1:0"
		row := database.AIProvider{
			Type:    database.AiProviderTypeAnthropic,
			Name:    "anthropic-bedrock",
			BaseUrl: "https://bedrock-runtime.us-west-2.amazonaws.com/",
		}
		settings := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-west-2",
				AccessKey:       &accessKey,
				AccessKeySecret: &secret,
				Model:           model,
				SmallFastModel:  smallModel,
			},
		}
		got := bedrockConfigFromRow(row, settings)
		require.NotNil(t, got)
		assert.Equal(t, row.BaseUrl, got.BaseURL)
		assert.Equal(t, "us-west-2", got.Region)
		assert.Equal(t, accessKey, got.AccessKey)
		assert.Equal(t, secret, got.AccessKeySecret)
		assert.Equal(t, model, got.Model)
		assert.Equal(t, smallModel, got.SmallFastModel)
	})
}

func providerNames(providers []aibridge.Provider) []string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name()
	}
	return names
}
