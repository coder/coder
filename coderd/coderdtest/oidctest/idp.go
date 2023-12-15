package oidctest

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/go-jose/go-jose/v3"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/util/syncmap"
	"github.com/coder/coder/v2/codersdk"
)

// FakeIDP is a functional OIDC provider.
// It only supports 1 OIDC client.
type FakeIDP struct {
	issuer    string
	issuerURL *url.URL
	key       *rsa.PrivateKey
	provider  ProviderJSON
	handler   http.Handler
	cfg       *oauth2.Config

	// clientID to be used by coderd
	clientID     string
	clientSecret string
	// externalProviderID is optional to match the provider in coderd for
	// redirectURLs.
	externalProviderID string
	logger             slog.Logger
	// externalAuthValidate will be called when the user tries to validate their
	// external auth. The fake IDP will reject any invalid tokens, so this just
	// controls the response payload after a successfully authed token.
	externalAuthValidate func(email string, rw http.ResponseWriter, r *http.Request)

	// These maps are used to control the state of the IDP.
	// That is the various access tokens, refresh tokens, states, etc.
	codeToStateMap *syncmap.Map[string, string]
	// Token -> Email
	accessTokens *syncmap.Map[string, string]
	// Refresh Token -> Email
	refreshTokensUsed    *syncmap.Map[string, bool]
	refreshTokens        *syncmap.Map[string, string]
	stateToIDTokenClaims *syncmap.Map[string, jwt.MapClaims]
	refreshIDTokenClaims *syncmap.Map[string, jwt.MapClaims]

	// hooks
	// hookValidRedirectURL can be used to reject a redirect url from the
	// IDP -> Application. Almost all IDPs have the concept of
	// "Authorized Redirect URLs". This can be used to emulate that.
	hookValidRedirectURL func(redirectURL string) error
	hookUserInfo         func(email string) (jwt.MapClaims, error)
	hookMutateToken      func(token map[string]interface{})
	fakeCoderd           func(req *http.Request) (*http.Response, error)
	hookOnRefresh        func(email string) error
	// Custom authentication for the client. This is useful if you want
	// to test something like PKI auth vs a client_secret.
	hookAuthenticateClient func(t testing.TB, req *http.Request) (url.Values, error)
	serve                  bool
}

func StatusError(code int, err error) error {
	return statusHookError{
		Err:            err,
		HTTPStatusCode: code,
	}
}

// statusHookError allows a hook to change the returned http status code.
type statusHookError struct {
	Err            error
	HTTPStatusCode int
}

func (s statusHookError) Error() string {
	if s.Err == nil {
		return ""
	}
	return s.Err.Error()
}

type FakeIDPOpt func(idp *FakeIDP)

func WithAuthorizedRedirectURL(hook func(redirectURL string) error) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookValidRedirectURL = hook
	}
}

// WithRefresh is called when a refresh token is used. The email is
// the email of the user that is being refreshed assuming the claims are correct.
func WithRefresh(hook func(email string) error) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookOnRefresh = hook
	}
}

// WithExtra returns extra fields that be accessed on the returned Oauth Token.
// These extra fields can override the default fields (id_token, access_token, etc).
func WithMutateToken(mutateToken func(token map[string]interface{})) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookMutateToken = mutateToken
	}
}

func WithCustomClientAuth(hook func(t testing.TB, req *http.Request) (url.Values, error)) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookAuthenticateClient = hook
	}
}

// WithLogging is optional, but will log some HTTP calls made to the IDP.
func WithLogging(t testing.TB, options *slogtest.Options) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.logger = slogtest.Make(t, options)
	}
}

// WithStaticUserInfo is optional, but will return the same user info for
// every user on the /userinfo endpoint.
func WithStaticUserInfo(info jwt.MapClaims) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookUserInfo = func(_ string) (jwt.MapClaims, error) {
			return info, nil
		}
	}
}

func WithDynamicUserInfo(userInfoFunc func(email string) (jwt.MapClaims, error)) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookUserInfo = userInfoFunc
	}
}

// WithServing makes the IDP run an actual http server.
func WithServing() func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.serve = true
	}
}

func WithIssuer(issuer string) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.issuer = issuer
	}
}

const (
	// nolint:gosec // It thinks this is a secret lol
	tokenPath     = "/oauth2/token"
	authorizePath = "/oauth2/authorize"
	keysPath      = "/oauth2/keys"
	userInfoPath  = "/oauth2/userinfo"
)

func NewFakeIDP(t testing.TB, opts ...FakeIDPOpt) *FakeIDP {
	t.Helper()

	block, _ := pem.Decode([]byte(testRSAPrivateKey))
	pkey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)

	idp := &FakeIDP{
		key:                  pkey,
		clientID:             uuid.NewString(),
		clientSecret:         uuid.NewString(),
		logger:               slog.Make(),
		codeToStateMap:       syncmap.New[string, string](),
		accessTokens:         syncmap.New[string, string](),
		refreshTokens:        syncmap.New[string, string](),
		refreshTokensUsed:    syncmap.New[string, bool](),
		stateToIDTokenClaims: syncmap.New[string, jwt.MapClaims](),
		refreshIDTokenClaims: syncmap.New[string, jwt.MapClaims](),
		hookOnRefresh:        func(_ string) error { return nil },
		hookUserInfo:         func(email string) (jwt.MapClaims, error) { return jwt.MapClaims{}, nil },
		hookValidRedirectURL: func(redirectURL string) error { return nil },
	}

	for _, opt := range opts {
		opt(idp)
	}

	if idp.issuer == "" {
		idp.issuer = "https://coder.com"
	}

	idp.handler = idp.httpHandler(t)
	idp.updateIssuerURL(t, idp.issuer)
	if idp.serve {
		idp.realServer(t)
	}

	return idp
}

func (f *FakeIDP) WellknownConfig() ProviderJSON {
	return f.provider
}

func (f *FakeIDP) updateIssuerURL(t testing.TB, issuer string) {
	t.Helper()

	u, err := url.Parse(issuer)
	require.NoError(t, err, "invalid issuer URL")

	f.issuer = issuer
	f.issuerURL = u
	// ProviderJSON is the JSON representation of the OpenID Connect provider
	// These are all the urls that the IDP will respond to.
	f.provider = ProviderJSON{
		Issuer:      issuer,
		AuthURL:     u.ResolveReference(&url.URL{Path: authorizePath}).String(),
		TokenURL:    u.ResolveReference(&url.URL{Path: tokenPath}).String(),
		JWKSURL:     u.ResolveReference(&url.URL{Path: keysPath}).String(),
		UserInfoURL: u.ResolveReference(&url.URL{Path: userInfoPath}).String(),
		Algorithms: []string{
			"RS256",
		},
	}
}

// realServer turns the FakeIDP into a real http server.
func (f *FakeIDP) realServer(t testing.TB) *httptest.Server {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewUnstartedServer(f.handler)
	srv.Config.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}
	srv.Start()
	t.Cleanup(srv.CloseClientConnections)
	t.Cleanup(srv.Close)
	t.Cleanup(cancel)

	f.updateIssuerURL(t, srv.URL)
	return srv
}

// GenerateAuthenticatedToken skips all oauth2 flows, and just generates a
// valid token for some given claims.
func (f *FakeIDP) GenerateAuthenticatedToken(claims jwt.MapClaims) (*oauth2.Token, error) {
	state := uuid.NewString()
	f.stateToIDTokenClaims.Store(state, claims)
	code := f.newCode(state)
	return f.cfg.Exchange(oidc.ClientContext(context.Background(), f.HTTPClient(nil)), code)
}

// Login does the full OIDC flow starting at the "LoginButton".
// The client argument is just to get the URL of the Coder instance.
//
// The client passed in is just to get the url of the Coder instance.
// The actual client that is used is 100% unauthenticated and fresh.
func (f *FakeIDP) Login(t testing.TB, client *codersdk.Client, idTokenClaims jwt.MapClaims, opts ...func(r *http.Request)) (*codersdk.Client, *http.Response) {
	t.Helper()

	client, resp := f.AttemptLogin(t, client, idTokenClaims, opts...)
	require.Equal(t, http.StatusOK, resp.StatusCode, "client failed to login")
	return client, resp
}

func (f *FakeIDP) AttemptLogin(t testing.TB, client *codersdk.Client, idTokenClaims jwt.MapClaims, opts ...func(r *http.Request)) (*codersdk.Client, *http.Response) {
	t.Helper()
	var err error

	cli := f.HTTPClient(client.HTTPClient)
	shallowCpyCli := *cli

	if shallowCpyCli.Jar == nil {
		shallowCpyCli.Jar, err = cookiejar.New(nil)
		require.NoError(t, err, "failed to create cookie jar")
	}

	unauthenticated := codersdk.New(client.URL)
	unauthenticated.HTTPClient = &shallowCpyCli

	return f.LoginWithClient(t, unauthenticated, idTokenClaims, opts...)
}

// LoginWithClient reuses the context of the passed in client. This means the same
// cookies will be used. This should be an unauthenticated client in most cases.
//
// This is a niche case, but it is needed for testing ConvertLoginType.
func (f *FakeIDP) LoginWithClient(t testing.TB, client *codersdk.Client, idTokenClaims jwt.MapClaims, opts ...func(r *http.Request)) (*codersdk.Client, *http.Response) {
	t.Helper()

	coderOauthURL, err := client.URL.Parse("/api/v2/users/oidc/callback")
	require.NoError(t, err)
	f.SetRedirect(t, coderOauthURL.String())

	cli := f.HTTPClient(client.HTTPClient)
	cli.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// Store the idTokenClaims to the specific state request. This ties
		// the claims 1:1 with a given authentication flow.
		state := req.URL.Query().Get("state")
		f.stateToIDTokenClaims.Store(state, idTokenClaims)
		return nil
	}

	req, err := http.NewRequestWithContext(context.Background(), "GET", coderOauthURL.String(), nil)
	require.NoError(t, err)
	if cli.Jar == nil {
		cli.Jar, err = cookiejar.New(nil)
		require.NoError(t, err, "failed to create cookie jar")
	}

	for _, opt := range opts {
		opt(req)
	}

	res, err := cli.Do(req)
	require.NoError(t, err)

	// If the coder session token exists, return the new authed client!
	var user *codersdk.Client
	cookies := cli.Jar.Cookies(client.URL)
	for _, cookie := range cookies {
		if cookie.Name == codersdk.SessionTokenCookie {
			user = codersdk.New(client.URL)
			user.SetSessionToken(cookie.Value)
		}
	}

	t.Cleanup(func() {
		if res.Body != nil {
			_ = res.Body.Close()
		}
	})

	return user, res
}

// ExternalLogin does the oauth2 flow for external auth providers. This requires
// an authenticated coder client.
func (f *FakeIDP) ExternalLogin(t testing.TB, client *codersdk.Client, opts ...func(r *http.Request)) {
	coderOauthURL, err := client.URL.Parse(fmt.Sprintf("/external-auth/%s/callback", f.externalProviderID))
	require.NoError(t, err)
	f.SetRedirect(t, coderOauthURL.String())

	cli := f.HTTPClient(client.HTTPClient)
	cli.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// Store the idTokenClaims to the specific state request. This ties
		// the claims 1:1 with a given authentication flow.
		state := req.URL.Query().Get("state")
		f.stateToIDTokenClaims.Store(state, jwt.MapClaims{})
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, "GET", coderOauthURL.String(), nil)
	require.NoError(t, err)
	// External auth flow requires the user be authenticated.
	headerName := client.SessionTokenHeader
	if headerName == "" {
		headerName = codersdk.SessionTokenHeader
	}
	req.Header.Set(headerName, client.SessionToken())
	if cli.Jar == nil {
		cli.Jar, err = cookiejar.New(nil)
		require.NoError(t, err, "failed to create cookie jar")
	}

	for _, opt := range opts {
		opt(req)
	}

	res, err := cli.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode, "client failed to login")
	_ = res.Body.Close()
}

// OIDCCallback will emulate the IDP redirecting back to the Coder callback.
// This is helpful if no Coderd exists because the IDP needs to redirect to
// something.
// Essentially this is used to fake the Coderd side of the exchange.
// The flow starts at the user hitting the OIDC login page.
func (f *FakeIDP) OIDCCallback(t testing.TB, state string, idTokenClaims jwt.MapClaims) (*http.Response, error) {
	t.Helper()
	if f.serve {
		panic("cannot use OIDCCallback with WithServing. This is only for the in memory usage")
	}

	f.stateToIDTokenClaims.Store(state, idTokenClaims)

	cli := f.HTTPClient(nil)
	u := f.cfg.AuthCodeURL(state)
	req, err := http.NewRequest("GET", u, nil)
	require.NoError(t, err)

	resp, err := cli.Do(req.WithContext(context.Background()))
	require.NoError(t, err)

	t.Cleanup(func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	return resp, nil
}

// ProviderJSON is the .well-known/configuration JSON
type ProviderJSON struct {
	Issuer      string   `json:"issuer"`
	AuthURL     string   `json:"authorization_endpoint"`
	TokenURL    string   `json:"token_endpoint"`
	JWKSURL     string   `json:"jwks_uri"`
	UserInfoURL string   `json:"userinfo_endpoint"`
	Algorithms  []string `json:"id_token_signing_alg_values_supported"`
}

// newCode enforces the code exchanged is actually a valid code
// created by the IDP.
func (f *FakeIDP) newCode(state string) string {
	code := uuid.NewString()
	f.codeToStateMap.Store(code, state)
	return code
}

// newToken enforces the access token exchanged is actually a valid access token
// created by the IDP.
func (f *FakeIDP) newToken(email string) string {
	accessToken := uuid.NewString()
	f.accessTokens.Store(accessToken, email)
	return accessToken
}

func (f *FakeIDP) newRefreshTokens(email string) string {
	refreshToken := uuid.NewString()
	f.refreshTokens.Store(refreshToken, email)
	return refreshToken
}

// authenticateBearerTokenRequest enforces the access token is valid.
func (f *FakeIDP) authenticateBearerTokenRequest(t testing.TB, req *http.Request) (string, error) {
	t.Helper()

	auth := req.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	_, ok := f.accessTokens.Load(token)
	if !ok {
		return "", xerrors.New("invalid access token")
	}
	return token, nil
}

// authenticateOIDCClientRequest enforces the client_id and client_secret are valid.
func (f *FakeIDP) authenticateOIDCClientRequest(t testing.TB, req *http.Request) (url.Values, error) {
	t.Helper()

	if f.hookAuthenticateClient != nil {
		return f.hookAuthenticateClient(t, req)
	}

	data, err := io.ReadAll(req.Body)
	if !assert.NoError(t, err, "read token request body") {
		return nil, xerrors.Errorf("authenticate request, read body: %w", err)
	}
	values, err := url.ParseQuery(string(data))
	if !assert.NoError(t, err, "parse token request values") {
		return nil, xerrors.New("invalid token request")
	}

	if !assert.Equal(t, f.clientID, values.Get("client_id"), "client_id mismatch") {
		return nil, xerrors.New("client_id mismatch")
	}

	if !assert.Equal(t, f.clientSecret, values.Get("client_secret"), "client_secret mismatch") {
		return nil, xerrors.New("client_secret mismatch")
	}

	return values, nil
}

// encodeClaims is a helper func to convert claims to a valid JWT.
func (f *FakeIDP) encodeClaims(t testing.TB, claims jwt.MapClaims) string {
	t.Helper()

	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).UnixMilli()
	}

	if _, ok := claims["aud"]; !ok {
		claims["aud"] = f.clientID
	}

	if _, ok := claims["iss"]; !ok {
		claims["iss"] = f.issuer
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(f.key)
	require.NoError(t, err)

	return signed
}

// httpHandler is the IDP http server.
func (f *FakeIDP) httpHandler(t testing.TB) http.Handler {
	t.Helper()

	mux := chi.NewMux()
	// This endpoint is required to initialize the OIDC provider.
	// It is used to get the OIDC configuration.
	mux.Get("/.well-known/openid-configuration", func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "http OIDC config", slog.F("url", r.URL.String()))

		_ = json.NewEncoder(rw).Encode(f.provider)
	})

	// Authorize is called when the user is redirected to the IDP to login.
	// This is the browser hitting the IDP and the user logging into Google or
	// w/e and clicking "Allow". They will be redirected back to the redirect
	// when this is done.
	mux.Handle(authorizePath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "http call authorize", slog.F("url", r.URL.String()))

		clientID := r.URL.Query().Get("client_id")
		if !assert.Equal(t, f.clientID, clientID, "unexpected client_id") {
			http.Error(rw, "invalid client_id", http.StatusBadRequest)
			return
		}

		redirectURI := r.URL.Query().Get("redirect_uri")
		state := r.URL.Query().Get("state")

		scope := r.URL.Query().Get("scope")
		assert.NotEmpty(t, scope, "scope is empty")

		responseType := r.URL.Query().Get("response_type")
		switch responseType {
		case "code":
		case "token":
			t.Errorf("response_type %q not supported", responseType)
			http.Error(rw, "invalid response_type", http.StatusBadRequest)
			return
		default:
			t.Errorf("unexpected response_type %q", responseType)
			http.Error(rw, "invalid response_type", http.StatusBadRequest)
			return
		}

		err := f.hookValidRedirectURL(redirectURI)
		if err != nil {
			t.Errorf("not authorized redirect_uri by custom hook %q: %s", redirectURI, err.Error())
			http.Error(rw, fmt.Sprintf("invalid redirect_uri: %s", err.Error()), httpErrorCode(http.StatusBadRequest, err))
			return
		}

		ru, err := url.Parse(redirectURI)
		if err != nil {
			t.Errorf("invalid redirect_uri %q: %s", redirectURI, err.Error())
			http.Error(rw, fmt.Sprintf("invalid redirect_uri: %s", err.Error()), http.StatusBadRequest)
			return
		}

		q := ru.Query()
		q.Set("state", state)
		q.Set("code", f.newCode(state))
		ru.RawQuery = q.Encode()

		http.Redirect(rw, r, ru.String(), http.StatusTemporaryRedirect)
	}))

	mux.Handle(tokenPath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		values, err := f.authenticateOIDCClientRequest(t, r)
		f.logger.Info(r.Context(), "http idp call token",
			slog.Error(err),
			slog.F("values", values.Encode()),
		)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid token request: %s", err.Error()), httpErrorCode(http.StatusBadRequest, err))
			return
		}
		getEmail := func(claims jwt.MapClaims) string {
			email, ok := claims["email"]
			if !ok {
				return "unknown"
			}
			emailStr, ok := email.(string)
			if !ok {
				return "wrong-type"
			}
			return emailStr
		}

		var claims jwt.MapClaims
		switch values.Get("grant_type") {
		case "authorization_code":
			code := values.Get("code")
			if !assert.NotEmpty(t, code, "code is empty") {
				http.Error(rw, "invalid code", http.StatusBadRequest)
				return
			}
			stateStr, ok := f.codeToStateMap.Load(code)
			if !assert.True(t, ok, "invalid code") {
				http.Error(rw, "invalid code", http.StatusBadRequest)
				return
			}
			// Always invalidate the code after it is used.
			f.codeToStateMap.Delete(code)

			idTokenClaims, ok := f.stateToIDTokenClaims.Load(stateStr)
			if !ok {
				t.Errorf("missing id token claims")
				http.Error(rw, "missing id token claims", http.StatusBadRequest)
				return
			}
			claims = idTokenClaims
		case "refresh_token":
			refreshToken := values.Get("refresh_token")
			if !assert.NotEmpty(t, refreshToken, "refresh_token is empty") {
				http.Error(rw, "invalid refresh_token", http.StatusBadRequest)
				return
			}

			_, ok := f.refreshTokens.Load(refreshToken)
			if !assert.True(t, ok, "invalid refresh_token") {
				http.Error(rw, "invalid refresh_token", http.StatusBadRequest)
				return
			}

			idTokenClaims, ok := f.refreshIDTokenClaims.Load(refreshToken)
			if !ok {
				t.Errorf("missing id token claims in refresh")
				http.Error(rw, "missing id token claims in refresh", http.StatusBadRequest)
				return
			}

			claims = idTokenClaims
			err := f.hookOnRefresh(getEmail(claims))
			if err != nil {
				http.Error(rw, fmt.Sprintf("refresh hook blocked refresh: %s", err.Error()), httpErrorCode(http.StatusBadRequest, err))
				return
			}

			f.refreshTokensUsed.Store(refreshToken, true)
			// Always invalidate the refresh token after it is used.
			f.refreshTokens.Delete(refreshToken)
		default:
			t.Errorf("unexpected grant_type %q", values.Get("grant_type"))
			http.Error(rw, "invalid grant_type", http.StatusBadRequest)
			return
		}

		exp := time.Now().Add(time.Minute * 5)
		claims["exp"] = exp.UnixMilli()
		email := getEmail(claims)
		refreshToken := f.newRefreshTokens(email)
		token := map[string]interface{}{
			"access_token":  f.newToken(email),
			"refresh_token": refreshToken,
			"token_type":    "Bearer",
			"expires_in":    int64((time.Minute * 5).Seconds()),
			"id_token":      f.encodeClaims(t, claims),
		}
		if f.hookMutateToken != nil {
			f.hookMutateToken(token)
		}
		// Store the claims for the next refresh
		f.refreshIDTokenClaims.Store(refreshToken, claims)

		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(token)
	}))

	validateMW := func(rw http.ResponseWriter, r *http.Request) (email string, ok bool) {
		token, err := f.authenticateBearerTokenRequest(t, r)
		f.logger.Info(r.Context(), "http call idp user info",
			slog.Error(err),
			slog.F("url", r.URL.String()),
		)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid user info request: %s", err.Error()), http.StatusBadRequest)
			return "", false
		}

		email, ok = f.accessTokens.Load(token)
		if !ok {
			t.Errorf("access token user for user_info has no email to indicate which user")
			http.Error(rw, "invalid access token, missing user info", http.StatusBadRequest)
			return "", false
		}
		return email, true
	}
	mux.Handle(userInfoPath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		email, ok := validateMW(rw, r)
		if !ok {
			return
		}

		claims, err := f.hookUserInfo(email)
		if err != nil {
			http.Error(rw, fmt.Sprintf("user info hook returned error: %s", err.Error()), httpErrorCode(http.StatusBadRequest, err))
			return
		}
		_ = json.NewEncoder(rw).Encode(claims)
	}))

	// There is almost no difference between this and /userinfo.
	// The main tweak is that this route is "mounted" vs "handle" because "/userinfo"
	// should be strict, and this one needs to handle sub routes.
	mux.Mount("/external-auth-validate/", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		email, ok := validateMW(rw, r)
		if !ok {
			return
		}

		if f.externalAuthValidate == nil {
			t.Errorf("missing external auth validate handler")
			http.Error(rw, "missing external auth validate handler", http.StatusBadRequest)
			return
		}

		f.externalAuthValidate(email, rw, r)
	}))

	mux.Handle(keysPath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "http call idp /keys")
		set := jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{
				{
					Key:       f.key.Public(),
					KeyID:     "test-key",
					Algorithm: "RSA",
				},
			},
		}
		_ = json.NewEncoder(rw).Encode(set)
	}))

	mux.NotFound(func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Error(r.Context(), "http call not found", slog.F("path", r.URL.Path))
		t.Errorf("unexpected request to IDP at path %q. Not supported", r.URL.Path)
	})

	return mux
}

// HTTPClient does nothing if IsServing is used.
//
// If IsServing is not used, then it will return a client that will make requests
// to the IDP all in memory. If a request is not to the IDP, then the passed in
// client will be used. If no client is passed in, then any regular network
// requests will fail.
func (f *FakeIDP) HTTPClient(rest *http.Client) *http.Client {
	if f.serve {
		if rest == nil || rest.Transport == nil {
			return &http.Client{}
		}
		return rest
	}

	var jar http.CookieJar
	if rest != nil {
		jar = rest.Jar
	}
	return &http.Client{
		Jar: jar,
		Transport: fakeRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				u, _ := url.Parse(f.issuer)
				if req.URL.Host != u.Host {
					if f.fakeCoderd != nil {
						return f.fakeCoderd(req)
					}
					if rest == nil || rest.Transport == nil {
						return nil, xerrors.Errorf("unexpected network request to %q", req.URL.Host)
					}
					return rest.Transport.RoundTrip(req)
				}
				resp := httptest.NewRecorder()
				f.handler.ServeHTTP(resp, req)
				return resp.Result(), nil
			},
		},
	}
}

// RefreshUsed returns if the refresh token has been used. All refresh tokens
// can only be used once, then they are deleted.
func (f *FakeIDP) RefreshUsed(refreshToken string) bool {
	used, _ := f.refreshTokensUsed.Load(refreshToken)
	return used
}

// UpdateRefreshClaims allows the caller to change what claims are returned
// for a given refresh token. By default, all refreshes use the same claims as
// the original IDToken issuance.
func (f *FakeIDP) UpdateRefreshClaims(refreshToken string, claims jwt.MapClaims) {
	f.refreshIDTokenClaims.Store(refreshToken, claims)
}

// SetRedirect is required for the IDP to know where to redirect and call
// Coderd.
func (f *FakeIDP) SetRedirect(t testing.TB, u string) {
	t.Helper()

	f.cfg.RedirectURL = u
}

// SetCoderdCallback is optional and only works if not using the IsServing.
// It will setup a fake "Coderd" for the IDP to call when the IDP redirects
// back after authenticating.
func (f *FakeIDP) SetCoderdCallback(callback func(req *http.Request) (*http.Response, error)) {
	if f.serve {
		panic("cannot set callback handler when using 'WithServing'. Must implement an actual 'Coderd'")
	}
	f.fakeCoderd = callback
}

func (f *FakeIDP) SetCoderdCallbackHandler(handler http.HandlerFunc) {
	f.SetCoderdCallback(func(req *http.Request) (*http.Response, error) {
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		return resp.Result(), nil
	})
}

// ExternalAuthConfigOptions exists to provide additional functionality ontop
// of the standard "validate" url. Some providers like github we actually parse
// the response from the validate URL to gain additional information.
type ExternalAuthConfigOptions struct {
	// ValidatePayload is the payload that is used when the user calls the
	// equivalent of "userinfo" for oauth2. This is not standardized, so is
	// different for each provider type.
	ValidatePayload func(email string) interface{}

	// routes is more advanced usage. This allows the caller to
	// completely customize the response. It captures all routes under the /external-auth-validate/*
	// so the caller can do whatever they want and even add routes.
	routes map[string]func(email string, rw http.ResponseWriter, r *http.Request)
}

func (o *ExternalAuthConfigOptions) AddRoute(route string, handle func(email string, rw http.ResponseWriter, r *http.Request)) *ExternalAuthConfigOptions {
	if route == "/" || route == "" || route == "/user" {
		panic("cannot override the /user route. Use ValidatePayload instead")
	}
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	if o.routes == nil {
		o.routes = make(map[string]func(email string, rw http.ResponseWriter, r *http.Request))
	}
	o.routes[route] = handle
	return o
}

// ExternalAuthConfig is the config for external auth providers.
func (f *FakeIDP) ExternalAuthConfig(t testing.TB, id string, custom *ExternalAuthConfigOptions, opts ...func(cfg *externalauth.Config)) *externalauth.Config {
	if custom == nil {
		custom = &ExternalAuthConfigOptions{}
	}
	f.externalProviderID = id
	f.externalAuthValidate = func(email string, rw http.ResponseWriter, r *http.Request) {
		newPath := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/external-auth-validate/%s", id))
		switch newPath {
		// /user is ALWAYS supported under the `/` path too.
		case "/user", "/", "":
			var payload interface{} = "OK"
			if custom.ValidatePayload != nil {
				payload = custom.ValidatePayload(email)
			}
			_ = json.NewEncoder(rw).Encode(payload)
		default:
			if custom.routes == nil {
				custom.routes = make(map[string]func(email string, rw http.ResponseWriter, r *http.Request))
			}
			handle, ok := custom.routes[newPath]
			if !ok {
				t.Errorf("missing route handler for %s", newPath)
				http.Error(rw, fmt.Sprintf("missing route handler for %s", newPath), http.StatusBadRequest)
				return
			}
			handle(email, rw, r)
		}
	}
	cfg := &externalauth.Config{
		OAuth2Config: f.OIDCConfig(t, nil),
		ID:           id,
		// No defaults for these fields by omitting the type
		Type:        "",
		DisplayIcon: f.WellknownConfig().UserInfoURL,
		// Omit the /user for the validate so we can easily append to it when modifying
		// the cfg for advanced tests.
		ValidateURL: f.issuerURL.ResolveReference(&url.URL{Path: fmt.Sprintf("/external-auth-validate/%s", id)}).String(),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// OIDCConfig returns the OIDC config to use for Coderd.
func (f *FakeIDP) OIDCConfig(t testing.TB, scopes []string, opts ...func(cfg *coderd.OIDCConfig)) *coderd.OIDCConfig {
	t.Helper()
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}

	oauthCfg := &oauth2.Config{
		ClientID:     f.clientID,
		ClientSecret: f.clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   f.provider.AuthURL,
			TokenURL:  f.provider.TokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		// If the user is using a real network request, they will need to do
		// 'fake.SetRedirect()'
		RedirectURL: "https://redirect.com",
		Scopes:      scopes,
	}

	ctx := oidc.ClientContext(context.Background(), f.HTTPClient(nil))
	p, err := oidc.NewProvider(ctx, f.provider.Issuer)
	require.NoError(t, err, "failed to create OIDC provider")
	cfg := &coderd.OIDCConfig{
		OAuth2Config: oauthCfg,
		Provider:     p,
		Verifier: oidc.NewVerifier(f.provider.Issuer, &oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{f.key.Public()},
		}, &oidc.Config{
			ClientID: oauthCfg.ClientID,
			SupportedSigningAlgs: []string{
				"RS256",
			},
			// Todo: add support for Now()
		}),
		UsernameField: "preferred_username",
		EmailField:    "email",
		AuthURLParams: map[string]string{"access_type": "offline"},
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(cfg)
	}

	f.cfg = oauthCfg

	return cfg
}

func httpErrorCode(defaultCode int, err error) int {
	var stautsErr statusHookError
	status := defaultCode
	if errors.As(err, &stautsErr) {
		status = stautsErr.HTTPStatusCode
	}
	return status
}

type fakeRoundTripper struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (f fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.roundTrip(req)
}

//nolint:gosec // these are test credentials
const testRSAPrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXQIBAAKBgQDLets8+7M+iAQAqN/5BVyCIjhTQ4cmXulL+gm3v0oGMWzLupUS
v8KPA+Tp7dgC/DZPfMLaNH1obBBhJ9DhS6RdS3AS3kzeFrdu8zFHLWF53DUBhS92
5dCAEuJpDnNizdEhxTfoHrhuCmz8l2nt1pe5eUK2XWgd08Uc93h5ij098wIDAQAB
AoGAHLaZeWGLSaen6O/rqxg2laZ+jEFbMO7zvOTruiIkL/uJfrY1kw+8RLIn+1q0
wLcWcuEIHgKKL9IP/aXAtAoYh1FBvRPLkovF1NZB0Je/+CSGka6wvc3TGdvppZJe
rKNcUvuOYLxkmLy4g9zuY5qrxFyhtIn2qZzXEtLaVOHzPQECQQDvN0mSajpU7dTB
w4jwx7IRXGSSx65c+AsHSc1Rj++9qtPC6WsFgAfFN2CEmqhMbEUVGPv/aPjdyWk9
pyLE9xR/AkEA2cGwyIunijE5v2rlZAD7C4vRgdcMyCf3uuPcgzFtsR6ZhyQSgLZ8
YRPuvwm4cdPJMmO3YwBfxT6XGuSc2k8MjQJBAI0+b8prvpV2+DCQa8L/pjxp+VhR
Xrq2GozrHrgR7NRokTB88hwFRJFF6U9iogy9wOx8HA7qxEbwLZuhm/4AhbECQC2a
d8h4Ht09E+f3nhTEc87mODkl7WJZpHL6V2sORfeq/eIkds+H6CJ4hy5w/bSw8tjf
sz9Di8sGIaUbLZI2rd0CQQCzlVwEtRtoNCyMJTTrkgUuNufLP19RZ5FpyXxBO5/u
QastnN77KfUwdj3SJt44U/uh1jAIv4oSLBr8HYUkbnI8
-----END RSA PRIVATE KEY-----`
