package database_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/util/slice"
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

	const claimField = "claim-list"

	// https://en.wikipedia.org/wiki/Alice_and_Bob#Cast_of_characters
	alice := g.withLink(database.LoginTypeOIDC, toJSON(extraKeys{
		UserLinkClaims: database.UserLinkClaims{
			IDTokenClaims: map[string]interface{}{
				"sub":      "alice",
				"alice-id": "from-bob",
			},
			UserInfoClaims: nil,
			MergedClaims: map[string]interface{}{
				"sub":      "alice",
				"alice-id": "from-bob",
				claimField: []string{
					"one", "two", "three",
				},
			},
		},
		// Always should be a no-op
		Foo: "bar",
	}))
	bob := g.withLink(database.LoginTypeOIDC, toJSON(database.UserLinkClaims{
		IDTokenClaims: map[string]interface{}{
			"sub":    "bob",
			"bob-id": "from-bob",
			"array": []string{
				"a", "b", "c",
			},
			"map": map[string]interface{}{
				"key": "value",
				"foo": "bar",
			},
			"nil": nil,
		},
		UserInfoClaims: map[string]interface{}{
			"sub":      "bob",
			"bob-info": []string{},
			"number":   42,
		},
		MergedClaims: map[string]interface{}{
			"sub":      "bob",
			"bob-info": []string{},
			"number":   42,
			"bob-id":   "from-bob",
			"array": []string{
				"a", "b", "c",
			},
			"map": map[string]interface{}{
				"key": "value",
				"foo": "bar",
			},
			"nil": nil,
			claimField: []any{
				"three", 5, []string{"test"}, "four",
			},
		},
	}))
	charlie := g.withLink(database.LoginTypeOIDC, toJSON(database.UserLinkClaims{
		IDTokenClaims: map[string]interface{}{
			"sub":        "charlie",
			"charlie-id": "charlie",
		},
		UserInfoClaims: map[string]interface{}{
			"sub":          "charlie",
			"charlie-info": "charlie",
		},
		MergedClaims: map[string]interface{}{
			"sub":          "charlie",
			"charlie-id":   "charlie",
			"charlie-info": "charlie",
			claimField:     "charlie",
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
			MergedClaims: map[string]interface{}{
				"not":      "allowed",
				"do-not":   "look",
				claimField: 42,
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
			bob,
		)...,
	).Do()
	orgB := dbfake.Organization(t, db).Members(
		append(problematics,
			bob,
			charlie,
		)...,
	).Do()
	orgC := dbfake.Organization(t, db).Members().Do()

	// Verify the OIDC claim fields
	always := []string{"array", "map", "nil", "number"}
	expectA := append([]string{"sub", "alice-id", "bob-id", "bob-info", "claim-list"}, always...)
	expectB := append([]string{"sub", "bob-id", "bob-info", "charlie-id", "charlie-info", "claim-list"}, always...)
	requireClaims(t, db, orgA.Org.ID, expectA)
	requireClaims(t, db, orgB.Org.ID, expectB)
	requireClaims(t, db, orgC.Org.ID, []string{})
	requireClaims(t, db, uuid.Nil, slice.Unique(append(expectA, expectB...)))

	// Verify the claim field values
	expectAValues := []string{"one", "two", "three", "four"}
	expectBValues := []string{"three", "four", "charlie"}
	requireClaimValues(t, db, orgA.Org.ID, claimField, expectAValues)
	requireClaimValues(t, db, orgB.Org.ID, claimField, expectBValues)
	requireClaimValues(t, db, orgC.Org.ID, claimField, []string{})
}

func requireClaimValues(t *testing.T, db database.Store, orgID uuid.UUID, field string, want []string) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitMedium)
	got, err := db.OIDCClaimFieldValues(ctx, database.OIDCClaimFieldValuesParams{
		ClaimField:     field,
		OrganizationID: orgID,
	})
	require.NoError(t, err)

	require.ElementsMatch(t, want, got)
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
	t := g.t
	db := g.db

	t.Helper()

	u := dbgen.User(t, db, database.User{
		LoginType: lt,
	})
	return u
}

func (g userGenerator) withLink(lt database.LoginType, rawJSON json.RawMessage) database.User {
	t := g.t
	db := g.db

	user := g.noLink(lt)

	link := dbgen.UserLink(t, db, database.UserLink{
		UserID:    user.ID,
		LoginType: lt,
	})

	if sql, ok := db.(rawUpdater); ok {
		// The only way to put arbitrary json into the db for testing edge cases.
		// Making this a public API would be a mistake.
		err := sql.UpdateUserLinkRawJSON(context.Background(), user.ID, rawJSON)
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

	return user
}

type rawUpdater interface {
	UpdateUserLinkRawJSON(ctx context.Context, userID uuid.UUID, data json.RawMessage) error
}
