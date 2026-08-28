package oauth2provider_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

	t.Run("RefreshDoesNotWidenTheScope", func(t *testing.T) {
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

	// Authorization cannot write such a row, so it is seeded: the case covers a
	// name removed since, or a row written by another version of this server.
	t.Run("StoredScopeOutsideEnumRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		code := seedCode(ctx, t, db, app.ID, owner.UserID, challenge, scopeOutOfCatalog)

		status, body := postTokenRequest(ctx, t, client, tokenExchangeForm(app, code, verifier))

		description := requireTokenScopeError(t, status, body)
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

		description := requireTokenScopeError(t, status, body)
		require.Contains(t, description, oauth2provider.ReasonStaleScope)
		require.Contains(t, description, "workspace:ssh",
			"the client needs the scope name to know what was withdrawn")
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
	})

	// RFC 6749 §6 bounds a refresh by the scope originally granted, not by the
	// live allowlist.
	t.Run("RefreshIgnoresAllowlistNarrowing", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedAppWithSecret(t, db, sql.NullString{String: scopeInCatalog, Valid: true})
		code, verifier := authorizeCode(ctx, t, client, app.ID.String(), "workspace:ssh")
		token := exchangeCode(ctx, t, client, app, code, verifier)
		setAppAllowlist(ctx, t, db, app, sql.NullString{String: scopeAlsoInCatalog, Valid: true})

		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", token.RefreshToken)
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", app.ClientSecret)
		status, body := postTokenRequest(ctx, t, client, form)
		refreshed := requireTokenResponse(t, status, body)

		require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeWorkspaceSsh},
			mintedKeyScopes(ctx, t, db, refreshed.RefreshToken))
	})
}

// The redemptions race rather than run in sequence: a sequential pair passes
// whether or not the delete arbitrates single use.
func TestOAuth2TokenExchangeSingleUse(t *testing.T) {
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
	form := tokenExchangeForm(app, code, verifier)

	type exchange struct {
		status int
		body   string
	}

	var barrier sync.WaitGroup
	barrier.Add(2)
	redeem := func() exchange {
		barrier.Done()
		barrier.Wait()
		status, body := postTokenRequest(ctx, t, client, form)
		return exchange{status: status, body: body}
	}

	other := make(chan exchange, 1)
	go func() { other <- redeem() }()
	results := []exchange{redeem(), <-other}

	var minted, rejected int
	for _, result := range results {
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
	require.Equal(t, 1, minted, "a code may mint at most one token")
	require.Equal(t, 1, rejected)
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

// requireTokenScopeError asserts an RFC 6749 §5.2 invalid_scope response and
// returns its description.
func requireTokenScopeError(t *testing.T, status int, body string) string {
	t.Helper()

	require.Equal(t, http.StatusBadRequest, status, body)
	var oauthErr struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &oauthErr))
	require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), oauthErr.Error)
	return oauthErr.ErrorDescription
}

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
