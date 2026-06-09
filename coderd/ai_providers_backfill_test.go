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

func TestBackfillBedrockProviderType(t *testing.T) {
	t.Parallel()

	bedrockSettings := sql.NullString{
		String: `{"_type":"bedrock","_version":1,"region":"us-east-1"}`,
		Valid:  true,
	}

	t.Run("NoLegacyRows", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
			IncludeDeleted:  true,
			IncludeDisabled: true,
		})
		require.NoError(t, err)
		require.Empty(t, all)
	})

	t.Run("PromotesLegacyAnthropicBedrockRow", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider := dbgen.AIProvider(t, db, database.AIProvider{
			Type:     database.AiProviderTypeAnthropic,
			Settings: bedrockSettings,
		})

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))

		row, err := db.GetAIProviderByName(ctx, provider.Name)
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeBedrock, row.Type)
	})

	t.Run("Idempotent", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider := dbgen.AIProvider(t, db, database.AIProvider{
			Type:     database.AiProviderTypeAnthropic,
			Settings: bedrockSettings,
		})

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))
		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))

		row, err := db.GetAIProviderByName(ctx, provider.Name)
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeBedrock, row.Type)

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
			IncludeDeleted:  false,
			IncludeDisabled: true,
		})
		require.NoError(t, err)
		require.Len(t, all, 1, "second run must not create duplicate rows")
	})

	t.Run("PreservesNativeAnthropicRow", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider := dbgen.AIProvider(t, db, database.AIProvider{
			Type: database.AiProviderTypeAnthropic,
		})

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))

		row, err := db.GetAIProviderByName(ctx, provider.Name)
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeAnthropic, row.Type)
	})

	t.Run("PreservesNativeBedrockRow", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider := dbgen.AIProvider(t, db, database.AIProvider{
			Type:     database.AiProviderTypeBedrock,
			Settings: bedrockSettings,
		})

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))

		row, err := db.GetAIProviderByName(ctx, provider.Name)
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeBedrock, row.Type)
	})

	t.Run("SkipsDeletedRows", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider := dbgen.AIProvider(t, db, database.AIProvider{
			Type:     database.AiProviderTypeAnthropic,
			Settings: bedrockSettings,
		})
		require.NoError(t, db.DeleteAIProviderByID(ctx, provider.ID))

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))

		all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{
			IncludeDeleted:  true,
			IncludeDisabled: true,
		})
		require.NoError(t, err)
		require.Len(t, all, 1)
		require.Equal(t, database.AiProviderTypeAnthropic, all[0].Type, "deleted row must not be promoted")
	})

	t.Run("IncludesDisabledRows", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		provider := dbgen.AIProvider(t, db, database.AIProvider{
			Type:     database.AiProviderTypeAnthropic,
			Enabled:  false,
			Settings: bedrockSettings,
		})

		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))

		row, err := db.GetAIProviderByName(ctx, provider.Name)
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeBedrock, row.Type, "disabled legacy row must be promoted")
	})

	t.Run("SkipsUnparseableSettings", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		malformed := dbgen.AIProvider(t, db, database.AIProvider{
			Type:     database.AiProviderTypeAnthropic,
			Settings: sql.NullString{String: "{", Valid: true},
		})
		good := dbgen.AIProvider(t, db, database.AIProvider{
			Type:     database.AiProviderTypeAnthropic,
			Settings: bedrockSettings,
		})

		// Must not panic; must continue to the good row despite the bad one.
		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))

		malformedRow, err := db.GetAIProviderByName(ctx, malformed.Name)
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeAnthropic, malformedRow.Type, "row with unparsable settings must not be touched")

		goodRow, err := db.GetAIProviderByName(ctx, good.Name)
		require.NoError(t, err)
		require.Equal(t, database.AiProviderTypeBedrock, goodRow.Type, "valid row must still be promoted")
	})

	t.Run("ListFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		db.EXPECT().
			GetAIProviders(gomock.Any(), gomock.Any()).
			Return(nil, sql.ErrConnDone)

		// Must not panic.
		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))
	})

	t.Run("UpdateFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		legacyProvider := database.AIProvider{
			Type:     database.AiProviderTypeAnthropic,
			Settings: bedrockSettings,
		}
		db.EXPECT().
			GetAIProviders(gomock.Any(), gomock.Any()).
			Return([]database.AIProvider{legacyProvider}, nil)
		db.EXPECT().
			UpdateAIProvider(gomock.Any(), gomock.Any()).
			Return(database.AIProvider{}, sql.ErrConnDone)

		// Must not panic; error must be logged but not propagated.
		coderd.BackfillBedrockProviderType(ctx, db, testLogger(t))
	})
}
