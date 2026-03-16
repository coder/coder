package provisionerdserver

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestShouldRefreshOIDCToken(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()
	testCases := []struct {
		name string
		link database.UserLink
		want bool
	}{
		{
			name: "NoRefreshToken",
			link: database.UserLink{OAuthExpiry: now.Add(-time.Hour)},
			want: false,
		},
		{
			name: "ZeroExpiry",
			link: database.UserLink{OAuthRefreshToken: "refresh"},
			want: false,
		},
		{
			name: "LongExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(-1 * time.Hour),
			},
			want: true,
		},
		{
			// Edge being "+/- 10 minutes"
			name: "EdgeExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(-1 * time.Minute * 10),
			},
			want: true,
		},
		{
			name: "Expired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(-1 * time.Minute),
			},
			want: true,
		},
		{
			name: "SoonToBeExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(5 * time.Minute),
			},
			want: true,
		},
		{
			name: "SoonToBeExpiredEdge",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(9 * time.Minute),
			},
			want: true,
		},
		{
			name: "AfterEdge",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(11 * time.Minute),
			},
			want: false,
		},
		{
			name: "NotExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(time.Hour),
			},
			want: false,
		},
		{
			name: "NotEvenCloseExpired",
			link: database.UserLink{
				OAuthRefreshToken: "refresh",
				OAuthExpiry:       now.Add(time.Hour * 24),
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			shouldRefresh, _ := shouldRefreshOIDCToken(tc.link)
			require.Equal(t, tc.want, shouldRefresh)
		})
	}
}

func TestObtainOIDCAccessToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("NoToken", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		_, err := ObtainOIDCAccessToken(ctx, testutil.Logger(t), db, nil, uuid.Nil)
		require.NoError(t, err)
	})
	t.Run("InvalidConfig", func(t *testing.T) {
		// We still want OIDC to succeed even if exchanging the token fails.
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:      user.ID,
			LoginType:   database.LoginTypeOIDC,
			OAuthExpiry: dbtime.Now().Add(-time.Hour),
		})
		_, err := ObtainOIDCAccessToken(ctx, testutil.Logger(t), db, &oauth2.Config{}, user.ID)
		require.NoError(t, err)
	})
	t.Run("MissingLink", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{
			LoginType: database.LoginTypeOIDC,
		})
		tok, err := ObtainOIDCAccessToken(ctx, testutil.Logger(t), db, &oauth2.Config{}, user.ID)
		require.Empty(t, tok)
		require.NoError(t, err)
	})
	t.Run("Exchange", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:      user.ID,
			LoginType:   database.LoginTypeOIDC,
			OAuthExpiry: dbtime.Now().Add(-time.Hour),
		})
		_, err := ObtainOIDCAccessToken(ctx, testutil.Logger(t), db, &testutil.OAuth2Config{
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
