package provisionerdserver

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestObtainOIDCAccessToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("NoToken", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		_, err := obtainOIDCAccessToken(ctx, db, nil, uuid.Nil)
		require.NoError(t, err)
	})
	t.Run("InvalidConfig", func(t *testing.T) {
		// We still want OIDC to succeed even if exchanging the token fails.
		t.Parallel()
		db := dbfake.New()
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:      user.ID,
			LoginType:   database.LoginTypeOIDC,
			OAuthExpiry: dbtime.Now().Add(-time.Hour),
		})
		_, err := obtainOIDCAccessToken(ctx, db, &oauth2.Config{}, user.ID)
		require.NoError(t, err)
	})
	t.Run("Exchange", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:      user.ID,
			LoginType:   database.LoginTypeOIDC,
			OAuthExpiry: dbtime.Now().Add(-time.Hour),
		})
		_, err := obtainOIDCAccessToken(ctx, db, &testutil.OAuth2Config{
			Token: &oauth2.Token{
				AccessToken: "token",
			},
		}, user.ID)
		require.NoError(t, err)
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "token", link.OAuthAccessToken)
	})
}
