package provisionerdserver

import (
	"context"
	"testing"
	"time"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
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
			OAuthExpiry: database.Now().Add(-time.Hour),
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
			OAuthExpiry: database.Now().Add(-time.Hour),
		})
		_, err := obtainOIDCAccessToken(ctx, db, &oauth2Config{
			tokenSource: func() (*oauth2.Token, error) {
				return &oauth2.Token{
					AccessToken: "token",
				}, nil
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

type oauth2Config struct {
	tokenSource oauth2TokenSource
}

func (o *oauth2Config) TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource {
	return o.tokenSource
}

func (*oauth2Config) AuthCodeURL(string, ...oauth2.AuthCodeOption) string {
	return ""
}

func (*oauth2Config) Exchange(context.Context, string, ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{}, nil
}

type oauth2TokenSource func() (*oauth2.Token, error)

func (o oauth2TokenSource) Token() (*oauth2.Token, error) {
	return o()
}
