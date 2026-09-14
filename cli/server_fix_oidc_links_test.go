package cli_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

// fakeOIDCDiscovery returns a test server serving an OIDC discovery document.
func fakeOIDCDiscovery(t *testing.T, issuer string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": issuer,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFixOIDCLinks(t *testing.T) {
	t.Parallel()

	t.Run("DryRun", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		t.Cleanup(cancel)

		const expectedIssuer = "https://accounts.google.com"
		oidcSrv := fakeOIDCDiscovery(t, expectedIssuer)

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer sqlDB.Close()

		db := database.New(sqlDB)

		// Seed a correctly linked user.
		correctUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    correctUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  expectedIssuer + "||sub-correct",
		})

		// Seed a mismatched user.
		mismatchedUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    mismatchedUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://old-issuer.example.com||sub-mismatched",
		})

		inv, _ := clitest.New(t,
			"server", "fix-oidc-links",
			"--postgres-url", connectionURL,
			"--issuer-url", oidcSrv.URL,
			"--dry-run",
		)

		stdout := expecter.NewAttachedToInvocation(t, inv)
		w := clitest.StartWithWaiter(t, inv)

		stdout.ExpectMatch(ctx, "Resolved OIDC issuer: \""+expectedIssuer+"\"")
		stdout.ExpectMatch(ctx, "Total OIDC users:")
		stdout.ExpectMatch(ctx, "Correctly linked:")
		stdout.ExpectMatch(ctx, "Linked to other issuers:")
		w.RequireSuccess()

		// Verify no changes were made.
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    mismatchedUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "https://old-issuer.example.com||sub-mismatched", link.LinkedID, "dry-run must not modify the database")
	})

	t.Run("Confirm", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		t.Cleanup(cancel)

		const expectedIssuer = "https://accounts.google.com"
		oidcSrv := fakeOIDCDiscovery(t, expectedIssuer)

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer sqlDB.Close()

		db := database.New(sqlDB)

		// Seed a correctly linked user.
		correctUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    correctUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  expectedIssuer + "||sub-correct",
		})

		// Seed mismatched users.
		mismatchedUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    mismatchedUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://old-issuer.example.com||sub-mismatched",
		})

		inv, _ := clitest.New(t,
			"server", "fix-oidc-links",
			"--postgres-url", connectionURL,
			"--issuer-url", oidcSrv.URL,
			"--yes",
		)

		stdout := expecter.NewAttachedToInvocation(t, inv)
		w := clitest.StartWithWaiter(t, inv)

		stdout.ExpectMatch(ctx, "Reset 1 linked IDs.")
		w.RequireSuccess()

		// Verify the mismatched link was reset.
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    mismatchedUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "", link.LinkedID)

		// Verify the correct link is unchanged.
		link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    correctUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, expectedIssuer+"||sub-correct", link.LinkedID)
	})

	t.Run("ForceResetAll", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		t.Cleanup(cancel)

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer sqlDB.Close()

		db := database.New(sqlDB)

		// Seed users with different issuers.
		user1 := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    user1.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://accounts.google.com||sub-1",
		})

		user2 := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    user2.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://old-issuer.example.com||sub-2",
		})

		inv, _ := clitest.New(t,
			"server", "fix-oidc-links",
			"--postgres-url", connectionURL,
			"--force-reset-all",
			"--yes",
		)

		stdout := expecter.NewAttachedToInvocation(t, inv)
		w := clitest.StartWithWaiter(t, inv)

		stdout.ExpectMatch(ctx, "Linked to other issuers:")
		stdout.ExpectMatch(ctx, "Reset 2 linked IDs.")
		w.RequireSuccess()

		// Verify both links were reset.
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user1.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "", link.LinkedID)

		link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user2.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "", link.LinkedID)
	})

	t.Run("ForceResetAllDryRun", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		t.Cleanup(cancel)

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer sqlDB.Close()

		db := database.New(sqlDB)

		// Seed users with different issuers.
		user1 := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    user1.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://accounts.google.com||sub-1",
		})

		user2 := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    user2.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://old-issuer.example.com||sub-2",
		})

		inv, _ := clitest.New(t,
			"server", "fix-oidc-links",
			"--postgres-url", connectionURL,
			"--force-reset-all",
			"--dry-run",
		)

		stdout := expecter.NewAttachedToInvocation(t, inv)
		w := clitest.StartWithWaiter(t, inv)

		stdout.ExpectMatch(ctx, "Total OIDC users:")
		stdout.ExpectMatch(ctx, "Linked to other issuers:")
		w.RequireSuccess()

		// Verify no changes were made.
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user1.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "https://accounts.google.com||sub-1", link.LinkedID, "dry-run must not modify the database")

		link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    user2.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "https://old-issuer.example.com||sub-2", link.LinkedID, "dry-run must not modify the database")
	})

	t.Run("NothingToDo", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		t.Cleanup(cancel)

		const expectedIssuer = "https://accounts.google.com"
		oidcSrv := fakeOIDCDiscovery(t, expectedIssuer)

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer sqlDB.Close()

		db := database.New(sqlDB)

		// All users correctly linked.
		user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    user.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  expectedIssuer + "||sub-correct",
		})

		inv, _ := clitest.New(t,
			"server", "fix-oidc-links",
			"--postgres-url", connectionURL,
			"--issuer-url", oidcSrv.URL,
			"--yes",
		)

		stdout := expecter.NewAttachedToInvocation(t, inv)
		w := clitest.StartWithWaiter(t, inv)

		stdout.ExpectMatch(ctx, "Nothing to do")
		w.RequireSuccess()
	})
}
