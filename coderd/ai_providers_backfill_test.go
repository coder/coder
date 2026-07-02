package coderd_test

import (
	"database/sql"
	"testing"

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
}
