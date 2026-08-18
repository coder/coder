package oauth2provider_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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

// The negotiation the authorize endpoint performs only bounds anything if the
// key minted from the code carries it. Every case here asserts against the
// api_keys row rather than the token response, because that row is what
// dbauthz reads on each later request; the response says nothing about the
// authority the client just received.
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

	// The refresh row has carried the scope since authorization; only the key
	// minted from it did not. Without that carried through, a narrowly scoped
	// token silently widened to coder:all the first time the client refreshed,
	// which is the longer-lived half of the grant.
	t.Run("RefreshKeepsTheGrant", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", token.RefreshToken)
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", app.ClientSecret)
		status, body := postTokenRequest(ctx, t, client, form)
		refreshed := requireTokenResponse(t, status, body)
		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken))
		require.Equal(t, "workspace:ssh", refreshed.Scope)
	})

	// An app with no allowlist negotiates the unrestricted sentinel, and the
	// key it mints has to say so by name. apikey.Generate defaults an empty
	// scope list to coder:all, so this case would pass even if the exchange
	// dropped the scope entirely; it is here to pin that the unrestricted path
	// keeps working, not to prove the scope was applied.
	t.Run("UnrestrictedGrantMintsCoderAll", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "")
		token := exchangeCode(ctx, t, client, app, code, verifier)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			mintedKeyScopes(ctx, t, db, token.RefreshToken))
	})

	// The scopes on the key only mean something once the authorizer reads them,
	// so this drives the issued access token against the real API: one action
	// the negotiated scope covers, one it does not. coder:workspaces.access
	// carries template:read but not template:delete, and the user behind the
	// grant owns the deployment, so the role permits both calls and the scope
	// is the only thing standing between the client and the deletion.
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

		// RFC 6749 §5.1 requires the response to state the granted scope when it
		// differs from the request. This client asked for nothing and was granted
		// the app's allowlist, so the response is the only place it learns the
		// bounds the calls below are about to hit.
		require.Equal(t, scopeInCatalog, token.Scope)

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

	// A grant that predates the scope columns carries what migration 000569
	// backfilled onto it: coder:all, which records an unrestricted grant rather
	// than an absent one. Refreshing one has to keep working and has to keep
	// meaning unrestricted, so the row is seeded the way the migration leaves
	// it instead of being written by an exchange this server just ran.
	t.Run("BackfilledScopeRefreshesUnrestricted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: owner.OrganizationID,
			CreatedBy:      owner.UserID,
		})

		app := seedAppWithSecret(t, db, sql.NullString{})
		refreshToken := seedRefreshToken(ctx, t, db, app, owner.UserID, string(database.ApiKeyScopeCoderAll))

		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refreshToken)
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", app.ClientSecret)
		status, body := postTokenRequest(ctx, t, client, form)
		refreshed := requireTokenResponse(t, status, body)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken))

		asApp := codersdk.New(client.URL)
		asApp.SetSessionToken(refreshed.AccessToken)
		require.NoError(t, asApp.DeleteTemplate(ctx, tpl.ID),
			"an unrestricted grant must still reach what it reached before")
	})

	// A scope the api_key_scope enum does not define cannot become a key. The
	// authorize endpoint cannot produce such a row, so the code is seeded
	// directly: the case covers a row written before the name was removed, or
	// by a different version of this server. The request itself is well-formed,
	// so it must not surface as a 500.
	t.Run("StoredScopeOutsideEnumRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		code := seedCode(ctx, t, db, app.ID, owner.UserID, challenge, scopeOutOfCatalog)

		status, body := postTokenRequest(ctx, t, client, tokenExchangeForm(app, code, verifier))

		require.Equal(t, http.StatusBadRequest, status, body)
		var oauthErr struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &oauthErr))
		require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), oauthErr.Error)
		require.Contains(t, oauthErr.ErrorDescription, scopeOutOfCatalog,
			"an operator cannot act on this without knowing which stored name is the problem")
	})
}

// appWithSecret is an app seeded straight into the database together with a
// client secret usable at the token endpoint. The management API registers no
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

// seedRefreshToken writes a refresh token row for an existing grant and returns
// the secret that redeems it, so a refresh can be exercised without the
// exchange that would otherwise have written the row. dbgen is not used because
// it derives expires_at from created_at, which would leave the row expired and
// fail the refresh before it reaches the scope.
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

// authorizeCode runs a full authorization and returns the issued code with the
// verifier that redeems it. The query is authorizeQuery's, with the challenge
// swapped for one whose verifier is kept, since the exchange needs it and
// authorizeQuery discards its own.
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

// seedCode writes an authorization code the authorize endpoint would refuse to
// write. dbgen is not used because it derives expires_at from created_at,
// which would leave the code already expired and fail the exchange before it
// reaches the scope it is here to exercise.
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

func tokenExchangeForm(app appWithSecret, code, verifier string) url.Values {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", app.ID.String())
	form.Set("client_secret", app.ClientSecret)
	form.Set("code_verifier", verifier)
	return form
}

func exchangeCode(ctx context.Context, t *testing.T, client *codersdk.Client, app appWithSecret, code, verifier string) codersdk.OAuth2TokenResponse {
	t.Helper()

	status, body := postTokenRequest(ctx, t, client, tokenExchangeForm(app, code, verifier))
	return requireTokenResponse(t, status, body)
}

// postTokenRequest posts to the token endpoint and returns the status and body
// it answered with, so both the granted and rejected cases read the same way.
func postTokenRequest(ctx context.Context, t *testing.T, client *codersdk.Client, form url.Values) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.URL.String()+"/oauth2/tokens", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	return resp.StatusCode, readBody(t, resp)
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

// mintedKeyScopes follows a refresh token to the API key issued alongside it
// and returns the scopes recorded on that key.
func mintedKeyScopes(ctx context.Context, t *testing.T, db database.Store, refreshToken string) database.APIKeyScopes {
	t.Helper()

	parsed, err := oauth2provider.ParseFormattedSecret(refreshToken)
	require.NoError(t, err)

	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(parsed.Prefix))
	require.NoError(t, err)
	key, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), dbToken.APIKeyID)
	require.NoError(t, err)
	return key.Scopes
}
