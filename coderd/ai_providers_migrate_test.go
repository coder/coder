package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestSeedAIProvidersFromEnv(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfigNoOp", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()
		err := coderd.SeedAIProvidersFromEnv(ctx, db, codersdk.AIBridgeConfig{}, auditor, testLogger(t))
		require.NoError(t, err)
		require.Empty(t, auditor.AuditLogs())
	})

	t.Run("LegacyOpenAI", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-legacy"),
			},
		}
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t))
		require.NoError(t, err)

		// One row exists for "openai".
		row, err := db.GetAIProviderByName(ctx, "openai")
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeOpenai, row.Type)
		require.Equal(t, "https://api.openai.com/v1", row.BaseUrl)
		require.True(t, row.Enabled)

		// One ai_provider_keys row was created with the env key.
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, "sk-legacy", keys[0].APIKey)

		// Re-running with the same config is a no-op (no errors, no
		// new audit logs because the row matches).
		auditor.ResetLogs()
		err = coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t))
		require.NoError(t, err)
		require.Empty(t, auditor.AuditLogs())

		// Verify there's still only one row and one key.
		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, err)
		require.Len(t, all, 1)
		keys, err = db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Len(t, keys, 1)
	})

	t.Run("DriftFailsStartup", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-original"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		// Changing the API key alone does NOT count as drift: keys
		// live in a separate table and operators rotate them via the
		// API. Only changes to provider-level fields (base_url, type,
		// Bedrock settings) trip the drift check.
		cfg.LegacyOpenAI.Key = serpent.String("sk-rotated")
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		// Changing the base URL is real drift.
		cfg.LegacyOpenAI.BaseURL = serpent.String("https://api.openai.com/v2")
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "different fields")
	})

	t.Run("LegacyAnthropicWithBedrock", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			LegacyAnthropic: codersdk.AIBridgeAnthropicConfig{
				BaseURL: serpent.String("https://api.anthropic.com/"),
				Key:     serpent.String("sk-ant"),
			},
			LegacyBedrock: codersdk.AIBridgeBedrockConfig{
				Region:          serpent.String("us-west-2"),
				AccessKey:       serpent.String("AKIA"),
				AccessKeySecret: serpent.String("secret"),
				Model:           serpent.String("anthropic.claude-3-5-sonnet"),
				SmallFastModel:  serpent.String("anthropic.claude-3-5-haiku"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "anthropic")
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeAnthropic, row.Type)
		// Settings carry both the Bedrock access key and secret
		// alongside the model identifiers.
		require.Contains(t, row.Settings.String, "us-west-2")
		require.Contains(t, row.Settings.String, "anthropic.claude-3-5-sonnet")
		require.Contains(t, row.Settings.String, "anthropic.claude-3-5-haiku")
		require.Contains(t, row.Settings.String, "AKIA")
		require.Contains(t, row.Settings.String, "secret")
		// Anthropic + Bedrock together still gets the Anthropic
		// bearer key as a regular ai_provider_keys row.
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, "sk-ant", keys[0].APIKey)
	})

	t.Run("BedrockOnlyAnthropic", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			LegacyBedrock: codersdk.AIBridgeBedrockConfig{
				Region:          serpent.String("us-east-1"),
				AccessKey:       serpent.String("AKIAONLY"),
				AccessKeySecret: serpent.String("secretonly"),
				Model:           serpent.String("anthropic.claude-3-5-sonnet"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))
		row, err := db.GetAIProviderByName(ctx, "anthropic")
		require.NoError(t, err)
		require.Contains(t, row.Settings.String, "us-east-1")
		require.Contains(t, row.Settings.String, "AKIAONLY")
		require.Contains(t, row.Settings.String, "secretonly")
		// Bedrock-only Anthropic has zero ai_provider_keys: it
		// authenticates via the settings blob.
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Empty(t, keys)
	})

	t.Run("IndexedProviders", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "primary-openai",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-1", "sk-2"},
				},
				{
					Type:                  "anthropic",
					Name:                  "primary-anthropic",
					BaseURL:               "https://api.anthropic.com/",
					Keys:                  []string{"sk-ant-1"},
					BedrockRegion:         "us-east-1",
					BedrockModel:          "anthropic.claude-3-5-sonnet",
					BedrockSmallFastModel: "anthropic.claude-3-5-haiku",
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		oa, err := db.GetAIProviderByName(ctx, "primary-openai")
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeOpenai, oa.Type)
		oaKeys, err := db.GetAIProviderKeysByProviderID(ctx, oa.ID)
		require.NoError(t, err)
		require.Len(t, oaKeys, 2, "openai keys should be seeded in input order")
		require.Equal(t, "sk-1", oaKeys[0].APIKey)
		require.Equal(t, "sk-2", oaKeys[1].APIKey)

		an, err := db.GetAIProviderByName(ctx, "primary-anthropic")
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeAnthropic, an.Type)
		// Without AWS credentials, the bedrock_* env fields are not
		// stored: the row is a regular bearer-token Anthropic provider
		// and the discriminated settings would otherwise misrepresent
		// it as a Bedrock auth.
		require.False(t, an.Settings.Valid, "no settings blob without AWS credentials")
		anKeys, err := db.GetAIProviderKeysByProviderID(ctx, an.ID)
		require.NoError(t, err)
		require.Len(t, anKeys, 1)
		require.Equal(t, "sk-ant-1", anKeys[0].APIKey)
	})

	t.Run("BedrockIndexedProviderHasNoKeys", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:                    "anthropic",
					Name:                    "bedrock-anthropic",
					BaseURL:                 "https://bedrock-runtime.us-east-1.amazonaws.com/",
					BedrockRegion:           "us-east-1",
					BedrockModel:            "anthropic.claude-3-5-sonnet",
					BedrockAccessKeys:       []string{"AKIA-indexed"},
					BedrockAccessKeySecrets: []string{"indexed-secret"},
					// Keys would normally be a bearer api_key, but
					// Bedrock providers ignore those: credentials
					// live in the settings blob.
					Keys: []string{"sk-should-not-be-seeded"},
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "bedrock-anthropic")
		require.NoError(t, err)
		require.Contains(t, row.Settings.String, "AKIA-indexed")
		require.Contains(t, row.Settings.String, "indexed-secret")
		// Crucially, no ai_provider_keys rows for Bedrock providers.
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Empty(t, keys, "Bedrock providers must not seed bearer keys")
		// The plaintext Keys entry should not appear in settings.
		require.NotContains(t, row.Settings.String, "sk-should-not-be-seeded")
	})

	t.Run("LegacyAndIndexedSameNameConflict", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-legacy"),
			},
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "openai",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-indexed"},
				},
			},
		}
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "conflicts")
	})

	t.Run("InvalidProviderName", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "Bad_Name",
					BaseURL: "https://api.openai.com/v1",
				},
			},
		}
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t))
		require.Error(t, err)
	})

	t.Run("UnknownProviderTypeIsSkipped", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "copilot",
					Name:    "gh-copilot",
					BaseURL: "https://api.githubcopilot.com/",
				},
				{
					Type:    "openai",
					Name:    "real-openai",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk"},
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, err)
		require.Len(t, all, 1)
		require.Equal(t, "real-openai", all[0].Name)
	})

	t.Run("SoftDeletedRowIsNotResurrected", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-original"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "openai")
		require.NoError(t, err)
		require.NoError(t, db.DeleteAIProviderByID(ctx, row.ID))

		// Re-run seed; the soft-deleted row should remain soft-deleted
		// and no new row should be created.
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, err)
		require.Empty(t, all, "expected no active rows after soft-delete + re-seed")
	})

	t.Run("ExistingKeysArePreserved", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-original"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "openai")
		require.NoError(t, err)

		// Operator rotates the env key. The seed must not duplicate
		// keys on a row that already exists; the new key is only
		// installed via the API/CRUD layer in this flow.
		cfg.LegacyOpenAI.Key = serpent.String("sk-rotated")
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, auditor, testLogger(t)))

		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Len(t, keys, 1, "env reseed must not duplicate keys on existing rows")
		require.Equal(t, "sk-original", keys[0].APIKey)
	})
}

func testLogger(t *testing.T) slog.Logger {
	t.Helper()
	return slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
}
