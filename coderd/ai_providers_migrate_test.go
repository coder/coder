package coderd_test

import (
	"bytes"
	"database/sql"
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
		require.Equal(t, database.AIProviderTypeOpenai, row.Type)
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

		// Changing the API key counts as drift: keys are included
		// in the canonical hash so operators notice when env-var
		// credential changes are ignored by an existing provider.
		cfg.LegacyOpenAI.Key = serpent.String("sk-rotated")
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")

		// Changing the base URL is also real drift.
		cfg.LegacyOpenAI.Key = serpent.String("sk-original")
		cfg.LegacyOpenAI.BaseURL = serpent.String("https://api.openai.com/v2")
		err = coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")
	})

	t.Run("BedrockCredentialChangeIsDrift", func(t *testing.T) {
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

		// Rotating the Bedrock access key in env trips the drift
		// check so operators know the change did not take effect.
		cfg.LegacyBedrock.AccessKey = serpent.String("AKIA-rotated")
		cfg.LegacyBedrock.AccessKeySecret = serpent.String("secret-rotated")
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")

		// Changing the Bedrock region (a non-credential field) is
		// also real drift.
		cfg.LegacyBedrock.AccessKey = serpent.String("AKIA-original")
		cfg.LegacyBedrock.AccessKeySecret = serpent.String("secret-original")
		cfg.LegacyBedrock.Region = serpent.String("us-west-2")
		err = coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")
	})

	t.Run("LegacyBedrockOnlyKeepsBedrockSettings", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Bedrock fields without an Anthropic key produce a type=bedrock
		// provider named "anthropic" with no bearer keys.
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
		require.Equal(t, database.AIProviderTypeBedrock, row.Type)
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
		require.Equal(t, database.AIProviderTypeOpenai, oa.Type)
		oaKeys, err := db.GetAIProviderKeysByProviderID(ctx, oa.ID)
		require.NoError(t, err)
		require.Len(t, oaKeys, 2)
		gotKeys := []string{oaKeys[0].APIKey, oaKeys[1].APIKey}
		require.ElementsMatch(t, []string{"sk-1", "sk-2"}, gotKeys)

		an, err := db.GetAIProviderByName(ctx, "primary-anthropic")
		require.NoError(t, err)
		require.Equal(t, database.AIProviderTypeAnthropic, an.Type)
		// Plain bearer-token Anthropic with no Bedrock fields: no
		// settings blob, one bearer key.
		require.False(t, an.Settings.Valid, "no settings blob for bearer-token Anthropic")
		anKeys, err := db.GetAIProviderKeysByProviderID(ctx, an.ID)
		require.NoError(t, err)
		require.Len(t, anKeys, 1)
		require.Equal(t, "sk-ant-1", anKeys[0].APIKey)
	})

	t.Run("IndexedProvidersKeyDriftWithMultipleKeysAndProviders", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "primary-openai",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-openai-1", "sk-openai-2"},
				},
				{
					Type:    "anthropic",
					Name:    "primary-anthropic",
					BaseURL: "https://api.anthropic.com/",
					Keys:    []string{"sk-ant-1", "sk-ant-2"},
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		// Reordering keys must not count as drift. The canonical hash
		// sorts keys before hashing, so equivalent key sets remain
		// stable across restarts.
		cfg.Providers[0].Keys = []string{"sk-openai-2", "sk-openai-1"}
		cfg.Providers[1].Keys = []string{"sk-ant-2", "sk-ant-1"}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		// Changing one key on one provider must block startup even
		// when multiple providers are configured.
		cfg.Providers[1].Keys = []string{"sk-ant-2", "sk-ant-rotated"}
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")
		require.Contains(t, err.Error(), `"primary-anthropic"`)

		oa, err := db.GetAIProviderByName(ctx, "primary-openai")
		require.NoError(t, err)
		oaKeys, err := db.GetAIProviderKeysByProviderID(ctx, oa.ID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"sk-openai-1", "sk-openai-2"}, []string{oaKeys[0].APIKey, oaKeys[1].APIKey})

		an, err := db.GetAIProviderByName(ctx, "primary-anthropic")
		require.NoError(t, err)
		anKeys, err := db.GetAIProviderKeysByProviderID(ctx, an.ID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"sk-ant-1", "sk-ant-2"}, []string{anKeys[0].APIKey, anKeys[1].APIKey})
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

	t.Run("IndexedClaudePlatformProvider", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:                          string(database.AIProviderTypeClaudePlatformAws),
					Name:                          "claude-platform",
					BaseURL:                       "https://aws-external-anthropic.us-east-1.api.aws",
					ClaudePlatformRegion:          "us-east-1",
					ClaudePlatformWorkspaceID:     "wrkspc-123",
					ClaudePlatformAccessKey:       "AKIA-indexed",
					ClaudePlatformAccessKeySecret: "indexed-secret",
					ClaudePlatformAPIKey:          "sk-workspace",
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		row, err := db.GetAIProviderByName(ctx, "claude-platform")
		require.NoError(t, err)
		require.Equal(t, database.AIProviderTypeClaudePlatformAws, row.Type)
		require.Equal(t, "https://aws-external-anthropic.us-east-1.api.aws", row.BaseUrl)
		require.Contains(t, row.Settings.String, "us-east-1")
		require.Contains(t, row.Settings.String, "wrkspc-123")
		require.Contains(t, row.Settings.String, "AKIA-indexed")
		require.Contains(t, row.Settings.String, "indexed-secret")
		require.Contains(t, row.Settings.String, "sk-workspace")
		// Claude Platform providers authenticate via settings, never
		// via bearer keys in ai_provider_keys.
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Empty(t, keys, "Claude Platform providers must not seed bearer keys")

		// Re-running with the same config is a no-op (no drift).
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))
	})

	t.Run("ClaudePlatformCredentialChangeIsDrift", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:                          string(database.AIProviderTypeClaudePlatformAws),
					Name:                          "claude-platform",
					BaseURL:                       "https://aws-external-anthropic.us-east-1.api.aws",
					ClaudePlatformRegion:          "us-east-1",
					ClaudePlatformWorkspaceID:     "wrkspc-123",
					ClaudePlatformAccessKey:       "AKIA-original",
					ClaudePlatformAccessKeySecret: "secret-original",
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		// Rotating the access key in env trips the drift check so
		// operators know the change did not take effect.
		cfg.Providers[0].ClaudePlatformAccessKey = "AKIA-rotated"
		cfg.Providers[0].ClaudePlatformAccessKeySecret = "secret-rotated"
		err := coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")

		// Changing the workspace ID is also real drift.
		cfg.Providers[0].ClaudePlatformAccessKey = "AKIA-original"
		cfg.Providers[0].ClaudePlatformAccessKeySecret = "secret-original"
		cfg.Providers[0].ClaudePlatformWorkspaceID = "wrkspc-changed"
		err = coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")
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
		require.NoError(t, db.DeleteAIProviderByID(ctx, row.ID))

		// Re-run seed; the soft-deleted row should remain soft-deleted
		// and no new row should be created.
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, err)
		require.Empty(t, all, "expected no active rows after soft-delete + re-seed")
	})

	t.Run("ExistingKeysBlockOnDrift", func(t *testing.T) {
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

		// Operator rotates the env key. The seed now blocks startup
		// because the keys differ, alerting the operator.
		cfg.LegacyOpenAI.Key = serpent.String("sk-rotated")
		err = coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t))
		require.Error(t, err)
		require.Contains(t, err.Error(), "differs from the current environment configuration")

		// The original key is still in the database.
		keys, err := db.GetAIProviderKeysByProviderID(ctx, row.ID)
		require.NoError(t, err)
		require.Len(t, keys, 1)
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

	t.Run("IndexedDuplicateNameMatchingHashDedupesReorderedKeys", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// Key order should not affect the canonical hash. Reordered
		// duplicates under the same name should still dedupe.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    "openai",
					Name:    "shared",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-1", "sk-2"},
				},
				{
					Type:    "openai",
					Name:    "shared",
					BaseURL: "https://api.openai.com/v1",
					Keys:    []string{"sk-2", "sk-1"},
				},
			},
		}
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, err)
		require.Len(t, all, 1)
		keys, err := db.GetAIProviderKeysByProviderID(ctx, all[0].ID)
		require.NoError(t, err)
		require.Len(t, keys, 2)
		require.ElementsMatch(t, []string{"sk-1", "sk-2"}, []string{keys[0].APIKey, keys[1].APIKey})
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

	t.Run("SeedIsIdempotentAfterBedrockBackfill", func(t *testing.T) {
		t.Parallel()
		// Regression: seed must not treat a type=anthropic row promoted to
		// type=bedrock by the backfill as drift.
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cfg := codersdk.AIBridgeConfig{
			LegacyBedrock: codersdk.AIBridgeBedrockConfig{
				Region:          serpent.String("us-east-1"),
				AccessKey:       serpent.String("AKIA"),
				AccessKeySecret: serpent.String("secret"),
				Model:           serpent.String("anthropic.claude-3-5-sonnet"),
			},
		}

		// Seed to get a row with correct settings, then set type=anthropic to
		// simulate the pre-upgrade state where the old seed stored that type.
		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))
		row, err := db.GetAIProviderByName(ctx, "anthropic")
		require.NoError(t, err)
		_, err = db.UpdateAIProvider(ctx, database.UpdateAIProviderParams{
			ID:            row.ID,
			Type:          database.AIProviderTypeAnthropic,
			DisplayName:   row.DisplayName,
			Enabled:       row.Enabled,
			BaseUrl:       row.BaseUrl,
			Settings:      row.Settings,
			SettingsKeyID: sql.NullString{},
		})
		require.NoError(t, err)
		row, err = db.GetAIProviderByName(ctx, "anthropic")
		require.NoError(t, err)
		require.Equal(t, database.AIProviderTypeAnthropic, row.Type, "pre-condition: row must be anthropic before seed runs")

		require.NoError(t, coderd.SeedAIProvidersFromEnv(ctx, db, cfg, testLogger(t)))
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
