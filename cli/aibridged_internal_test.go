//go:build !slim

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func testLogger(t *testing.T) slog.Logger {
	t.Helper()
	return slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
}

func TestBuildProviders(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfig", func(t *testing.T) {
		t.Parallel()
		providers, err := BuildProviders(codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		assert.Empty(t, providers)
	})

	t.Run("LegacyOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
		assert.Len(t, names, 2)
	})

	t.Run("IndexedOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    aibridge.ProviderAnthropic,
					Name:    "anthropic-zdr",
					Keys:    []string{"sk-zdr"},
					DumpDir: "/tmp/anthropic-dump",
				},
				{
					Type:    aibridge.ProviderOpenAI,
					Name:    "openai-azure",
					Keys:    []string{"sk-azure"},
					BaseURL: "https://azure.openai.com",
					DumpDir: "/tmp/openai-dump",
				},
			},
		}

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Equal(t, []string{"anthropic-zdr", "openai-azure"}, names)
		assert.Equal(t, "/tmp/anthropic-dump", providers[0].APIDumpDir())
		assert.Equal(t, "/tmp/openai-dump", providers[1].APIDumpDir())
	})

	t.Run("LegacyOpenAIConflictsWithIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI, Keys: []string{"sk-indexed"}},
			},
		}
		cfg.LegacyOpenAI.Key = serpent.String("sk-legacy")

		_, err := BuildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with indexed provider")
	})

	t.Run("LegacyAnthropicConflictsWithIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: aibridge.ProviderAnthropic, Keys: []string{"sk-indexed"}},
			},
		}
		cfg.LegacyAnthropic.Key = serpent.String("sk-legacy")

		_, err := BuildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with indexed provider")
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

		providers, err := BuildProviders(cfg)
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

		providers, err := BuildProviders(cfg)
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

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)
		require.Len(t, providers, 1)

		p := providers[0]
		assert.Equal(t, aibridge.ProviderAnthropic, p.Type())
		assert.Equal(t, aibridge.ProviderAnthropic, p.Name())
	})

	t.Run("UnknownType", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: "gemini", Name: "gemini-pro"},
			},
		}

		_, err := BuildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown provider type")
	})

	t.Run("CopilotVariants", func(t *testing.T) {
		t.Parallel()
		// Copilot providers can target any of the three GitHub
		// Copilot API hosts via an explicit BASE_URL.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderCopilot, Name: aibridge.ProviderCopilot, DumpDir: "/tmp/copilot-dump"},
				{Type: aibridge.ProviderCopilot, Name: agplaibridge.ProviderCopilotBusiness, BaseURL: "https://" + agplaibridge.HostCopilotBusiness},
				{Type: aibridge.ProviderCopilot, Name: agplaibridge.ProviderCopilotEnterprise, BaseURL: "https://" + agplaibridge.HostCopilotEnterprise},
			},
		}

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)
		require.Len(t, providers, 3)

		assert.Equal(t, aibridge.ProviderCopilot, providers[0].Name())
		assert.Equal(t, "/tmp/copilot-dump", providers[0].APIDumpDir())
		assert.Equal(t, agplaibridge.ProviderCopilotBusiness, providers[1].Name())
		assert.Equal(t, "https://"+agplaibridge.HostCopilotBusiness, providers[1].BaseURL())
		assert.Equal(t, agplaibridge.ProviderCopilotEnterprise, providers[2].Name())
		assert.Equal(t, "https://"+agplaibridge.HostCopilotEnterprise, providers[2].BaseURL())
	})

	t.Run("ChatGPTProvider", func(t *testing.T) {
		t.Parallel()
		// ChatGPT is an OpenAI-compatible provider with a custom
		// base URL. Admins configure it as an indexed openai provider.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: agplaibridge.ProviderChatGPT, BaseURL: agplaibridge.BaseURLChatGPT},
			},
		}

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)
		require.Len(t, providers, 1)

		assert.Equal(t, agplaibridge.ProviderChatGPT, providers[0].Name())
		assert.Equal(t, agplaibridge.BaseURLChatGPT, providers[0].BaseURL())
	})
}

func providerNames(providers []aibridge.Provider) []string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name()
	}
	return names
}

func TestBuildProvidersFromDB(t *testing.T) {
	t.Parallel()

	t.Run("EmptyDB", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		providers, err := BuildProvidersFromDB(context.Background(), testLogger(t), db, codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		assert.Empty(t, providers)
	})

	t.Run("OpenAIProviderWithKeys", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		prov, err := db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeOpenai,
			Name:    "my-openai",
			Enabled: true,
			BaseUrl: "https://api.openai.com",
		})
		require.NoError(t, err)

		_, err = db.InsertAIProviderKey(ctx, database.InsertAIProviderKeyParams{
			ID:         uuid.New(),
			ProviderID: prov.ID,
			APIKey:     "sk-test-key-1",
		})
		require.NoError(t, err)

		_, err = db.InsertAIProviderKey(ctx, database.InsertAIProviderKeyParams{
			ID:         uuid.New(),
			ProviderID: prov.ID,
			APIKey:     "sk-test-key-2",
		})
		require.NoError(t, err)

		providers, err := BuildProvidersFromDB(ctx, testLogger(t), db, codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		require.Len(t, providers, 1)

		p := providers[0]
		assert.Equal(t, "my-openai", p.Name())
		assert.Equal(t, aibridge.ProviderOpenAI, p.Type())
		assert.Equal(t, "https://api.openai.com", p.BaseURL())
		assert.Equal(t, prov.ID, p.ID())
	})

	t.Run("AnthropicProviderNoKeys", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		prov, err := db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeAnthropic,
			Name:    "my-anthropic",
			Enabled: true,
			BaseUrl: "https://api.anthropic.com",
		})
		require.NoError(t, err)

		providers, err := BuildProvidersFromDB(ctx, testLogger(t), db, codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		require.Len(t, providers, 1)

		p := providers[0]
		assert.Equal(t, "my-anthropic", p.Name())
		assert.Equal(t, aibridge.ProviderAnthropic, p.Type())
		assert.Equal(t, prov.ID, p.ID())
	})

	t.Run("AnthropicProviderWithBedrock", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		accessKey := "AKID"
		accessKeySecret := "secret"
		settingsObj := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-west-2",
				AccessKey:       &accessKey,
				AccessKeySecret: &accessKeySecret,
				Model:           "claude-v2",
			},
		}
		settingsJSON, err := json.Marshal(settingsObj)
		require.NoError(t, err)

		_, err = db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:       uuid.New(),
			Type:     database.AiProviderTypeAnthropic,
			Name:     "bedrock-anthropic",
			Enabled:  true,
			BaseUrl:  "https://bedrock.us-west-2.amazonaws.com",
			Settings: sql.NullString{String: string(settingsJSON), Valid: true},
		})
		require.NoError(t, err)

		providers, err := BuildProvidersFromDB(ctx, testLogger(t), db, codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		require.Len(t, providers, 1)

		p := providers[0]
		assert.Equal(t, "bedrock-anthropic", p.Name())
		assert.Equal(t, aibridge.ProviderAnthropic, p.Type())
	})

	t.Run("MultipleProviders", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		_, err := db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeOpenai,
			Name:    "openai-1",
			Enabled: true,
		})
		require.NoError(t, err)

		_, err = db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeAnthropic,
			Name:    "anthropic-1",
			Enabled: true,
		})
		require.NoError(t, err)

		providers, err := BuildProvidersFromDB(ctx, testLogger(t), db, codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		require.Len(t, providers, 2)

		names := providerNames(providers)
		assert.Contains(t, names, "openai-1")
		assert.Contains(t, names, "anthropic-1")
	})

	t.Run("DisabledProviderExcluded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		_, err := db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeOpenai,
			Name:    "enabled-provider",
			Enabled: true,
		})
		require.NoError(t, err)

		_, err = db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeAnthropic,
			Name:    "disabled-provider",
			Enabled: false,
		})
		require.NoError(t, err)

		providers, err := BuildProvidersFromDB(ctx, testLogger(t), db, codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "enabled-provider", providers[0].Name())
	})

	t.Run("UnsupportedProviderTypeIsSkipped", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := context.Background()

		_, err := db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeGoogle,
			Name:    "google-provider",
			Enabled: true,
		})
		require.NoError(t, err)
		_, err = db.InsertAIProvider(ctx, database.InsertAIProviderParams{
			ID:      uuid.New(),
			Type:    database.AiProviderTypeOpenai,
			Name:    "openai-provider",
			Enabled: true,
		})
		require.NoError(t, err)

		// Unsupported types must not block startup; the supported provider
		// continues to load.
		providers, err := BuildProvidersFromDB(ctx, testLogger(t), db, codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "openai-provider", providers[0].Name())
	})
}
