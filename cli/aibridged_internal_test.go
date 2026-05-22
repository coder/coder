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
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
)

func testLogger(t *testing.T) slog.Logger {
	t.Helper()
	return slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
}

func providerNames(providers []aibridge.Provider) []string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name()
	}
	return names
}

// providerKeyCount returns the number of API keys the provider's failover
// pool holds, or 0 when no pool is configured.
func providerKeyCount(t *testing.T, p aibridge.Provider) int {
	t.Helper()
	pool := p.KeyFailoverConfig(testLogger(t)).Pool
	if pool == nil {
		return 0
	}
	return len(pool.PoolState())
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
		assert.Equal(t, 2, providerKeyCount(t, p), "both seeded keys must be provisioned into the failover pool")
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
		assert.Zero(t, providerKeyCount(t, p), "no keys were seeded so the failover pool must be empty")
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
