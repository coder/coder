package oauth2provider_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// Cases assert against the api_keys row, not the response: that row is what
// dbauthz reads on each later request.
func TestOAuth2TokenExchangeScope(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	owner := coderdtest.CreateFirstUser(t, client)

	t.Run("NegotiatedScopeMintsNarrowKey", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, token.RefreshToken))
	})

	// RFC 6749 §4.1.3 defines no scope parameter here, but the form carries
	// one, and discarding it hands back what the client gave up.
	t.Run("ExchangeNarrowsTheAccessToken", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")

		form := tokenExchangeForm(app, code, verifier)
		form.Set("scope", "workspace:ssh")
		status, body := postTokenRequest(ctx, t, client, form)
		token := requireTokenResponse(t, status, body)

		require.Equal(t, "workspace:ssh", token.Scope)
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, token.RefreshToken))
		require.Equal(t, scopeInCatalog, tokenRow(ctx, t, db, token.RefreshToken).Scope,
			"the grant is what the user consented to, not what the exchange asked for")
	})

	t.Run("ExchangeCannotWidenTheScope", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")

		form := tokenExchangeForm(app, code, verifier)
		form.Set("scope", scopeAlsoInCatalog)
		status, body := postTokenRequest(ctx, t, client, form)

		require.Contains(t, requireTokenScopeError(t, status, body),
			oauth2provider.ReasonScopeNotGranted)
	})

	t.Run("RefreshDoesNotWidenTheScope", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		status, body := postTokenRequest(ctx, t, client, refreshForm(app, token.RefreshToken))
		refreshed := requireTokenResponse(t, status, body)
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken))
		require.Equal(t, "workspace:ssh", refreshed.Scope)
	})

	// coder:workspaces.access covers workspace:ssh, so this gives up real
	// authority. The refresh token still carries the grant.
	t.Run("RefreshNarrowsTheAccessToken", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		form := refreshForm(app, token.RefreshToken)
		form.Set("scope", "workspace:ssh")
		status, body := postTokenRequest(ctx, t, client, form)
		refreshed := requireTokenResponse(t, status, body)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken))
		require.Equal(t, "workspace:ssh", refreshed.Scope)
		require.Equal(t, scopeInCatalog, tokenRow(ctx, t, db, refreshed.RefreshToken).Scope,
			"OAuth 2.1 §4.3.3: a rotated refresh token carries the scope of the one presented")
	})

	// Narrowed earlier, now needs a different part of the same grant, which
	// OAuth 2.1 §4.3 names as a reason to refresh.
	t.Run("NarrowingDoesNotBindLaterRefreshes", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		form := refreshForm(app, token.RefreshToken)
		form.Set("scope", "workspace:ssh")
		status, body := postTokenRequest(ctx, t, client, form)
		narrowed := requireTokenResponse(t, status, body)

		// A sibling permission of the same grant, which the user consented to.
		form = refreshForm(app, narrowed.RefreshToken)
		form.Set("scope", "workspace:read")
		status, body = postTokenRequest(ctx, t, client, form)
		sibling := requireTokenResponse(t, status, body)
		require.Equal(t, "workspace:read", sibling.Scope)
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceRead},
			mintedKeyScopes(ctx, t, db, sibling.RefreshToken))

		// And the whole grant back, per RFC 6749 §6's omitted-scope default.
		status, body = postTokenRequest(ctx, t, client, refreshForm(app, sibling.RefreshToken))
		restored := requireTokenResponse(t, status, body)
		require.Equal(t, scopeInCatalog, restored.Scope,
			"an omitted scope is the scope originally granted by the resource owner")
		require.Equal(t, scopeInCatalog, tokenRow(ctx, t, db, restored.RefreshToken).Scope)
	})

	t.Run("RefreshCannotWidenTheScope", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		form := refreshForm(app, token.RefreshToken)
		form.Set("scope", scopeAlsoInCatalog)
		status, body := postTokenRequest(ctx, t, client, form)

		description := requireTokenScopeError(t, status, body)
		require.Contains(t, description, oauth2provider.ReasonScopeNotGranted)
		require.Contains(t, description, scopeAlsoInCatalog)
		// Without it a client retries combinations that cannot succeed.
		require.Contains(t, description, "authorize again",
			"the rejection must name the only way to a broader grant")
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, token.RefreshToken),
			"a rejected refresh issues nothing")

		// Through the endpoint: reading the row would still pass if the
		// rejection had rotated the hash or moved ExpiresAt.
		status, body = postTokenRequest(ctx, t, client, refreshForm(app, token.RefreshToken))
		require.Equal(t, "workspace:ssh", requireTokenResponse(t, status, body).Scope,
			"a rejected refresh leaves the original token redeemable")
	})

	t.Run("RefreshUnknownScopeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		form := refreshForm(app, token.RefreshToken)
		form.Set("scope", "not_a_real_scope")
		status, body := postTokenRequest(ctx, t, client, form)

		description := requireTokenScopeError(t, status, body)
		require.Contains(t, description, oauth2provider.ReasonUnknownScope)
		// The catalog check runs first so a typo gets the name to fix.
		require.Contains(t, description, "not_a_real_scope",
			"the client cannot fix its request without the name that failed")
	})

	// RFC 6749 §5.1 only requires the parameter when the issued scope differs
	// from the request, so both halves here make it differ.
	t.Run("ResponseStatesTheScopeGranted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
		token := exchangeCode(ctx, t, client, app, code, verifier)
		require.Equal(t, scopeInCatalog, token.Scope)

		unrestricted := seedAppWithSecret(t, db, sql.NullString{})
		code, verifier = authorizeCode(ctx, t, client, unrestricted.ID.String(), "")
		granted := exchangeCode(ctx, t, client, unrestricted, code, verifier)
		require.Equal(t, string(database.ApiKeyScopeCoderAll), granted.Scope)

		// Requestable as "all", never granted under that spelling.
		form := refreshForm(unrestricted, granted.RefreshToken)
		form.Set("scope", "all")
		status, body := postTokenRequest(ctx, t, client, form)
		refreshed := requireTokenResponse(t, status, body)

		require.Equal(t, string(database.ApiKeyScopeCoderAll), refreshed.Scope,
			"the response states the granted spelling, not the requested one")
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken),
			"api_key_scope has no member spelled all, so an uncanonicalized mint fails here")
		require.Equal(t, string(database.ApiKeyScopeCoderAll),
			tokenRow(ctx, t, db, refreshed.RefreshToken).Scope)
	})

	// apikey.Generate defaults an empty scope list to coder:all, so this passes
	// even if the exchange drops the scope. It pins the unrestricted path, not
	// that the scope was applied.
	t.Run("UnrestrictedGrantMintsCoderAll", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			mintedKeyScopes(ctx, t, db, token.RefreshToken))
	})

	// coder:workspaces.access carries template:read but not template:delete. The
	// grant's user owns the deployment, so the role permits both and the scope is
	// all that stands between the client and the deletion.
	t.Run("IssuedTokenBoundsTheAPI", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      owner.UserID,
		})

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		require.Equal(t, scopeInCatalog, token.Scope,
			"RFC 6749 §5.1: a request that named no scope must be told what it got")

		asApp := codersdk.New(client.URL)
		asApp.SetSessionToken(token.AccessToken)

		got, err := asApp.Template(ctx, tpl.ID)
		require.NoError(t, err, "template:read is within the negotiated scope")
		require.Equal(t, tpl.ID, got.ID)

		err = asApp.DeleteTemplate(ctx, tpl.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	// Grants predating the scope columns carry what migration 000569 backfilled:
	// coder:all. Seeded the way the migration leaves it rather than exchanged.
	// An alias the api_key_scope enum does not hold, so both exits have to
	// canonicalize it.
	t.Run("LegacyAliasRefreshesTheSameEitherWay", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Two apps, not two tokens on one: nothing enforces a single holder
		// of a refreshed key's name for this login type.
		omittedApp := seedAppWithSecret(t, db, sql.NullString{})
		narrowingApp := seedAppWithSecret(t, db, sql.NullString{})

		omitted := seedRefreshToken(ctx, t, db, omittedApp, owner.UserID, "all")
		status, body := postTokenRequest(ctx, t, client, refreshForm(omittedApp, omitted))
		refreshed := requireTokenResponse(t, status, body)
		require.Equal(t, string(database.ApiKeyScopeCoderAll), refreshed.Scope)
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken))

		narrowing := seedRefreshToken(ctx, t, db, narrowingApp, owner.UserID, "all")
		form := refreshForm(narrowingApp, narrowing)
		form.Set("scope", "workspace:read")
		status, body = postTokenRequest(ctx, t, client, form)
		require.Equal(t, "workspace:read", requireTokenResponse(t, status, body).Scope,
			"the alias must resolve the same way whether or not a scope is named")
	})

	t.Run("BackfilledScopeRefreshesUnrestricted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      owner.UserID,
		})

		app := seedAppWithSecret(t, db, sql.NullString{})
		refreshToken := seedRefreshToken(ctx, t, db, app, owner.UserID, string(database.ApiKeyScopeCoderAll))

		status, body := postTokenRequest(ctx, t, client, refreshForm(app, refreshToken))
		refreshed := requireTokenResponse(t, status, body)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken))

		asApp := codersdk.New(client.URL)
		asApp.SetSessionToken(refreshed.AccessToken)
		require.NoError(t, asApp.DeleteTemplate(ctx, tpl.ID),
			"an unrestricted grant must still reach what it reached before")
	})

	// Authorization cannot write such a row, so it is seeded: the case covers a
	// name removed since, or a row written by another version of this server.
	t.Run("StoredScopeOutsideEnumRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		code := seedCode(ctx, t, db, app.ID, owner.UserID, challenge, scopeOutOfCatalog)

		status, body := postTokenRequest(ctx, t, client, tokenExchangeForm(app, code, verifier))

		description := requireTokenGrantError(t, status, body)
		require.Contains(t, description, oauth2provider.ReasonUnmintableScope,
			"the mint check is what this case is named for, and both sentinels echo the scope")
		require.Contains(t, description, scopeOutOfCatalog,
			"an operator cannot act on this without knowing which stored name is the problem")
	})

	t.Run("AllowlistNarrowedAfterAuthorizationRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
		setAppAllowlist(ctx, t, db, app, sql.NullString{String: scopeAlsoInCatalog, Valid: true})

		status, body := postTokenRequest(ctx, t, client, tokenExchangeForm(app, code, verifier))

		description := requireTokenGrantError(t, status, body)
		require.Contains(t, description, oauth2provider.ReasonStaleScope)
		require.Contains(t, description, "workspace:ssh",
			"the client needs the scope name to know what was withdrawn")
		requireNoSessionKey(ctx, t, db, owner.UserID, app.ID)
	})

	t.Run("AllowlistWidenedAfterAuthorizationStillRedeems", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
		setAppAllowlist(ctx, t, db, app, sql.NullString{String: scopeInCatalog + " " + scopeAlsoInCatalog, Valid: true})

		token := exchangeCode(ctx, t, client, app, code, verifier)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, token.RefreshToken))
		requireCodeConsumed(ctx, t, db, code)
	})

	t.Run("RefreshIgnoresAllowlistNarrowing", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
		token := exchangeCode(ctx, t, client, app, code, verifier)
		setAppAllowlist(ctx, t, db, app, sql.NullString{String: scopeAlsoInCatalog, Valid: true})

		status, body := postTokenRequest(ctx, t, client, refreshForm(app, token.RefreshToken))
		refreshed := requireTokenResponse(t, status, body)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken))
	})
}

// The token endpoint's error_description obeys RFC 6749 §5.2, on the decoded
// value, and is bounded.
func TestOAuth2TokenErrorDescription(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	coderdtest.CreateFirstUser(t, client)

	refreshWithScope := func(ctx context.Context, t *testing.T, scope string) string {
		t.Helper()

		app := seedAppWithSecret(t, db, sql.NullString{})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		form := refreshForm(app, token.RefreshToken)
		form.Set("scope", scope)
		status, body := postTokenRequest(ctx, t, client, form)
		return requireTokenScopeError(t, status, body)
	}

	t.Run("UnknownScopeEchoIsSanitized", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// No whitespace, or strings.Fields splits it.
		description := refreshWithScope(ctx, t, "\x07\x1b[31m\"\\caf\u00e9")

		requireNQSCHAR(t, description)
		require.Contains(t, description, oauth2provider.ReasonUnknownScope,
			"sanitizing must not cost the client the reason")
	})

	t.Run("UnknownScopeEchoIsCapped", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		description := refreshWithScope(ctx, t, strings.Repeat("x", oauth2provider.MaxErrorDescription*8))

		requireNQSCHAR(t, description)
		require.LessOrEqual(t, len(description), oauth2provider.MaxErrorDescription+len(" (truncated)"))
		require.Contains(t, description, "(truncated)")
	})

	// The sanitizer runs on every description, so a fixed message outside the
	// set silently loses characters. A section sign is the easy mistake.
	t.Run("FixedMessageIsUnchangedBySanitizing", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{})
		code, _ := authorizeCode(ctx, t, client, app.ID.String(), "")

		// Rejected for its length before the code is ever looked up.
		form := tokenExchangeForm(app, code, "too-short")
		status, body := postTokenRequest(ctx, t, client, form)
		description := requireTokenError(t, status, body, codersdk.OAuth2ErrorCodeInvalidRequest)
		requireNQSCHAR(t, description)
		require.Contains(t, description, "RFC 7636 section 4.1")
	})
}

// The redemptions race rather than run in sequence: a sequential pair passes
// whether or not the delete arbitrates single use. barrierStore makes that
// overlap deterministic instead of probabilistic.
func TestOAuth2TokenExchangeSingleUse(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	var reads sync.WaitGroup
	reads.Add(2)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: barrierStore{Store: db, reads: &reads},
		Pubsub:   pubsub,
	})
	coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	app := seedAppWithSecret(t, db, sql.NullString{})
	code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
	form := tokenExchangeForm(app, code, verifier)

	type exchange struct {
		status int
		body   string
		err    error
	}

	redeem := func() exchange {
		status, body, err := tryTokenRequest(ctx, t, client, form)
		return exchange{status: status, body: body, err: err}
	}

	other := make(chan exchange, 1)
	go func() { other <- redeem() }()
	results := []exchange{redeem(), <-other}

	var winner codersdk.OAuth2TokenResponse
	var minted, rejected int
	for _, result := range results {
		require.NoError(t, result.err)
		switch result.status {
		case http.StatusOK:
			winner = requireTokenResponse(t, result.status, result.body)
			minted++
		case http.StatusBadRequest:
			require.Contains(t, result.body, string(codersdk.OAuth2ErrorCodeInvalidGrant), result.body)
			rejected++
		default:
			t.Fatalf("unexpected status %d: %s", result.status, result.body)
		}
	}
	require.Equal(t, 1, minted, "a code may mint at most one token")
	require.Equal(t, 1, rejected)
	requireTokenAuthenticates(ctx, t, client, winner.AccessToken)
}

// A refresh mints a replacement token and deletes the key the presented one
// hangs off, so the same race as the exchange applies: the deletion has to
// arbitrate, or both requests mint from one refresh token.
func TestOAuth2RefreshSingleUse(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
	code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
	token := exchangeCode(ctx, t, client, app, code, verifier)

	requireExactlyOneMinted(ctx, t, client, refreshForm(app, token.RefreshToken),
		"a refresh token may mint at most one replacement")
}

// requireExactlyOneMinted posts form twice concurrently and requires one 200 and
// one `invalid_grant`. The two requests are started together rather than held at
// a read, so unlike the exchange's barrierStore this overlaps them without
// pinning which of the two arbitrates.
func requireExactlyOneMinted(ctx context.Context, t *testing.T, client *codersdk.Client, form url.Values, msg string) {
	t.Helper()

	type attempt struct {
		status int
		body   string
		err    error
	}

	var barrier sync.WaitGroup
	barrier.Add(2)
	redeem := func() attempt {
		barrier.Done()
		barrier.Wait()
		status, body, err := tryTokenRequest(ctx, t, client, form)
		return attempt{status: status, body: body, err: err}
	}

	other := make(chan attempt, 1)
	go func() { other <- redeem() }()
	results := []attempt{redeem(), <-other}

	var minted, rejected int
	for _, result := range results {
		require.NoError(t, result.err)
		switch result.status {
		case http.StatusOK:
			minted++
		case http.StatusBadRequest:
			require.Contains(t, result.body, string(codersdk.OAuth2ErrorCodeInvalidGrant), result.body)
			rejected++
		default:
			t.Fatalf("unexpected status %d: %s", result.status, result.body)
		}
	}
	require.Equal(t, 1, minted, msg)
	require.Equal(t, 1, rejected)
}

// The ordinary replay: a client retries a redemption whose answer it never saw.
// Here the first read refuses it. The race test cannot cover this path
// deterministically, since which read or delete arbitrates there depends on
// scheduling.
func TestOAuth2TokenExchangeReplay(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	app := seedAppWithSecret(t, db, sql.NullString{})
	code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
	token := exchangeCode(ctx, t, client, app, code, verifier)

	status, body := postTokenRequest(ctx, t, client, tokenExchangeForm(app, code, verifier))
	requireTokenGrantError(t, status, body)
	requireTokenAuthenticates(ctx, t, client, token.AccessToken)
}

// barrierStore holds each redemption at its code read until every redemption
// has read, so both reach the delete with the same stale view. Starting the
// requests together is not enough on its own: nothing stops one handler from
// committing before the other reads, and the read then refuses the second
// before the delete ever arbitrates.
//
// InTx hands its closure a fresh Store, so this intercepts only the read that
// precedes the transaction, which is the one that fixes the interleaving.
type barrierStore struct {
	database.Store
	reads *sync.WaitGroup
}

// GetOAuth2ProviderAppCodeByPrefix has one production caller, the code read in
// authorizationCodeGrant, so every arrival here is a redemption.
func (s barrierStore) GetOAuth2ProviderAppCodeByPrefix(ctx context.Context, prefix []byte) (database.OAuth2ProviderAppCode, error) {
	code, err := s.Store.GetOAuth2ProviderAppCodeByPrefix(ctx, prefix)
	s.reads.Done()
	s.reads.Wait()
	return code, err
}

// requireTokenAuthenticates asserts the accepted redemption's own credential
// still works. Callers grant coder:all so the probed endpoint is in scope.
//
// Row counts cannot show this: the grant deletes whatever key already holds
// the name it is about to write, and oauth2_provider_app_tokens cascades on
// that delete, so the rows converge on one key and one token however many
// redemptions succeed. Only the winner's token separates "its key survived"
// from "a second redemption rotated it out".
func requireTokenAuthenticates(ctx context.Context, t *testing.T, client *codersdk.Client, accessToken string) {
	t.Helper()

	asApp := codersdk.New(client.URL)
	asApp.SetSessionToken(accessToken)
	_, err := asApp.User(ctx, codersdk.Me)
	require.NoError(t, err, "a refused redemption must leave the accepted one's token usable")
}

// appWithSecret is seeded directly because the management API registers no
// scope allowlist, and the allowlist is what these tests turn.
type appWithSecret struct {
	database.OAuth2ProviderApp
	ClientSecret string
	SecretID     uuid.UUID
}

func seedAppWithSecret(t *testing.T, db database.Store, allowlist sql.NullString) appWithSecret {
	t.Helper()

	app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
		Name:        testutil.GetRandomName(t),
		CallbackURL: appCallbackURL,
		Scope:       allowlist,
	})

	secret, err := oauth2provider.GenerateSecret()
	require.NoError(t, err)
	dbSecret := dbgen.OAuth2ProviderAppSecret(t, db, database.OAuth2ProviderAppSecret{
		AppID:        app.ID,
		SecretPrefix: []byte(secret.Prefix),
		HashedSecret: secret.Hashed,
	})

	return appWithSecret{
		OAuth2ProviderApp: app,
		ClientSecret:      secret.Formatted,
		SecretID:          dbSecret.ID,
	}
}

// setAppAllowlist rewrites an app's registered scopes, leaving every other
// column as seeded.
func setAppAllowlist(ctx context.Context, t *testing.T, db database.Store, app appWithSecret, allowlist sql.NullString) {
	t.Helper()

	_, err := db.UpdateOAuth2ProviderAppByID(dbauthz.AsSystemRestricted(ctx), database.UpdateOAuth2ProviderAppByIDParams{
		ID:                      app.ID,
		UpdatedAt:               dbtime.Now(),
		Name:                    app.Name,
		Icon:                    app.Icon,
		CallbackURL:             app.CallbackURL,
		RedirectUris:            app.RedirectUris,
		ClientType:              app.ClientType,
		DynamicallyRegistered:   app.DynamicallyRegistered,
		ClientSecretExpiresAt:   app.ClientSecretExpiresAt,
		GrantTypes:              app.GrantTypes,
		ResponseTypes:           app.ResponseTypes,
		TokenEndpointAuthMethod: app.TokenEndpointAuthMethod,
		Scope:                   allowlist,
		Contacts:                app.Contacts,
		ClientUri:               app.ClientUri,
		LogoUri:                 app.LogoUri,
		TosUri:                  app.TosUri,
		PolicyUri:               app.PolicyUri,
		JwksUri:                 app.JwksUri,
		Jwks:                    app.Jwks,
		SoftwareID:              app.SoftwareID,
		SoftwareVersion:         app.SoftwareVersion,
	})
	require.NoError(t, err)
}

// Returns the secret that redeems the row. dbgen is unusable here: it derives
// expires_at from created_at, leaving the row already expired.
func seedRefreshToken(ctx context.Context, t *testing.T, db database.Store, app appWithSecret, userID uuid.UUID, scope string) string {
	t.Helper()

	key, _ := dbgen.APIKey(t, db, database.APIKey{
		UserID:    userID,
		LoginType: database.LoginTypeOAuth2ProviderApp,
	})

	secret, err := oauth2provider.GenerateSecret()
	require.NoError(t, err)

	_, err = db.InsertOAuth2ProviderAppToken(dbauthz.AsSystemRestricted(ctx), database.InsertOAuth2ProviderAppTokenParams{
		ID:          uuid.New(),
		CreatedAt:   dbtime.Now(),
		ExpiresAt:   dbtime.Now().Add(time.Hour),
		HashPrefix:  []byte(secret.Prefix),
		RefreshHash: secret.Hashed,
		AppID:       app.ID,
		AppSecretID: uuid.NullUUID{UUID: app.SecretID, Valid: true},
		APIKeyID:    key.ID,
		UserID:      userID,
		Scope:       scope,
	})
	require.NoError(t, err)
	return secret.Formatted
}

// Returns the issued code with the verifier that redeems it. authorizeQuery
// discards its own verifier, so the challenge is swapped for one kept here.
func authorizeCode(ctx context.Context, t *testing.T, client *codersdk.Client, clientID, scope string) (code, verifier string) {
	t.Helper()

	verifier, challenge := oauth2providertest.GeneratePKCE(t)
	query := authorizeQuery(t, clientID, scope)
	query.Set("code_challenge", challenge)

	resp := sendAuthorizeRequest(ctx, t, client, http.MethodPost, query)
	defer resp.Body.Close()

	require.Equal(t, http.StatusFound, resp.StatusCode)
	location, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	code = location.Query().Get("code")
	require.NotEmpty(t, code, "authorization did not issue a code")
	return code, verifier
}

// Writes a code the authorize endpoint would refuse to write. dbgen is unusable
// for the same reason as in seedRefreshToken.
func seedCode(ctx context.Context, t *testing.T, db database.Store, appID, userID uuid.UUID, challenge, scope string) string {
	t.Helper()

	secret, err := oauth2provider.GenerateSecret()
	require.NoError(t, err)

	_, err = db.InsertOAuth2ProviderAppCode(dbauthz.AsSystemRestricted(ctx), database.InsertOAuth2ProviderAppCodeParams{
		ID:                  uuid.New(),
		CreatedAt:           dbtime.Now(),
		ExpiresAt:           dbtime.Now().Add(time.Hour),
		SecretPrefix:        []byte(secret.Prefix),
		HashedSecret:        secret.Hashed,
		AppID:               appID,
		UserID:              userID,
		CodeChallenge:       sql.NullString{String: challenge, Valid: true},
		CodeChallengeMethod: sql.NullString{String: "S256", Valid: true},
		Scope:               scope,
	})
	require.NoError(t, err)
	return secret.Formatted
}

// requireNoSessionKey asserts the exchange minted nothing. The scope checks run
// before the transaction that deletes the code and inserts the key, so a
// rejection that left a row behind would still answer 400.
func requireNoSessionKey(ctx context.Context, t *testing.T, db database.Store, userID, appID uuid.UUID) {
	t.Helper()

	_, err := db.GetAPIKeyByName(dbauthz.AsSystemRestricted(ctx), database.GetAPIKeyByNameParams{
		UserID:    userID,
		TokenName: fmt.Sprintf("%s_%s_oauth_session_token", userID, appID),
	})
	require.ErrorIs(t, err, sql.ErrNoRows, "a refused redemption must mint no key")
}

// requireCodeConsumed asserts a redeemed code cannot be redeemed again.
func requireCodeConsumed(ctx context.Context, t *testing.T, db database.Store, code string) {
	t.Helper()

	parsed, err := oauth2provider.ParseFormattedSecret(code)
	require.NoError(t, err)
	_, err = db.GetOAuth2ProviderAppCodeByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(parsed.Prefix))
	require.ErrorIs(t, err, sql.ErrNoRows, "a redeemed code must not survive the exchange")
}

func tokenExchangeForm(app appWithSecret, code, verifier string) url.Values {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", app.ID.String())
	form.Set("client_secret", app.ClientSecret)
	form.Set("code_verifier", verifier)
	return form
}

func refreshForm(app appWithSecret, refreshToken string) url.Values {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", app.ID.String())
	form.Set("client_secret", app.ClientSecret)
	return form
}

func exchangeCode(ctx context.Context, t *testing.T, client *codersdk.Client, app appWithSecret, code, verifier string) codersdk.OAuth2TokenResponse {
	t.Helper()

	status, body := postTokenRequest(ctx, t, client, tokenExchangeForm(app, code, verifier))
	return requireTokenResponse(t, status, body)
}

func postTokenRequest(ctx context.Context, t *testing.T, client *codersdk.Client, form url.Values) (int, string) {
	t.Helper()

	status, body, err := tryTokenRequest(ctx, t, client, form)
	require.NoError(t, err)
	return status, body
}

// tryTokenRequest returns the request error instead of asserting on it, so a
// caller on a spawned goroutine can carry it back to the test goroutine.
// require there runs runtime.Goexit, which skips whatever the goroutine still
// owed its parent.
func tryTokenRequest(ctx context.Context, t *testing.T, client *codersdk.Client, form url.Values) (int, string, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.URL.String()+"/oauth2/tokens", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, "", xerrors.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", xerrors.Errorf("post token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", xerrors.Errorf("read token response: %w", err)
	}
	return resp.StatusCode, string(body), nil
}

func requireTokenResponse(t *testing.T, status int, body string) codersdk.OAuth2TokenResponse {
	t.Helper()

	require.Equal(t, http.StatusOK, status, body)
	var token codersdk.OAuth2TokenResponse
	require.NoError(t, json.Unmarshal([]byte(body), &token))
	require.NotEmpty(t, token.AccessToken)
	require.NotEmpty(t, token.RefreshToken)
	return token
}

// requireTokenError asserts an RFC 6749 §5.2 error response carrying want and
// returns its description.
func requireTokenError(t *testing.T, status int, body string, want codersdk.OAuth2ErrorCode) string {
	t.Helper()

	require.Equal(t, http.StatusBadRequest, status, body)
	var oauthErr codersdk.OAuth2Error
	require.NoError(t, json.Unmarshal([]byte(body), &oauthErr))
	require.Equal(t, want, oauthErr.Error)
	return oauthErr.ErrorDescription
}

func requireTokenGrantError(t *testing.T, status int, body string) string {
	t.Helper()
	return requireTokenError(t, status, body, codersdk.OAuth2ErrorCodeInvalidGrant)
}

func requireTokenScopeError(t *testing.T, status int, body string) string {
	t.Helper()
	return requireTokenError(t, status, body, codersdk.OAuth2ErrorCodeInvalidScope)
}

// requireNQSCHAR asserts the set RFC 6749 Appendix A permits in
// error_description, on the decoded value.
func requireNQSCHAR(t *testing.T, description string) {
	t.Helper()

	for _, r := range description {
		require.True(t, r == 0x20 || r == 0x21 || (r >= 0x23 && r <= 0x5B) || (r >= 0x5D && r <= 0x7E),
			"%q is outside the NQSCHAR set RFC 6749 Appendix A permits", r)
	}
}

func tokenRow(ctx context.Context, t *testing.T, db database.Store, refreshToken string) database.OAuth2ProviderAppToken {
	t.Helper()

	parsed, err := oauth2provider.ParseFormattedSecret(refreshToken)
	require.NoError(t, err)

	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(parsed.Prefix))
	require.NoError(t, err)
	return dbToken
}

func mintedKeyScopes(ctx context.Context, t *testing.T, db database.Store, refreshToken string) database.APIKeyScopes {
	t.Helper()

	key, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), tokenRow(ctx, t, db, refreshToken).APIKeyID)
	require.NoError(t, err)
	return key.Scopes
}
