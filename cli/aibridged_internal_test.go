//go:build !slim

package cli

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	agplaiseats "github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

// buildFromEnv exercises the same env-config-in/providers-out path that
// production uses on boot: SeedAIProvidersFromEnv writes the env-derived
// rows to the database, the server's GetAIProviders handler reads them back
// over the (post-refactor) DB-read path and maps them to proto, and
// BuildProvidersFromProto constructs the runtime [aibridge.Provider]
// instances. This keeps the existing TestBuildProviders table intact while
// reflecting the post-refactor flow where the database is the single source
// of truth and the gateway fetches providers over DRPC.
func buildFromEnv(t *testing.T, cfg codersdk.AIBridgeConfig) ([]aibridge.Provider, error) {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	if err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, logger); err != nil {
		return nil, err
	}
	providers, _, err := buildFromDB(ctx, t, db, cfg, logger)
	return providers, err
}

// buildFromDB runs the production fetch path against a database: it calls the
// server's GetAIProviders handler (DB read + proto mapping) and then
// BuildProvidersFromProto (proto -> runtime providers), returning the same
// (providers, outcomes) the embedded reloader would observe.
func buildFromDB(ctx context.Context, t *testing.T, db database.Store, cfg codersdk.AIBridgeConfig, logger slog.Logger) ([]aibridge.Provider, []aibridged.ProviderOutcome, error) {
	t.Helper()
	srv, err := aibridgedserver.NewServer(ctx, db, nil, logger, "/", cfg, nil, nil, agplaiseats.Noop{}, quartz.NewReal())
	if err != nil {
		return nil, nil, err
	}
	resp, err := srv.GetAIProviders(ctx, &proto.GetAIProvidersRequest{})
	if err != nil {
		return nil, nil, err
	}
	providers, outcomes := BuildProvidersFromProto(ctx, resp.GetProviders(), cfg, logger, nil)
	return providers, outcomes, nil
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

	t.Run("IndexedOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
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
		// Anthropic provider. No CODER_AIBRIDGE_ANTHROPIC_KEY needed.
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
		// Copilot API hosts via an explicit BASE_URL.
		cfg := codersdk.AIBridgeConfig{
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
			Type:    database.AIProviderTypeAnthropic,
			Name:    aibridge.ProviderAnthropic,
			BaseUrl: "https://api.anthropic.com/",
		}
		assert.Nil(t, bedrockConfig(row.BaseUrl, codersdk.AIProviderSettings{}.Bedrock))
	})

	t.Run("NativeAnthropicCustomBaseURL", func(t *testing.T) {
		t.Parallel()
		row := database.AIProvider{
			Type:    database.AIProviderTypeAnthropic,
			Name:    "anthropic-proxy",
			BaseUrl: "https://internal-proxy.example.com/anthropic/",
		}
		assert.Nil(t, bedrockConfig(row.BaseUrl, codersdk.AIProviderSettings{}.Bedrock))
	})

	t.Run("BedrockSettingsPresent", func(t *testing.T) {
		t.Parallel()
		accessKey := "AKID"
		secret := "secret"
		model := "anthropic.claude-3-5-sonnet-20241022-v2:0"
		smallModel := "anthropic.claude-3-5-haiku-20241022-v1:0"
		row := database.AIProvider{
			Type:    database.AIProviderTypeAnthropic,
			Name:    "anthropic-bedrock",
			BaseUrl: "https://bedrock-runtime.us-west-2.amazonaws.com/",
		}
		roleARN := "arn:aws:iam::123456789012:role/BedrockRole"
		settings := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-west-2",
				AccessKey:       &accessKey,
				AccessKeySecret: &secret,
				Model:           model,
				SmallFastModel:  smallModel,
				RoleARN:         roleARN,
			},
		}
		got := bedrockConfig(row.BaseUrl, settings.Bedrock)
		require.NotNil(t, got)
		assert.Equal(t, row.BaseUrl, got.BaseURL)
		assert.Equal(t, "us-west-2", got.Region)
		assert.Equal(t, accessKey, got.AccessKey)
		assert.Equal(t, secret, got.AccessKeySecret)
		assert.Equal(t, model, got.Model)
		assert.Equal(t, smallModel, got.SmallFastModel)
		assert.Equal(t, roleARN, got.RoleARN)
	})

	t.Run("BedrockSettingsEmpty", func(t *testing.T) {
		t.Parallel()
		// A non-nil but zero-valued Bedrock settings blob should not
		// produce a Bedrock config; the provider's generic BaseUrl is
		// not a Bedrock detection signal.
		row := database.AIProvider{
			Type:    database.AIProviderTypeAnthropic,
			Name:    "anthropic-empty-bedrock",
			BaseUrl: "https://api.anthropic.com/",
		}
		settings := codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{},
		}
		assert.Nil(t, bedrockConfig(row.BaseUrl, settings.Bedrock))
	})
}

// TestBuildProvidersSkipsBadRows exercises the skip-and-continue path
// directly: rows whose settings blob is malformed or whose type is not
// supported by the runtime builder are logged and excluded from the
// returned snapshot without surfacing a top-level error. The seed path
// filters most of these out before insert, so we bypass it and insert
// rows straight into the database via dbgen.
func TestBuildProvidersSkipsBadRows(t *testing.T) {
	t.Parallel()

	t.Run("CorruptSettings", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		dbgen.AIProvider(t, db, database.AIProvider{
			Type:     database.AIProviderTypeAnthropic,
			Name:     "anthropic-broken",
			BaseUrl:  "https://api.anthropic.com/",
			Settings: sql.NullString{String: "not-json", Valid: true},
		})

		// A row whose settings blob cannot be decoded is dropped server-side
		// in GetAIProviders, so it never reaches the client: no provider and
		// no outcome. This keeps one corrupt row from breaking the fetch (and
		// thus provider configuration) for every gateway.
		providers, outcomes, err := buildFromDB(ctx, t, db, codersdk.AIBridgeConfig{}, logger)
		require.NoError(t, err)
		assert.Empty(t, providers)
		assert.Empty(t, outcomes)
	})

	t.Run("EnabledButNoKeys", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		// Azure routes through the OpenAI-family builder, which rejects
		// rows without keys when BYOK is disabled. The row must be
		// classified as error and excluded from the snapshot.
		dbgen.AIProvider(t, db, database.AIProvider{
			Type:    database.AIProviderTypeAzure,
			Name:    "azure-openai",
			BaseUrl: "https://example.openai.azure.com/",
		})

		providers, outcomes, err := buildFromDB(ctx, t, db, codersdk.AIBridgeConfig{}, logger)
		require.NoError(t, err)
		assert.Empty(t, providers)
		require.Len(t, outcomes, 1)
		assert.Equal(t, aibridged.ProviderStatusError, outcomes[0].Status)
	})

	t.Run("BadRowDoesNotBlockGoodRow", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		// An enabled provider with no keys (and BYOK disabled) fails to build
		// on the client side, yielding a ProviderStatusError outcome. It must
		// not prevent the good provider from being built.
		dbgen.AIProvider(t, db, database.AIProvider{
			Type:    database.AIProviderTypeAzure,
			Name:    "azure-broken",
			BaseUrl: "https://example.openai.azure.com/",
		})
		good := dbgen.AIProvider(t, db, database.AIProvider{
			Type:    database.AIProviderTypeOpenai,
			Name:    "openai-good",
			BaseUrl: "https://api.openai.com/",
		})
		dbgen.AIProviderKey(t, db, database.AIProviderKey{
			ProviderID: good.ID,
			APIKey:     "sk-good",
		})

		providers, outcomes, err := buildFromDB(ctx, t, db, codersdk.AIBridgeConfig{}, logger)
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "openai-good", providers[0].Name())
		require.Len(t, outcomes, 2)
		byName := map[string]aibridged.ProviderOutcome{}
		for _, o := range outcomes {
			byName[o.Name] = o
		}
		assert.Equal(t, aibridged.ProviderStatusError, byName["azure-broken"].Status)
		assert.Equal(t, aibridged.ProviderStatusEnabled, byName["openai-good"].Status)
	})

	t.Run("DisabledRowClassifiedAsDisabled", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name string
			row  database.AIProvider
		}{
			{
				name: "OpenAI",
				row: database.AIProvider{
					Type:    database.AIProviderTypeOpenai,
					Name:    "openai-off",
					BaseUrl: "https://api.openai.com/",
				},
			},
			{
				// Anthropic and Bedrock have stricter credential checks
				// than the OpenAI family; the disabled short-circuit
				// must reach them too. No keys, no bedrock settings.
				name: "Anthropic",
				row: database.AIProvider{
					Type:    database.AIProviderTypeAnthropic,
					Name:    "anthropic-off",
					BaseUrl: "https://api.anthropic.com/",
				},
			},
			{
				name: "Bedrock",
				row: database.AIProvider{
					Type:    database.AIProviderTypeBedrock,
					Name:    "bedrock-off",
					BaseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com/",
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				db, _ := dbtestutil.NewDB(t)
				ctx := testutil.Context(t, testutil.WaitShort)
				logger := slogtest.Make(t, nil)

				dbgen.AIProvider(t, db, tc.row, func(p *database.InsertAIProviderParams) {
					p.Enabled = false
				})

				providers, outcomes, err := buildFromDB(ctx, t, db, codersdk.AIBridgeConfig{}, logger)
				require.NoError(t, err)
				require.Len(t, providers, 1, "disabled providers stay in the snapshot so the bridge can serve a 503 sentinel")
				assert.Equal(t, tc.row.Name, providers[0].Name())
				assert.False(t, providers[0].Enabled())
				require.Len(t, outcomes, 1)
				assert.Equal(t, tc.row.Name, outcomes[0].Name)
				assert.Equal(t, aibridged.ProviderStatusDisabled, outcomes[0].Status)
				assert.NoError(t, outcomes[0].Err)
			})
		}
	})
}

func providerNames(providers []aibridge.Provider) []string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name()
	}
	return names
}
