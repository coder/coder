package coderd_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd"
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
		err := coderd.SeedAIProvidersFromEnv(ctx, db, codersdk.AIBridgeConfig{}, testLogger(t))
		require.NoError(t, err)
	})

	t.Run("LegacyOpenAI", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-legacy"),
			},
		}
		var firstSeedLogs bytes.Buffer
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, capturedLogger(&firstSeedLogs))
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

		// The seed emits one info line per inserted provider and one per
		// inserted key, replacing the audit entries that used to record
		// the same events.
		require.Contains(t, firstSeedLogs.String(), "env-seeded ai provider")
		require.Contains(t, firstSeedLogs.String(), "env-seeded ai provider key")

		// Re-running with the same config is a no-op and emits no new
		// env-seed log lines.
		var rerunLogs bytes.Buffer
		err = coderd.SeedAIProvidersFromEnv(ctx, db, cfg, capturedLogger(&rerunLogs))
		require.NoError(t, err)
		require.NotContains(t, rerunLogs.String(), "env-seeded ai provider")

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

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-original"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		// Changing the API key alone does NOT count as drift: keys
		// live in a separate table and operators rotate them via the
		// API. Only changes to non-credential provider-level fields
		// (base_url, type, Bedrock region/model) trip the drift check.
		cfg.LegacyOpenAI.Key = serpent.String("sk-rotated")
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		// Changing the base URL is real drift.
		cfg.LegacyOpenAI.BaseURL = serpent.String("https://api.openai.com/v2")
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")
	})

	t.Run("BedrockCredentialRotationIsNotDrift", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			LegacyBedrock: codersdk.AIBridgeBedrockConfig{
				Region:          serpent.String("us-east-1"),
				AccessKey:       serpent.String("AKIA-original"),
				AccessKeySecret: serpent.String("secret-original"),
				Model:           serpent.String("anthropic.claude-3-5-sonnet"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		// Rotating the Bedrock access key and secret in env must NOT
		// trip the drift check: they're credentials, equivalent to
		// bearer API keys, and operators rotate them via the API.
		cfg.LegacyBedrock.AccessKey = serpent.String("AKIA-rotated")
		cfg.LegacyBedrock.AccessKeySecret = serpent.String("secret-rotated")
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		// Changing the Bedrock region (a non-credential field) is
		// real drift.
		cfg.LegacyBedrock.Region = serpent.String("us-west-2")
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")
	})

	t.Run("LegacyBedrockOnlyKeepsBedrockSettings", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Bedrock fields without an Anthropic key produce a Bedrock-
		// authenticated Anthropic provider with no bearer keys.
		cfg := codersdk.AIBridgeConfig{
			LegacyBedrock: codersdk.AIBridgeBedrockConfig{
				Region:          serpent.String("us-west-2"),
				AccessKey:       serpent.String("AKIA"),
				AccessKeySecret: serpent.String("secret"),
				Model:           serpent.String("anthropic.claude-3-5-sonnet"),
				SmallFastModel:  serpent.String("anthropic.claude-3-5-haiku"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "anthropic")
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeAnthropic, row.Type)
		require.Contains(t, row.Settings.String, "us-west-2")
		require.Contains(t, row.Settings.String, "anthropic.claude-3-5-sonnet")
		require.Contains(t, row.Settings.String, "anthropic.claude-3-5-haiku")
		require.Contains(t, row.Settings.String, "AKIA")
		require.Contains(t, row.Settings.String, "secret")
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Empty(t, keys, "Bedrock provider must not seed bearer keys")
	})

	t.Run("LegacyAnthropicKeyOnlyIgnoresBedrockModelDefaults", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// LegacyBedrock.Model and LegacyBedrock.SmallFastModel both
		// have serpent-level defaults that are always populated in a
		// real deployment. Apply those defaults here so the test
		// reflects deployment state rather than a hand-crafted config,
		// then set only the Anthropic key. The result must be a pure
		// bearer-token Anthropic row with no Bedrock settings blob.
		dv := codersdk.DeploymentValues{}
		opts := dv.Options()
		require.NoError(t, opts.SetDefaults())
		// Sanity check: the defaults we rely on are present.
		require.NotEmpty(t, dv.AI.BridgeConfig.LegacyBedrock.Model.String())
		require.NotEmpty(t, dv.AI.BridgeConfig.LegacyBedrock.SmallFastModel.String())

		cfg := dv.AI.BridgeConfig
		cfg.LegacyAnthropic.Key = serpent.String("sk-ant-only")
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "anthropic")
		require.NoError(t, err)
		require.False(t, row.Settings.Valid, "model defaults alone must not produce a Bedrock settings blob")
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, "sk-ant-only", keys[0].APIKey)
	})

	t.Run("BedrockWithoutCredentialsUsesAWSEnvAuth", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Any non-empty Bedrock field signals Bedrock auth. AWS
		// credentials are optional because Bedrock can authenticate
		// via the AWS environment (instance profile, AWS_PROFILE, etc.).
		cfg := codersdk.AIBridgeConfig{
			LegacyBedrock: codersdk.AIBridgeBedrockConfig{
				Region: serpent.String("us-east-1"),
				Model:  serpent.String("anthropic.claude-3-5-sonnet"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "anthropic")
		require.NoError(t, err)
		require.True(t, row.Settings.Valid, "Bedrock metadata must produce a settings blob")
		require.Contains(t, row.Settings.String, "us-east-1")
		require.Contains(t, row.Settings.String, "anthropic.claude-3-5-sonnet")
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Empty(t, keys, "Bedrock provider must not seed bearer keys")
	})

	t.Run("BedrockOnlyAnthropic", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			LegacyBedrock: codersdk.AIBridgeBedrockConfig{
				Region:          serpent.String("us-east-1"),
				AccessKey:       serpent.String("AKIAONLY"),
				AccessKeySecret: serpent.String("secretonly"),
				Model:           serpent.String("anthropic.claude-3-5-sonnet"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))
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

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "primary-openai",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-1", "sk-2"},
				},
				{
					Type:    "anthropic",
					Name:    "primary-anthropic",
					BaseURL: "https://api.anthropic.com/",
					Keys:    []string{"sk-ant-1"},
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		oa, err := db.GetAIProviderByName(ctx, "primary-openai")
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeOpenai, oa.Type)
		oaKeys, err := db.GetAIProviderKeysByProviderID(ctx, oa.ID)
		require.NoError(t, err)
		require.Len(t, oaKeys, 2)
		gotKeys := []string{oaKeys[0].APIKey, oaKeys[1].APIKey}
		require.ElementsMatch(t, []string{"sk-1", "sk-2"}, gotKeys)

		an, err := db.GetAIProviderByName(ctx, "primary-anthropic")
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeAnthropic, an.Type)
		// Plain bearer-token Anthropic with no Bedrock fields: no
		// settings blob, one bearer key.
		require.False(t, an.Settings.Valid, "no settings blob for bearer-token Anthropic")
		anKeys, err := db.GetAIProviderKeysByProviderID(ctx, an.ID)
		require.NoError(t, err)
		require.Len(t, anKeys, 1)
		require.Equal(t, "sk-ant-1", anKeys[0].APIKey)
	})

	t.Run("BedrockIndexedProviderHasNoKeys", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

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
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "bedrock-anthropic")
		require.NoError(t, err)
		require.Contains(t, row.Settings.String, "AKIA-indexed")
		require.Contains(t, row.Settings.String, "indexed-secret")
		// Crucially, no ai_provider_keys rows for Bedrock providers.
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Empty(t, keys, "Bedrock providers must not seed bearer keys")
	})

	t.Run("LegacyAndIndexedSameNameConflict", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

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
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "conflicts")
	})

	t.Run("InvalidProviderName", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "Bad_Name",
					BaseURL: "https://api.openai.com/v1",
				},
			},
		}
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid AI provider name")
	})

	t.Run("UnknownProviderTypeIsSkipped", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// A TYPE that isn't part of the ai_provider_type enum falls
		// into the default branch and the row is skipped rather than
		// rejected, so deployments don't fail to start over a single
		// typo'd provider.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "not-a-real-provider",
					Name:    "ghost",
					BaseURL: "https://example.com",
				},
				{
					Type:    "openai",
					Name:    "real-openai",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk"},
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, err)
		require.Len(t, all, 1)
		require.Equal(t, "real-openai", all[0].Name)
	})

	t.Run("SoftDeletedRowIsNotResurrected", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-original"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "openai")
		require.NoError(t, err)
		require.NoError(t, db.DeleteAIProviderByID(ctx, database.DeleteAIProviderByIDParams{
			ID: row.ID,
		}))

		// Re-run seed; the soft-deleted row should remain soft-deleted
		// and no new row should be created.
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, err)
		require.Empty(t, all, "expected no active rows after soft-delete + re-seed")
	})

	t.Run("ExistingKeysArePreserved", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			LegacyOpenAI: codersdk.AIBridgeOpenAIConfig{
				BaseURL: serpent.String("https://api.openai.com/v1"),
				Key:     serpent.String("sk-original"),
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "openai")
		require.NoError(t, err)

		// Operator rotates the env key. The seed must not duplicate
		// keys on a row that already exists; the new key is only
		// installed via the API/CRUD layer in this flow.
		cfg.LegacyOpenAI.Key = serpent.String("sk-rotated")
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Len(t, keys, 1, "env reseed must not duplicate keys on existing rows")
		require.Equal(t, "sk-original", keys[0].APIKey)
	})

	t.Run("IndexedDuplicateNameMatchingHashDedupes", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Two entries under the same name with identical canonical
		// fields are deduplicated silently.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "shared",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-1"},
				},
				{
					Type:    "openai",
					Name:    "shared",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-1"},
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, err)
		require.Len(t, all, 1, "duplicate indexed entries with matching hash must produce a single row")
	})

	t.Run("IndexedDuplicateNameMismatchingHashFails", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Same name, different canonical fields: must be rejected.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "shared",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-1"},
				},
				{
					Type:    "openai",
					Name:    "shared",
					BaseURL: "https://api.openai.com/v2",
					Keys:    []string{"sk-2"},
				},
			},
		}
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "conflicting fields")
	})
}

func testLogger(t *testing.T) slog.Logger {
	t.Helper()
	return slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
}

// capturedLogger returns a logger that writes structured records to buf,
// for tests that assert on log output instead of audit-table emissions.
func capturedLogger(buf *bytes.Buffer) slog.Logger {
	return slog.Make(sloghuman.Sink(buf)).Leveled(slog.LevelDebug)
}
