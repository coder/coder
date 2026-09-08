//go:build !slim

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
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
)

// buildFromDB runs the production fetch path against a database: it calls the
// server's GetAIProviders handler (DB read + proto mapping) and then
// BuildProvidersFromProto (proto -> runtime providers), returning the same
// (providers, outcomes) the embedded reloader would observe.
func buildFromDB(ctx context.Context, t *testing.T, db database.Store, logger slog.Logger) ([]aibridge.Provider, []aibridged.ProviderOutcome, error) {
	t.Helper()
	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	if err != nil {
		return nil, nil, err
	}
	resp, err := srv.GetAIProviders(ctx, &proto.GetAIProvidersRequest{})
	if err != nil {
		return nil, nil, err
	}
	providers, outcomes := BuildProvidersFromProto(ctx, resp.GetProviders(), codersdk.AIBridgeConfig{}, logger, nil)
	return providers, outcomes, nil
}

func TestBuildProviders(t *testing.T) {
	t.Parallel()

	t.Run("EmptyDatabase", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		providers, outcomes, err := buildFromDB(ctx, t, db, slogtest.Make(t, nil))
		require.NoError(t, err)
		assert.Empty(t, providers)
		assert.Empty(t, outcomes)
	})

	t.Run("DatabaseProviders", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		bedrockSettings, err := json.Marshal(codersdk.AIProviderSettings{
			Bedrock: &codersdk.AIProviderBedrockSettings{
				Region:          "us-east-1",
				AccessKey:       new("AKID"),
				AccessKeySecret: new("secret"),
				Model:           "anthropic.claude-3-5-sonnet-20241022-v2:0",
				SmallFastModel:  "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		})
		require.NoError(t, err)
		rows := []database.AIProvider{
			{Type: database.AIProviderTypeAnthropic, Name: "anthropic-zdr", BaseUrl: "https://api.anthropic.com/"},
			{Type: database.AIProviderTypeOpenai, Name: "openai-azure", BaseUrl: "https://azure.openai.com"},
			{Type: database.AIProviderTypeCopilot, Name: aibridge.ProviderCopilot, BaseUrl: "https://api.individual.githubcopilot.com"},
			{Type: database.AIProviderTypeCopilot, Name: agplaibridge.ProviderCopilotBusiness, BaseUrl: "https://" + agplaibridge.HostCopilotBusiness},
			{Type: database.AIProviderTypeCopilot, Name: agplaibridge.ProviderCopilotEnterprise, BaseUrl: "https://" + agplaibridge.HostCopilotEnterprise},
			{Type: database.AIProviderTypeOpenai, Name: agplaibridge.ProviderChatGPT, BaseUrl: agplaibridge.BaseURLChatGPT},
			{
				Type:     database.AIProviderTypeBedrock,
				Name:     "bedrock",
				BaseUrl:  "https://bedrock-runtime.us-east-1.amazonaws.com/",
				Settings: sql.NullString{String: string(bedrockSettings), Valid: true},
			},
		}
		for _, row := range rows {
			row.Enabled = true
			key := "sk-" + row.Name
			if row.Type == database.AIProviderTypeCopilot || row.Type == database.AIProviderTypeBedrock {
				key = ""
			}
			dbgen.AIProviderWithOptionalKey(t, db, row, key)
		}

		providers, outcomes, err := buildFromDB(ctx, t, db, slogtest.Make(t, nil))
		require.NoError(t, err)
		require.Len(t, providers, len(rows))
		require.Len(t, outcomes, len(rows))
		for _, outcome := range outcomes {
			require.NoError(t, outcome.Err)
		}
		byName := make(map[string]aibridge.Provider, len(providers))
		for _, provider := range providers {
			byName[provider.Name()] = provider
		}
		for _, row := range rows {
			require.Contains(t, byName, row.Name)
			require.Equal(t, row.BaseUrl, byName[row.Name].BaseURL())
			if row.Type == database.AIProviderTypeBedrock {
				require.Equal(t, aibridge.ProviderAnthropic, byName[row.Name].Type())
			} else {
				require.EqualValues(t, row.Type, byName[row.Name].Type())
			}
			if row.Type != database.AIProviderTypeCopilot && row.Type != database.AIProviderTypeBedrock {
				require.Len(t, byName[row.Name].KeyPool().PoolState(), 1)
			} else {
				require.Nil(t, byName[row.Name].KeyPool())
			}
		}
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
// returned snapshot without surfacing a top-level error.
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
		providers, outcomes, err := buildFromDB(ctx, t, db, logger)
		require.NoError(t, err)
		assert.Empty(t, providers)
		assert.Empty(t, outcomes)
	})

	t.Run("BedrockWithoutSettings", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		dbgen.AIProvider(t, db, database.AIProvider{
			Type:    database.AIProviderTypeBedrock,
			Name:    "bedrock-no-settings",
			Enabled: true,
			BaseUrl: "https://bedrock-runtime.us-east-1.amazonaws.com/",
		})

		providers, outcomes, err := buildFromDB(ctx, t, db, logger)
		require.NoError(t, err)
		require.Empty(t, providers)
		require.Len(t, outcomes, 1)
		require.Equal(t, aibridged.ProviderStatusError, outcomes[0].Status)
		require.ErrorContains(t, outcomes[0].Err, "bedrock provider has no bedrock credentials configured")
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

		providers, outcomes, err := buildFromDB(ctx, t, db, logger)
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

		providers, outcomes, err := buildFromDB(ctx, t, db, logger)
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

				providers, outcomes, err := buildFromDB(ctx, t, db, logger)
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
