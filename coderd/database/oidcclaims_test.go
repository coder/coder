package database_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

type extraKeys struct {
	database.UserLinkClaims
	Foo string `json:"foo"`
}

func TestOIDCClaims(t *testing.T) {
	t.Parallel()

	toJSON := func(a any) json.RawMessage {
		b, _ := json.Marshal(a)
		return b
	}

	db, _ := dbtestutil.NewDB(t)
	g := userGenerator{t: t, db: db}

	// https://en.wikipedia.org/wiki/Alice_and_Bob#Cast_of_characters
	alice := g.withLink(database.LoginTypeOIDC, toJSON(extraKeys{
		UserLinkClaims: database.UserLinkClaims{
			IDTokenClaims: map[string]interface{}{
				"sub": "alice",
			},
			UserInfoClaims: map[string]interface{}{
				"sub": "alice",
			},
		},
		// Always should be a no-op
		Foo: "bar",
	}))
	bob := g.withLink(database.LoginTypeOIDC, toJSON(database.UserLinkClaims{
		IDTokenClaims: map[string]interface{}{
			"sub": "bob",
		},
		UserInfoClaims: map[string]interface{}{
			"sub": "bob",
		},
	}))
	charlie := g.withLink(database.LoginTypeOIDC, toJSON(database.UserLinkClaims{
		IDTokenClaims: map[string]interface{}{
			"sub": "charlie",
		},
		UserInfoClaims: map[string]interface{}{
			"sub": "charlie",
		},
	}))

	// users that just try to cause problems, but should not affect the output of
	// queries.
	problematics := []database.User{
		g.withLink(database.LoginTypeOIDC, toJSON(database.UserLinkClaims{})), // null claims
		g.withLink(database.LoginTypeOIDC, []byte(`{}`)),                      // empty claims
		g.withLink(database.LoginTypeOIDC, []byte(`{"foo": "bar"}`)),          // random keys
		g.noLink(database.LoginTypeOIDC),                                      // no link

		g.withLink(database.LoginTypeGithub, toJSON(database.UserLinkClaims{
			IDTokenClaims: map[string]interface{}{
				"not": "allowed",
			},
			UserInfoClaims: map[string]interface{}{
				"do-not": "look",
			},
		})), // github should be omitted

		// extra random users
		g.noLink(database.LoginTypeGithub),
		g.noLink(database.LoginTypePassword),
	}

	// Insert some orgs, users, and links
	orgA := dbfake.Organization(t, db).Members(
		append(problematics,
			alice,
			bob)...,
	).Do()
	orgB := dbfake.Organization(t, db).Members(
		append(problematics,
			charlie,
		)...,
	).Do()

	// Verify the OIDC claim fields
	requireClaims(t, db, orgA.Org.ID, []string{"sub"})
	requireClaims(t, db, orgB.Org.ID, []string{"sub"})
}

func requireClaims(t *testing.T, db database.Store, orgID uuid.UUID, want []string) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitMedium)
	got, err := db.OIDCClaimFields(ctx, orgID)
	require.NoError(t, err)

	require.ElementsMatch(t, want, got)
}

type userGenerator struct {
	t  *testing.T
	db database.Store
}

func (g userGenerator) noLink(lt database.LoginType) database.User {
	return g.user(lt, false, nil)
}

func (g userGenerator) withLink(lt database.LoginType, rawJSON json.RawMessage) database.User {
	return g.user(lt, true, rawJSON)
}

func (g userGenerator) user(lt database.LoginType, createLink bool, rawJSON json.RawMessage) database.User {
	t := g.t
	db := g.db

	t.Helper()

	u, err := db.InsertUser(context.Background(), database.InsertUserParams{
		ID:        uuid.New(),
		Email:     testutil.GetRandomName(t),
		Username:  testutil.GetRandomName(t),
		Name:      testutil.GetRandomName(t),
		CreatedAt: dbtime.Now(),
		UpdatedAt: dbtime.Now(),
		RBACRoles: []string{},
		LoginType: lt,
		Status:    string(database.UserStatusActive),
	})
	require.NoError(t, err)

	if !createLink {
		return u
	}

	link, err := db.InsertUserLink(context.Background(), database.InsertUserLinkParams{
		UserID:    u.ID,
		LoginType: lt,
		Claims:    database.UserLinkClaims{},
	})
	require.NoError(t, err)

	if sql, ok := db.(rawUpdater); ok {
		// The only way to put arbitrary json into the db for testing edge cases.
		// Making this a public API would be a mistake.
		err = sql.UpdateUserLinkRawJSON(context.Background(), u.ID, rawJSON)
		require.NoError(t, err)
	} else {
		// no need to test the json key logic in dbmem. Everything is type safe.
		var claims database.UserLinkClaims
		err := json.Unmarshal(rawJSON, &claims)
		require.NoError(t, err)

		_, err = db.UpdateUserLink(context.Background(), database.UpdateUserLinkParams{
			OAuthAccessToken:       link.OAuthAccessToken,
			OAuthAccessTokenKeyID:  link.OAuthAccessTokenKeyID,
			OAuthRefreshToken:      link.OAuthRefreshToken,
			OAuthRefreshTokenKeyID: link.OAuthRefreshTokenKeyID,
			OAuthExpiry:            link.OAuthExpiry,
			UserID:                 link.UserID,
			LoginType:              link.LoginType,
			// The new claims
			Claims: claims,
		})
		require.NoError(t, err)
	}

	return u
}

type rawUpdater interface {
	UpdateUserLinkRawJSON(ctx context.Context, userID uuid.UUID, data json.RawMessage) error
}
