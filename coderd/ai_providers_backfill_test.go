package coderd_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

// TestBackfillBedrockProviderType runs all DB-backed cases against a single
// database instance. Subtests are intentionally sequential so that each one
// builds on the state left by the previous, which proves idempotency without
// extra setup: a second backfill call on an already-promoted DB must be a
// no-op. Failure-path tests use a mock and stay parallel.
func TestBackfillBedrockProviderType(t *testing.T) {
	t.Parallel()

	bedrockSettings := sql.NullString{
		String: `{"_type":"bedrock","_version":1,"region":"us-east-1"}`,
		Valid:  true,
	}

	// All DB subtests share one database instance and run sequentially.
	t.Run("DB", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		logger := testLogger(t)

		t.Run("NoLegacyRows", func(t *testing.T) {
			coderd.BackfillBedrockProviderType(ctx, db, logger)

			all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
				IncludeDeleted:  true,
				IncludeDisabled: true,
			})
			require.NoError(t, err)
			require.Empty(t, all)
		})

		t.Run("PromotesLegacyRow", func(t *testing.T) {
			legacy := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeAnthropic,
				Settings: bedrockSettings,
			})
			require.Equal(t, database.AIProviderTypeAnthropic, legacy.Type, "pre-condition: row must start as anthropic")

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, legacy.Name)
			require.NoError(t, err)
			require.Equal(t, database.AIProviderTypeBedrock, row.Type)
		})

		t.Run("Idempotent", func(t *testing.T) {
			// DB already has one bedrock row from the previous subtest.
			// A second run must be a no-op: no type changes, no new rows.
			before, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
				IncludeDeleted:  true,
				IncludeDisabled: true,
			})
			require.NoError(t, err)
			for _, r := range before {
				require.Equal(t, database.AIProviderTypeBedrock, r.Type,
					"pre-condition: all rows must already be promoted before testing idempotency")
			}

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			after, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
				IncludeDeleted:  true,
				IncludeDisabled: true,
			})
			require.NoError(t, err)
			require.Equal(t, len(before), len(after), "second run must not create rows")
			for i := range after {
				require.Equal(t, before[i].Type, after[i].Type, "second run must not change types")
			}
		})

		t.Run("PreservesNativeAnthropicRow", func(t *testing.T) {
			native := dbgen.AIProvider(t, db, database.AIProvider{
				Type: database.AIProviderTypeAnthropic,
			})
			require.Equal(t, database.AIProviderTypeAnthropic, native.Type, "pre-condition")

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, native.Name)
			require.NoError(t, err)
			require.Equal(t, database.AIProviderTypeAnthropic, row.Type)
		})

		t.Run("PreservesNativeBedrockRow", func(t *testing.T) {
			native := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeBedrock,
				Settings: bedrockSettings,
			})
			require.Equal(t, database.AIProviderTypeBedrock, native.Type, "pre-condition")

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, native.Name)
			require.NoError(t, err)
			require.Equal(t, database.AIProviderTypeBedrock, row.Type)
		})

		t.Run("SkipsDeletedRows", func(t *testing.T) {
			deleted := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeAnthropic,
				Settings: bedrockSettings,
			})
			require.Equal(t, database.AIProviderTypeAnthropic, deleted.Type, "pre-condition")
			require.NoError(t, db.DeleteAIProviderByID(ctx, deleted.ID))

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
				IncludeDeleted:  true,
				IncludeDisabled: true,
			})
			require.NoError(t, err)
			var found bool
			for _, r := range row {
				if r.ID == deleted.ID {
					found = true
					require.Equal(t, database.AIProviderTypeAnthropic, r.Type, "deleted row must not be promoted")
				}
			}
			require.True(t, found, "deleted row must appear in IncludeDeleted result set")
		})

		t.Run("IncludesDisabledRows", func(t *testing.T) {
			disabled := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeAnthropic,
				Enabled:  false,
				Settings: bedrockSettings,
			})
			require.Equal(t, database.AIProviderTypeAnthropic, disabled.Type, "pre-condition")

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, disabled.Name)
			require.NoError(t, err)
			require.Equal(t, database.AIProviderTypeBedrock, row.Type, "disabled legacy row must be promoted")
		})

		t.Run("PreservesAnthropicRowWithNonBedrockSettings", func(t *testing.T) {
			// {} has no _type discriminator, so UnmarshalJSON returns an error
			// and the row is skipped via the unparsable-settings path, not the
			// settings.Bedrock == nil guard. Either way the row must stay anthropic.
			nonBedrock := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeAnthropic,
				Settings: sql.NullString{String: "{}", Valid: true},
			})
			require.Equal(t, database.AIProviderTypeAnthropic, nonBedrock.Type, "pre-condition")

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, nonBedrock.Name)
			require.NoError(t, err)
			require.Equal(t, database.AIProviderTypeAnthropic, row.Type, "anthropic row with non-bedrock settings must not be promoted")
		})

		t.Run("SkipsUnparsableSettings", func(t *testing.T) {
			malformed := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeAnthropic,
				Settings: sql.NullString{String: "{", Valid: true},
			})
			require.Equal(t, database.AIProviderTypeAnthropic, malformed.Type, "pre-condition")
			good := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeAnthropic,
				Settings: bedrockSettings,
			})
			require.Equal(t, database.AIProviderTypeAnthropic, good.Type, "pre-condition")

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			malformedRow, err := db.GetAIProviderByName(ctx, malformed.Name)
			require.NoError(t, err)
			require.Equal(t, database.AIProviderTypeAnthropic, malformedRow.Type, "row with unparsable settings must not be touched")

			goodRow, err := db.GetAIProviderByName(ctx, good.Name)
			require.NoError(t, err)
			require.Equal(t, database.AIProviderTypeBedrock, goodRow.Type, "valid row alongside unparsable one must still be promoted")
		})

		// --- chat_model_configs.provider backfill ---
		// These subtests rely on the DB already having type=bedrock providers
		// from the provider backfill subtests above.

		t.Run("FixesStaleModelConfigProvider", func(t *testing.T) {
			// Simulate a model config created when the linked provider was still
			// type=anthropic. The stored provider string is "anthropic" but the
			// linked provider row now has type=bedrock.
			bedrockProvider := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeBedrock,
				Settings: bedrockSettings,
			})
			staleConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Provider:     "anthropic",
				AIProviderID: uuid.NullUUID{UUID: bedrockProvider.ID, Valid: true},
			})

			coderd.BackfillChatModelConfigProviderStrings(ctx, db, logger)

			updated, err := db.GetChatModelConfigByID(ctx, staleConfig.ID)
			require.NoError(t, err)
			require.Equal(t, "bedrock", updated.Provider, "stale anthropic provider string must be fixed to bedrock")

			// Second run must be a no-op: the same config must still be "bedrock".
			coderd.BackfillChatModelConfigProviderStrings(ctx, db, logger)

			updated, err = db.GetChatModelConfigByID(ctx, staleConfig.ID)
			require.NoError(t, err)
			require.Equal(t, "bedrock", updated.Provider, "provider must remain bedrock after second run")
		})

		t.Run("ModelConfigIdempotent", func(t *testing.T) {
			before, err := db.GetChatModelConfigs(ctx)
			require.NoError(t, err)

			coderd.BackfillChatModelConfigProviderStrings(ctx, db, logger)

			after, err := db.GetChatModelConfigs(ctx)
			require.NoError(t, err)
			require.Equal(t, len(before), len(after), "second run must not create or delete rows")
		})

		t.Run("PreservesNonAnthropicModelConfig", func(t *testing.T) {
			// A model config with provider="openai" linked to a Bedrock provider
			// must not be touched. Only "anthropic" → "bedrock" is in scope.
			bedrockProvider := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeBedrock,
				Settings: bedrockSettings,
			})
			openAIConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Provider:     "openai",
				AIProviderID: uuid.NullUUID{UUID: bedrockProvider.ID, Valid: true},
			})

			coderd.BackfillChatModelConfigProviderStrings(ctx, db, logger)

			row, err := db.GetChatModelConfigByID(ctx, openAIConfig.ID)
			require.NoError(t, err)
			require.Equal(t, "openai", row.Provider, "non-anthropic provider string must not be changed")
		})

		t.Run("SkipsModelConfigWithDeletedProvider", func(t *testing.T) {
			// Verifies the EXISTS subquery excludes soft-deleted providers.
			// The model config provider string must stay "anthropic" because
			// the linked provider is deleted and therefore excluded by the
			// AND deleted = FALSE condition in the query.
			deletedProvider := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeBedrock,
				Settings: bedrockSettings,
			})
			staleConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Provider:     "anthropic",
				AIProviderID: uuid.NullUUID{UUID: deletedProvider.ID, Valid: true},
			})
			require.NoError(t, db.DeleteAIProviderByID(ctx, deletedProvider.ID))

			coderd.BackfillChatModelConfigProviderStrings(ctx, db, logger)

			row, err := db.GetChatModelConfigByID(ctx, staleConfig.ID)
			require.NoError(t, err)
			require.Equal(t, "anthropic", row.Provider, "config linked to deleted provider must not be updated")
		})

		t.Run("SkipsDeletedModelConfig", func(t *testing.T) {
			// The SQL query guards on deleted = FALSE. Capture the config ID
			// before deletion so we delete the right row regardless of ordering.
			bedrockProvider := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AIProviderTypeBedrock,
				Settings: bedrockSettings,
			})
			cfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Provider:     "anthropic",
				AIProviderID: uuid.NullUUID{UUID: bedrockProvider.ID, Valid: true},
			})

			before, err := db.GetChatModelConfigs(ctx)
			require.NoError(t, err)
			require.NoError(t, db.DeleteChatModelConfigByID(ctx, cfg.ID))

			coderd.BackfillChatModelConfigProviderStrings(ctx, db, logger)

			after, err := db.GetChatModelConfigs(ctx)
			require.NoError(t, err)
			require.Equal(t, len(before)-1, len(after), "deleted config must not reappear after backfill")
		})
	})

	t.Run("ListFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		db.EXPECT().
			GetAIProviders(gomock.Any(), gomock.Any()).
			Return(nil, sql.ErrConnDone)

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))
	})

	t.Run("UpdateFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		db.EXPECT().
			GetAIProviders(gomock.Any(), gomock.Any()).
			Return([]database.AIProvider{{
				Type:     database.AIProviderTypeAnthropic,
				Settings: bedrockSettings,
			}}, nil)
		db.EXPECT().
			UpdateAIProvider(gomock.Any(), gomock.Any()).
			Return(database.AIProvider{}, sql.ErrConnDone)

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))
	})

	t.Run("ProviderDeletedDuringBackfill", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		db.EXPECT().
			GetAIProviders(gomock.Any(), gomock.Any()).
			Return([]database.AIProvider{{
				Type:     database.AIProviderTypeAnthropic,
				Settings: bedrockSettings,
			}}, nil)
		db.EXPECT().
			UpdateAIProvider(gomock.Any(), gomock.Any()).
			Return(database.AIProvider{}, sql.ErrNoRows)

		// ErrNoRows is benign: provider was deleted between list and update.
		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))
	})

	t.Run("ModelConfigQueryFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		db.EXPECT().
			BackfillChatModelConfigProvider(gomock.Any(), gomock.Any()).
			Return(nil, sql.ErrConnDone)

		coderd.BackfillChatModelConfigProviderStrings(ctx, db, testLogger(t))
	})
}
