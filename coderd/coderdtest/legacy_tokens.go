package coderdtest

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
)

// LegacyToken inserts an API key fixture whose database row mimics the
// pre-scopes migration shape where scopes/allow_list columns were empty. The
// returned API key behaves like a legacy token that should expand to coder:all.
func LegacyToken(t testing.TB, db database.Store, userID uuid.UUID) (database.APIKey, string) {
	t.Helper()

	tokenName := fmt.Sprintf("legacy-token-%s", uuid.NewString()[:8])

	key, secret := dbgen.APIKey(t, db, database.APIKey{
		UserID:    userID,
		TokenName: tokenName,
		LoginType: database.LoginTypeToken,
	}, func(params *database.InsertAPIKeyParams) {
		params.Scopes = database.APIKeyScopes{}
		params.AllowList = database.AllowList{}
	})

	return key, secret
}
