package coderd_test

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agplcoderd "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/testutil"
)

func TestBackfillBedrockProviderTypeEncryptedSettings(t *testing.T) {
	t.Parallel()

	rawDB, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	ciphers, err := dbcrypt.NewCiphers(key)
	require.NoError(t, err)
	cryptDB, err := dbcrypt.New(ctx, rawDB, ciphers...)
	require.NoError(t, err)

	rawSettings, err := json.Marshal(codersdk.AIProviderSettings{
		Bedrock: &codersdk.AIProviderBedrockSettings{Region: "us-east-1"},
	})
	require.NoError(t, err)
	provider := dbgen.AIProvider(t, cryptDB, database.AIProvider{
		Type:     database.AIProviderTypeAnthropic,
		Settings: sql.NullString{String: string(rawSettings), Valid: true},
	})

	agplcoderd.BackfillBedrockProviderType(ctx, cryptDB, logger)

	// Verify via raw DB: type is not encrypted so it is directly readable.
	row, err := rawDB.GetAIProviderByName(ctx, provider.Name)
	require.NoError(t, err)
	require.Equal(t, database.AIProviderTypeBedrock, row.Type, "encrypted legacy row must be promoted")
	require.True(t, row.SettingsKeyID.Valid, "settings must remain encrypted after backfill")
}
