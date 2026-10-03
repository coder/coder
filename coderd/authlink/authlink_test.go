package authlink_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/authlink"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestAnalyzeOIDCLinks(t *testing.T) {
	t.Parallel()

	t.Run("MixedIssuers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)
		const expectedIssuer = "https://accounts.google.com"

		// 3 users linked to the expected issuer.
		for i := 0; i < 3; i++ {
			user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
			dbgen.UserLink(t, db, database.UserLink{
				UserID:    user.ID,
				LoginType: database.LoginTypeOIDC,
				LinkedID:  expectedIssuer + "||sub-" + user.ID.String(),
			})
		}

		// 2 users linked to an old issuer.
		for i := 0; i < 2; i++ {
			user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
			dbgen.UserLink(t, db, database.UserLink{
				UserID:    user.ID,
				LoginType: database.LoginTypeOIDC,
				LinkedID:  "https://old-issuer.example.com||sub-" + user.ID.String(),
			})
		}

		// 1 user linked to another old issuer.
		user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    user.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://staging.example.com||sub-" + user.ID.String(),
		})

		// 1 unlinked user (empty linked_id).
		unlinkedUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    unlinkedUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "",
		})

		analysis, err := authlink.AnalyzeOIDCLinks(ctx, db, expectedIssuer)
		require.NoError(t, err)

		require.Equal(t, 7, analysis.Total)
		require.Equal(t, 3, analysis.CorrectIssuer)
		require.Equal(t, 1, analysis.Unlinked)
		require.Equal(t, 3, analysis.MismatchedTotal())
		require.Equal(t, 2, analysis.MismatchedCounts["https://old-issuer.example.com"])
		require.Equal(t, 1, analysis.MismatchedCounts["https://staging.example.com"])
	})

	t.Run("NoOIDCUsers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)

		analysis, err := authlink.AnalyzeOIDCLinks(ctx, db, "https://issuer.example.com")
		require.NoError(t, err)
		require.Equal(t, 0, analysis.Total)
		require.Equal(t, 0, analysis.CorrectIssuer)
		require.Equal(t, 0, analysis.Unlinked)
		require.Equal(t, 0, analysis.MismatchedTotal())
	})

	t.Run("AllCorrect", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)
		const expectedIssuer = "https://accounts.google.com"

		for i := 0; i < 3; i++ {
			user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
			dbgen.UserLink(t, db, database.UserLink{
				UserID:    user.ID,
				LoginType: database.LoginTypeOIDC,
				LinkedID:  expectedIssuer + "||sub-" + user.ID.String(),
			})
		}

		analysis, err := authlink.AnalyzeOIDCLinks(ctx, db, expectedIssuer)
		require.NoError(t, err)
		require.Equal(t, 3, analysis.Total)
		require.Equal(t, 3, analysis.CorrectIssuer)
		require.Equal(t, 0, analysis.Unlinked)
		require.Equal(t, 0, analysis.MismatchedTotal())
	})

	t.Run("DeletedUsersExcluded", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)
		const expectedIssuer = "https://accounts.google.com"

		// Active user with mismatched issuer.
		activeUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    activeUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://old-issuer.example.com||sub-active",
		})

		// Create user and link first, then soft-delete the user.
		// The DB trigger prevents inserting links for already-deleted users.
		deletedUser := dbgen.User(t, db, database.User{
			LoginType: database.LoginTypeOIDC,
		})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    deletedUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://old-issuer.example.com||sub-deleted",
		})
		require.NoError(t, db.UpdateUserDeletedByID(ctx, deletedUser.ID))

		analysis, err := authlink.AnalyzeOIDCLinks(ctx, db, expectedIssuer)
		require.NoError(t, err)
		require.Equal(t, 1, analysis.Total)
		require.Equal(t, 1, analysis.MismatchedTotal())
	})

	t.Run("NonOIDCLinksExcluded", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)
		const expectedIssuer = "https://accounts.google.com"

		// GitHub user link should not be counted.
		ghUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeGithub})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    ghUser.ID,
			LoginType: database.LoginTypeGithub,
			LinkedID:  "github||12345",
		})

		analysis, err := authlink.AnalyzeOIDCLinks(ctx, db, expectedIssuer)
		require.NoError(t, err)
		require.Equal(t, 0, analysis.Total)
	})
}

func TestResetMismatchedOIDCLinks(t *testing.T) {
	t.Parallel()

	t.Run("ResetsOnlyMismatched", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)
		const expectedIssuer = "https://accounts.google.com"

		// Correctly linked user.
		correctUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		correctLink := dbgen.UserLink(t, db, database.UserLink{
			UserID:    correctUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  expectedIssuer + "||sub-correct",
		})

		// Mismatched user.
		mismatchedUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    mismatchedUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://old-issuer.example.com||sub-mismatched",
		})

		// Unlinked user (empty linked_id).
		unlinkedUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    unlinkedUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "",
		})

		count, err := authlink.ResetMismatchedOIDCLinks(ctx, db, expectedIssuer)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)

		// Verify the correct link is unchanged.
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    correctUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, correctLink.LinkedID, link.LinkedID)

		// Verify the mismatched link was reset.
		link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    mismatchedUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "", link.LinkedID)

		// Verify the unlinked user is still unlinked.
		link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    unlinkedUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "", link.LinkedID)
	})

	t.Run("NothingToReset", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)
		const expectedIssuer = "https://accounts.google.com"

		user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    user.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  expectedIssuer + "||sub-correct",
		})

		count, err := authlink.ResetMismatchedOIDCLinks(ctx, db, expectedIssuer)
		require.NoError(t, err)
		require.EqualValues(t, 0, count)
	})
}

func TestResetMismatchedOIDCLinksWithUnmatchableIssuer(t *testing.T) {
	t.Parallel()

	t.Run("ResetsAll", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)

		// Correctly linked user.
		correctUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    correctUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://accounts.google.com||sub-correct",
		})

		// Mismatched user.
		mismatchedUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    mismatchedUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "https://old-issuer.example.com||sub-mismatched",
		})

		// Unlinked user (empty linked_id).
		unlinkedUser := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    unlinkedUser.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "",
		})

		count, err := authlink.ResetMismatchedOIDCLinks(ctx, db, authlink.UnmatchableIssuer)
		require.NoError(t, err)
		require.EqualValues(t, 2, count, "should reset correct + mismatched, not unlinked")

		// Verify the correct link was reset.
		link, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    correctUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "", link.LinkedID)

		// Verify the mismatched link was reset.
		link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    mismatchedUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "", link.LinkedID)

		// Verify the unlinked user is still unlinked.
		link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    unlinkedUser.ID,
			LoginType: database.LoginTypeOIDC,
		})
		require.NoError(t, err)
		require.Equal(t, "", link.LinkedID)
	})

	t.Run("NothingToReset", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		db, _ := dbtestutil.NewDB(t)

		// Only an unlinked user.
		user := dbgen.User(t, db, database.User{LoginType: database.LoginTypeOIDC})
		dbgen.UserLink(t, db, database.UserLink{
			UserID:    user.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  "",
		})

		count, err := authlink.ResetMismatchedOIDCLinks(ctx, db, authlink.UnmatchableIssuer)
		require.NoError(t, err)
		require.EqualValues(t, 0, count)
	})
}

func TestResolveIssuer(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		const expectedIssuer = "https://accounts.google.com"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/.well-known/openid-configuration" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer": expectedIssuer,
			})
		}))
		defer srv.Close()

		issuer, err := authlink.ResolveIssuer(ctx, srv.Client(), srv.URL)
		require.NoError(t, err)
		require.Equal(t, expectedIssuer, issuer)
	})

	t.Run("EmptyIssuer", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer": "",
			})
		}))
		defer srv.Close()

		_, err := authlink.ResolveIssuer(ctx, srv.Client(), srv.URL)
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty issuer")
	})

	t.Run("HTTPError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := authlink.ResolveIssuer(ctx, srv.Client(), srv.URL)
		require.Error(t, err)
		require.Contains(t, err.Error(), "HTTP 500")
	})
}
