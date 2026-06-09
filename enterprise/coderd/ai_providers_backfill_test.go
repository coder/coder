package coderd_test

import (
	"crypto/rand"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agplcoderd "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/testutil"
)

func TestBackfillBedrockProviderTypeEncryptedSettings(t *testing.T) {
	t.Parallel()

	rawDB, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	ciphers, err := dbcrypt.NewCiphers(key)
	require.NoError(t, err)
	cryptDB, err := dbcrypt.New(ctx, rawDB, ciphers...)
	require.NoError(t, err)

	bedrockSettings := sql.NullString{
		String: `{"_type":"bedrock","_version":1,"region":"us-east-1"}`,
		Valid:  true,
	}
	provider := dbgen.AIProvider(t, cryptDB, database.AIProvider{
		Type:     database.AiProviderTypeAnthropic,
		Settings: bedrockSettings,
	})

	agplcoderd.BackfillBedrockProviderType(ctx, cryptDB, logger)

	// Verify via raw DB: type is not encrypted so it is directly readable.
	row, err := rawDB.GetAIProviderByName(ctx, provider.Name)
	require.NoError(t, err)
	require.Equal(t, database.AiProviderTypeBedrock, row.Type, "encrypted legacy row must be promoted")
	require.True(t, row.SettingsKeyID.Valid, "settings must remain encrypted after backfill")
}
