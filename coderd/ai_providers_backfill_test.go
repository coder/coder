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
				Type:     database.AiProviderTypeAnthropic,
				Settings: bedrockSettings,
			})

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, legacy.Name)
			require.NoError(t, err)
			require.Equal(t, database.AiProviderTypeBedrock, row.Type)
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
				require.Equal(t, database.AiProviderTypeBedrock, r.Type,
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
				Type: database.AiProviderTypeAnthropic,
			})

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, native.Name)
			require.NoError(t, err)
			require.Equal(t, database.AiProviderTypeAnthropic, row.Type)
		})

		t.Run("PreservesNativeBedrockRow", func(t *testing.T) {
			native := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeBedrock,
				Settings: bedrockSettings,
			})

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, native.Name)
			require.NoError(t, err)
			require.Equal(t, database.AiProviderTypeBedrock, row.Type)
		})

		t.Run("SkipsDeletedRows", func(t *testing.T) {
			deleted := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeAnthropic,
				Settings: bedrockSettings,
			})
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
					require.Equal(t, database.AiProviderTypeAnthropic, r.Type, "deleted row must not be promoted")
				}
			}
			require.True(t, found, "deleted row must appear in IncludeDeleted result set")
		})

		t.Run("IncludesDisabledRows", func(t *testing.T) {
			disabled := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeAnthropic,
				Enabled:  false,
				Settings: bedrockSettings,
			})

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, disabled.Name)
			require.NoError(t, err)
			require.Equal(t, database.AiProviderTypeBedrock, row.Type, "disabled legacy row must be promoted")
		})

		t.Run("PreservesAnthropicRowWithNonBedrockSettings", func(t *testing.T) {
			// Valid JSON that parses without error but carries no Bedrock
			// discriminator. settings.Bedrock is nil so the row must not be
			// promoted, even though it would survive UnmarshalJSON cleanly.
			nonBedrock := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeAnthropic,
				Settings: sql.NullString{String: "{}", Valid: true},
			})

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			row, err := db.GetAIProviderByName(ctx, nonBedrock.Name)
			require.NoError(t, err)
			require.Equal(t, database.AiProviderTypeAnthropic, row.Type, "anthropic row with non-bedrock settings must not be promoted")
		})

		t.Run("SkipsUnparsableSettings", func(t *testing.T) {
			malformed := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeAnthropic,
				Settings: sql.NullString{String: "{", Valid: true},
			})
			good := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeAnthropic,
				Settings: bedrockSettings,
			})

			coderd.BackfillBedrockProviderType(ctx, db, logger)

			malformedRow, err := db.GetAIProviderByName(ctx, malformed.Name)
			require.NoError(t, err)
			require.Equal(t, database.AiProviderTypeAnthropic, malformedRow.Type, "row with unparsable settings must not be touched")

			goodRow, err := db.GetAIProviderByName(ctx, good.Name)
			require.NoError(t, err)
			require.Equal(t, database.AiProviderTypeBedrock, goodRow.Type, "valid row alongside unparsable one must still be promoted")
		})

		// --- chat_model_configs.provider backfill ---
		// These subtests rely on the DB already having type=bedrock providers
		// from the provider backfill subtests above.

		t.Run("FixesStaleModelConfigProvider", func(t *testing.T) {
			// Simulate a model config created when the linked provider was still
			// type=anthropic. The stored provider string is "anthropic" but the
			// linked provider row now has type=bedrock.
			bedrockProvider := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeBedrock,
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
		})

		t.Run("ModelConfigIdempotent", func(t *testing.T) {
			// DB already has the model config from the previous subtest with
			// provider="bedrock". A second run must be a no-op.
			coderd.BackfillChatModelConfigProviderStrings(ctx, db, logger)

			configs, err := db.GetChatModelConfigs(ctx)
			require.NoError(t, err)
			for _, c := range configs {
				require.NotEqual(t, "anthropic", c.Provider, "no model config must have provider=anthropic after second run")
			}
		})

		t.Run("PreservesNonAnthropicModelConfig", func(t *testing.T) {
			// A model config with provider="openai" linked to a Bedrock provider
			// must not be touched. Only "anthropic" → "bedrock" is in scope.
			bedrockProvider := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeBedrock,
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
				Type:     database.AiProviderTypeBedrock,
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
			// The SQL query guards on deleted = FALSE, so soft-deleted model
			// configs are not updated. There is no query that returns
			// soft-deleted configs, so we cannot directly read the provider
			// column on the deleted row. Instead we record the count of
			// visible configs before and after to confirm the backfill does
			// not resurrect the deleted row.
			bedrockProvider := dbgen.AIProvider(t, db, database.AIProvider{
				Type:     database.AiProviderTypeBedrock,
				Settings: bedrockSettings,
			})
			_ = dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Provider:     "anthropic",
				AIProviderID: uuid.NullUUID{UUID: bedrockProvider.ID, Valid: true},
			})

			before, err := db.GetChatModelConfigs(ctx)
			require.NoError(t, err)
			// Soft-delete the config after recording the count.
			require.NoError(t, db.DeleteChatModelConfigByID(ctx, before[len(before)-1].ID))

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
				Type:     database.AiProviderTypeAnthropic,
				Settings: bedrockSettings,
			}}, nil)
		db.EXPECT().
			UpdateAIProvider(gomock.Any(), gomock.Any()).
			Return(database.AIProvider{}, sql.ErrConnDone)

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
