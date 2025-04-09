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
	"math/rand"
	"mime"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/util/syncmap"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type token struct {
	issued time.Time
	email  string
	exp    time.Time
}

type deviceFlow struct {
	// userInput is the expected input to authenticate the device flow.
	userInput string
	exp       time.Time
	granted   bool
}

// fakeIDPLocked is a set of fields of FakeIDP that are protected
// behind a mutex.
type fakeIDPLocked struct {
	mu sync.RWMutex

	issuer     string
	issuerURL  *url.URL
	key        *rsa.PrivateKey
	provider   ProviderJSON
	handler    http.Handler
	cfg        *oauth2.Config
	fakeCoderd func(req *http.Request) (*http.Response, error)
}

func (f *fakeIDPLocked) Issuer() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.issuer
}

func (f *fakeIDPLocked) IssuerURL() *url.URL {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.issuerURL
}

func (f *fakeIDPLocked) PrivateKey() *rsa.PrivateKey {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.key
}

func (f *fakeIDPLocked) Provider() ProviderJSON {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.provider
}

func (f *fakeIDPLocked) Config() *oauth2.Config {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cfg
}

func (f *fakeIDPLocked) Handler() http.Handler {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.handler
}

func (f *fakeIDPLocked) SetIssuer(issuer string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issuer = issuer
}

func (f *fakeIDPLocked) SetIssuerURL(issuerURL *url.URL) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issuerURL = issuerURL
}

func (f *fakeIDPLocked) SetProvider(provider ProviderJSON) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.provider = provider
}

// MutateConfig is a helper function to mutate the oauth2.Config.
// Beware of re-entrant locks!
func (f *fakeIDPLocked) MutateConfig(fn func(cfg *oauth2.Config)) {
	f.mu.Lock()
	if f.cfg == nil {
		f.cfg = &oauth2.Config{}
	}
	fn(f.cfg)
	f.mu.Unlock()
}

func (f *fakeIDPLocked) SetHandler(handler http.Handler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = handler
}

func (f *fakeIDPLocked) SetFakeCoderd(fakeCoderd func(req *http.Request) (*http.Response, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fakeCoderd = fakeCoderd
}

func (f *fakeIDPLocked) FakeCoderd() func(req *http.Request) (*http.Response, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.fakeCoderd
}

// FakeIDP is a functional OIDC provider.
// It only supports 1 OIDC client.
type FakeIDP struct {
	locked fakeIDPLocked

	// callbackPath allows changing where the callback path to coderd is expected.
	// This only affects using the Login helper functions.
	callbackPath string
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
	accessTokens *syncmap.Map[string, token]
	// Refresh Token -> Email
	refreshTokensUsed    *syncmap.Map[string, bool]
	refreshTokens        *syncmap.Map[string, string]
	stateToIDTokenClaims *syncmap.Map[string, jwt.MapClaims]
	refreshIDTokenClaims *syncmap.Map[string, jwt.MapClaims]
	// Device flow
	deviceCode *syncmap.Map[string, deviceFlow]

	// hooks
	// hookWellKnown allows mutating the returned .well-known/configuration JSON.
	// Using this can break the IDP configuration, so be careful.
	hookWellKnown func(r *http.Request, j *ProviderJSON) error
	// hookValidRedirectURL can be used to reject a redirect url from the
	// IDP -> Application. Almost all IDPs have the concept of
	// "Authorized Redirect URLs". This can be used to emulate that.
	hookValidRedirectURL func(redirectURL string) error
	hookUserInfo         func(email string) (jwt.MapClaims, error)
	hookAccessTokenJWT   func(email string, exp time.Time) jwt.MapClaims
	// defaultIDClaims is if a new client connects and we didn't preset
	// some claims.
	defaultIDClaims jwt.MapClaims
	hookMutateToken func(token map[string]interface{})
	hookOnRefresh   func(email string) error
	// Custom authentication for the client. This is useful if you want
	// to test something like PKI auth vs a client_secret.
	hookAuthenticateClient func(t testing.TB, req *http.Request) (url.Values, error)
	serve                  bool
	// optional middlewares
	middlewares   chi.Middlewares
	defaultExpire time.Duration
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

func WithMiddlewares(mws ...func(http.Handler) http.Handler) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.middlewares = append(f.middlewares, mws...)
	}
}

func WithAccessTokenJWTHook(hook func(email string, exp time.Time) jwt.MapClaims) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookAccessTokenJWT = hook
	}
}

func WithHookWellKnown(hook func(r *http.Request, j *ProviderJSON) error) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookWellKnown = hook
	}
}

// WithRefresh is called when a refresh token is used. The email is
// the email of the user that is being refreshed assuming the claims are correct.
func WithRefresh(hook func(email string) error) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookOnRefresh = hook
	}
}

func WithDefaultExpire(d time.Duration) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.defaultExpire = d
	}
}

func WithCallbackPath(path string) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.callbackPath = path
	}
}

func WithStaticCredentials(id, secret string) func(*FakeIDP) {
	return func(f *FakeIDP) {
		if id != "" {
			f.clientID = id
		}
		if secret != "" {
			f.clientSecret = secret
		}
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

func WithLogger(logger slog.Logger) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.logger = logger
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

func WithDefaultIDClaims(claims jwt.MapClaims) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.defaultIDClaims = claims
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
		f.locked.SetIssuer(issuer)
	}
}

type With429Arguments struct {
	AllPaths      bool
	TokenPath     bool
	AuthorizePath bool
	KeysPath      bool
	UserInfoPath  bool
	DeviceAuth    bool
	DeviceVerify  bool
}

// With429 will emulate a 429 response for the selected paths.
func With429(params With429Arguments) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.middlewares = append(f.middlewares, func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if params.AllPaths {
					http.Error(rw, "429, being manually blocked (all)", http.StatusTooManyRequests)
					return
				}
				if params.TokenPath && strings.Contains(r.URL.Path, tokenPath) {
					http.Error(rw, "429, being manually blocked (token)", http.StatusTooManyRequests)
					return
				}
				if params.AuthorizePath && strings.Contains(r.URL.Path, authorizePath) {
					http.Error(rw, "429, being manually blocked (authorize)", http.StatusTooManyRequests)
					return
				}
				if params.KeysPath && strings.Contains(r.URL.Path, keysPath) {
					http.Error(rw, "429, being manually blocked (keys)", http.StatusTooManyRequests)
					return
				}
				if params.UserInfoPath && strings.Contains(r.URL.Path, userInfoPath) {
					http.Error(rw, "429, being manually blocked (userinfo)", http.StatusTooManyRequests)
					return
				}
				if params.DeviceAuth && strings.Contains(r.URL.Path, deviceAuth) {
					http.Error(rw, "429, being manually blocked (device-auth)", http.StatusTooManyRequests)
					return
				}
				if params.DeviceVerify && strings.Contains(r.URL.Path, deviceVerify) {
					http.Error(rw, "429, being manually blocked (device-verify)", http.StatusTooManyRequests)
					return
				}

				next.ServeHTTP(rw, r)
			})
		})
	}
}

const (
	// nolint:gosec // It thinks this is a secret lol
	tokenPath     = "/oauth2/token"
	authorizePath = "/oauth2/authorize"
	keysPath      = "/oauth2/keys"
	userInfoPath  = "/oauth2/userinfo"
	deviceAuth    = "/login/device/code"
	deviceVerify  = "/login/device"
)

func NewFakeIDP(t testing.TB, opts ...FakeIDPOpt) *FakeIDP {
	t.Helper()

	pkey, err := FakeIDPKey()
	require.NoError(t, err)

	idp := &FakeIDP{
		locked: fakeIDPLocked{
			key: pkey,
		},
		clientID:             uuid.NewString(),
		clientSecret:         uuid.NewString(),
		logger:               slog.Make(),
		codeToStateMap:       syncmap.New[string, string](),
		accessTokens:         syncmap.New[string, token](),
		refreshTokens:        syncmap.New[string, string](),
		refreshTokensUsed:    syncmap.New[string, bool](),
		stateToIDTokenClaims: syncmap.New[string, jwt.MapClaims](),
		refreshIDTokenClaims: syncmap.New[string, jwt.MapClaims](),
		deviceCode:           syncmap.New[string, deviceFlow](),
		hookOnRefresh:        func(_ string) error { return nil },
		hookUserInfo:         func(_ string) (jwt.MapClaims, error) { return jwt.MapClaims{}, nil },
		hookValidRedirectURL: func(_ string) error { return nil },
		defaultExpire:        time.Minute * 5,
	}

	for _, opt := range opts {
		opt(idp)
	}

	if idp.locked.Issuer() == "" {
		idp.locked.SetIssuer("https://coder.com")
	}

	idp.locked.SetHandler(idp.httpHandler(t))
	idp.updateIssuerURL(t, idp.locked.Issuer())
	if idp.serve {
		idp.realServer(t)
	}

	// Log the url to indicate which port the IDP is running on if it is
	// being served on a real port.
	idp.logger.Info(context.Background(),
		"fake IDP created",
		slog.F("issuer", idp.IssuerURL().String()),
	)

	return idp
}

func (f *FakeIDP) WellknownConfig() ProviderJSON {
	return f.locked.Provider()
}

func (f *FakeIDP) IssuerURL() *url.URL {
	return f.locked.IssuerURL()
}

func (f *FakeIDP) updateIssuerURL(t testing.TB, issuer string) {
	t.Helper()

	u, err := url.Parse(issuer)
	require.NoError(t, err, "invalid issuer URL")

	f.locked.SetIssuer(issuer)
	f.locked.SetIssuerURL(u)
	// ProviderJSON is the JSON representation of the OpenID Connect provider
	// These are all the urls that the IDP will respond to.
	f.locked.SetProvider(ProviderJSON{
		Issuer:        issuer,
		AuthURL:       u.ResolveReference(&url.URL{Path: authorizePath}).String(),
		TokenURL:      u.ResolveReference(&url.URL{Path: tokenPath}).String(),
		JWKSURL:       u.ResolveReference(&url.URL{Path: keysPath}).String(),
		UserInfoURL:   u.ResolveReference(&url.URL{Path: userInfoPath}).String(),
		DeviceCodeURL: u.ResolveReference(&url.URL{Path: deviceAuth}).String(),
		Algorithms: []string{
			"RS256",
		},
		ExternalAuthURL: u.ResolveReference(&url.URL{Path: "/external-auth-validate/user"}).String(),
	})
}

// realServer turns the FakeIDP into a real http server.
func (f *FakeIDP) realServer(t testing.TB) *httptest.Server {
	t.Helper()

	srvURL := "localhost:0"
	issURL, err := url.Parse(f.locked.Issuer())
	if err == nil {
		if issURL.Hostname() == "localhost" || issURL.Hostname() == "127.0.0.1" {
			srvURL = issURL.Host
		}
	}

	l, err := net.Listen("tcp", srvURL)
	require.NoError(t, err, "failed to create listener")

	ctx, cancel := context.WithCancel(context.Background())
	srv := &httptest.Server{
		Listener: l,
		Config:   &http.Server{Handler: f.locked.Handler(), ReadHeaderTimeout: time.Second * 5},
	}

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
	return f.locked.Config().Exchange(oidc.ClientContext(context.Background(), f.HTTPClient(nil)), code)
}

// Login does the full OIDC flow starting at the "LoginButton".
// The client argument is just to get the URL of the Coder instance.
//
// The client passed in is just to get the url of the Coder instance.
// The actual client that is used is 100% unauthenticated and fresh.
func (f *FakeIDP) Login(t testing.TB, client *codersdk.Client, idTokenClaims jwt.MapClaims, opts ...func(r *http.Request)) (*codersdk.Client, *http.Response) {
	t.Helper()

	client, resp := f.AttemptLogin(t, client, idTokenClaims, opts...)
	if resp.StatusCode != http.StatusOK {
		data, err := httputil.DumpResponse(resp, true)
		if err == nil {
			t.Logf("Attempt Login response payload\n%s", string(data))
		}
	}
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
	path := "/api/v2/users/oidc/callback"
	if f.callbackPath != "" {
		path = f.callbackPath
	}
	coderOauthURL, err := client.URL.Parse(path)
	require.NoError(t, err)
	f.SetRedirect(t, coderOauthURL.String())

	cli := f.HTTPClient(client.HTTPClient)
	redirectFn := cli.CheckRedirect
	checkRedirect := func(req *http.Request, via []*http.Request) error {
		// Store the idTokenClaims to the specific state request. This ties
		// the claims 1:1 with a given authentication flow.
		if state := req.URL.Query().Get("state"); state != "" {
			f.stateToIDTokenClaims.Store(state, idTokenClaims)
			return nil
		}
		// This is mainly intended to prevent the _last_ redirect
		// The one involving the state param is a core part of the
		// OIDC flow and shouldn't be redirected.
		if redirectFn != nil {
			return redirectFn(req, via)
		}
		return nil
	}
	cli.CheckRedirect = checkRedirect

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
	cli.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
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

// DeviceLogin does the oauth2 device flow for external auth providers.
func (*FakeIDP) DeviceLogin(t testing.TB, client *codersdk.Client, externalAuthID string) {
	// First we need to initiate the device flow. This will have Coder hit the
	// fake IDP and get a device code.
	device, err := client.ExternalAuthDeviceByID(context.Background(), externalAuthID)
	require.NoError(t, err)

	// Now the user needs to go to the fake IDP page and click "allow" and enter
	// the device code input. For our purposes, we just send an http request to
	// the verification url. No additional user input is needed.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	resp, err := client.Request(ctx, http.MethodPost, device.VerificationURI, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Now we need to exchange the device code for an access token. We do this
	// in this method because it is the user that does the polling for the device
	// auth flow, not the backend.
	err = client.ExternalAuthDeviceExchange(context.Background(), externalAuthID, codersdk.ExternalAuthDeviceExchange{
		DeviceCode: device.DeviceCode,
	})
	require.NoError(t, err)
}

// CreateAuthCode emulates a user clicking "allow" on the IDP page. When doing
// unit tests, it's easier to skip this step sometimes. It does make an actual
// request to the IDP, so it should be equivalent to doing this "manually" with
// actual requests.
func (f *FakeIDP) CreateAuthCode(t testing.TB, state string) string {
	// We need to store some claims, because this is also an OIDC provider, and
	// it expects some claims to be present.
	f.stateToIDTokenClaims.Store(state, jwt.MapClaims{})

	code, err := OAuth2GetCode(f.locked.Config().AuthCodeURL(state), func(req *http.Request) (*http.Response, error) {
		rw := httptest.NewRecorder()
		f.locked.Handler().ServeHTTP(rw, req)
		resp := rw.Result()
		return resp, nil
	})
	require.NoError(t, err, "failed to get auth code")
	return code
}

// OIDCCallback will emulate the IDP redirecting back to the Coder callback.
// This is helpful if no Coderd exists because the IDP needs to redirect to
// something.
// Essentially this is used to fake the Coderd side of the exchange.
// The flow starts at the user hitting the OIDC login page.
func (f *FakeIDP) OIDCCallback(t testing.TB, state string, idTokenClaims jwt.MapClaims) *http.Response {
	t.Helper()
	if f.serve {
		panic("cannot use OIDCCallback with WithServing. This is only for the in memory usage")
	}

	f.stateToIDTokenClaims.Store(state, idTokenClaims)

	cli := f.HTTPClient(nil)
	u := f.locked.Config().AuthCodeURL(state)
	req, err := http.NewRequest("GET", u, nil)
	require.NoError(t, err)

	resp, err := cli.Do(req.WithContext(context.Background()))
	require.NoError(t, err)

	t.Cleanup(func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	})
	return resp
}

// ProviderJSON is the .well-known/configuration JSON
type ProviderJSON struct {
	Issuer        string   `json:"issuer"`
	AuthURL       string   `json:"authorization_endpoint"`
	TokenURL      string   `json:"token_endpoint"`
	JWKSURL       string   `json:"jwks_uri"`
	UserInfoURL   string   `json:"userinfo_endpoint"`
	DeviceCodeURL string   `json:"device_authorization_endpoint"`
	Algorithms    []string `json:"id_token_signing_alg_values_supported"`
	// This is custom
	ExternalAuthURL string `json:"external_auth_url"`
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
func (f *FakeIDP) newToken(t testing.TB, email string, expires time.Time) string {
	accessToken := uuid.NewString()
	if f.hookAccessTokenJWT != nil {
		claims := f.hookAccessTokenJWT(email, expires)
		accessToken = f.encodeClaims(t, claims)
	}

	f.accessTokens.Store(accessToken, token{
		issued: time.Now(),
		email:  email,
		exp:    expires,
	})
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
	authToken, ok := f.accessTokens.Load(token)
	if !ok {
		return "", xerrors.New("invalid access token")
	}

	if !authToken.exp.IsZero() && authToken.exp.Before(time.Now()) {
		return "", xerrors.New("access token expired")
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
		claims["iss"] = f.locked.Issuer()
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(f.locked.PrivateKey())
	require.NoError(t, err)

	return signed
}

// httpHandler is the IDP http server.
func (f *FakeIDP) httpHandler(t testing.TB) http.Handler {
	t.Helper()

	mux := chi.NewMux()
	mux.Use(f.middlewares...)
	// This endpoint is required to initialize the OIDC provider.
	// It is used to get the OIDC configuration.
	mux.Get("/.well-known/openid-configuration", func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "http OIDC config", slogRequestFields(r)...)

		cpy := f.locked.Provider()
		if f.hookWellKnown != nil {
			err := f.hookWellKnown(r, &cpy)
			if err != nil {
				httpError(rw, http.StatusInternalServerError, err)
				return
			}
		}

		_ = json.NewEncoder(rw).Encode(cpy)
	})

	// Authorize is called when the user is redirected to the IDP to login.
	// This is the browser hitting the IDP and the user logging into Google or
	// w/e and clicking "Allow". They will be redirected back to the redirect
	// when this is done.
	mux.Handle(authorizePath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "http call authorize", slogRequestFields(r)...)

		clientID := r.URL.Query().Get("client_id")
		if !assert.Equal(t, f.clientID, clientID, "unexpected client_id") {
			httpError(rw, http.StatusBadRequest, xerrors.New("invalid client_id"))
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
			httpError(rw, http.StatusBadRequest, xerrors.Errorf("invalid redirect_uri: %w", err))
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
		var values url.Values
		var err error
		if r.URL.Query().Get("grant_type") == "urn:ietf:params:oauth:grant-type:device_code" {
			values = r.URL.Query()
		} else {
			values, err = f.authenticateOIDCClientRequest(t, r)
		}
		f.logger.Info(r.Context(), "http idp call token",
			append(slogRequestFields(r),
				slog.F("valid", err == nil),
				slog.F("grant_type", values.Get("grant_type")),
				slog.F("values", values.Encode()),
			)...)

		if err != nil {
			httpError(rw, http.StatusBadRequest, err)
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

			idTokenClaims, ok := f.getClaims(f.stateToIDTokenClaims, stateStr)
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

			idTokenClaims, ok := f.getClaims(f.refreshIDTokenClaims, refreshToken)
			if !ok {
				t.Errorf("missing id token claims in refresh")
				http.Error(rw, "missing id token claims in refresh", http.StatusBadRequest)
				return
			}

			claims = idTokenClaims
			err := f.hookOnRefresh(getEmail(claims))
			if err != nil {
				httpError(rw, http.StatusBadRequest, xerrors.Errorf("refresh hook blocked refresh: %w", err))
				return
			}

			f.refreshTokensUsed.Store(refreshToken, true)
			// Always invalidate the refresh token after it is used.
			f.refreshTokens.Delete(refreshToken)
		case "urn:ietf:params:oauth:grant-type:device_code":
			// Device flow
			var resp externalauth.ExchangeDeviceCodeResponse
			deviceCode := values.Get("device_code")
			if deviceCode == "" {
				resp.Error = "invalid_request"
				resp.ErrorDescription = "missing device_code"
				httpapi.Write(r.Context(), rw, http.StatusBadRequest, resp)
				return
			}

			deviceFlow, ok := f.deviceCode.Load(deviceCode)
			if !ok {
				resp.Error = "invalid_request"
				resp.ErrorDescription = "device_code provided not found"
				httpapi.Write(r.Context(), rw, http.StatusBadRequest, resp)
				return
			}

			if !deviceFlow.granted {
				// Status code ok with the error as pending.
				resp.Error = "authorization_pending"
				resp.ErrorDescription = ""
				httpapi.Write(r.Context(), rw, http.StatusOK, resp)
				return
			}

			// Would be nice to get an actual email here.
			claims = jwt.MapClaims{
				"email": "unknown-dev-auth",
			}
		default:
			t.Errorf("unexpected grant_type %q", values.Get("grant_type"))
			http.Error(rw, "invalid grant_type", http.StatusBadRequest)
			return
		}

		exp := time.Now().Add(f.defaultExpire)
		claims["exp"] = exp.UnixMilli()
		email := getEmail(claims)
		refreshToken := f.newRefreshTokens(email)
		token := map[string]interface{}{
			"access_token":  f.newToken(t, email, exp),
			"refresh_token": refreshToken,
			"token_type":    "Bearer",
			"expires_in":    int64((f.defaultExpire).Seconds()),
			"id_token":      f.encodeClaims(t, claims),
		}
		if f.hookMutateToken != nil {
			f.hookMutateToken(token)
		}
		// Store the claims for the next refresh
		f.refreshIDTokenClaims.Store(refreshToken, claims)

		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Accept"))
		if mediaType == "application/x-www-form-urlencoded" {
			// This val encode might not work for some data structures.
			// It's good enough for now...
			rw.Header().Set("Content-Type", "application/x-www-form-urlencoded")
			vals := url.Values{}
			for k, v := range token {
				vals.Set(k, fmt.Sprintf("%v", v))
			}
			_, _ = rw.Write([]byte(vals.Encode()))
			return
		}
		// Default to json since the oauth2 package doesn't use Accept headers.
		if mediaType == "application/json" || mediaType == "" {
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(token)
			return
		}

		// If we get something we don't support, throw an error.
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "'Accept' header contains unsupported media type",
			Detail:  fmt.Sprintf("Found %q", mediaType),
		})
	}))

	validateMW := func(rw http.ResponseWriter, r *http.Request) (email string, ok bool) {
		token, err := f.authenticateBearerTokenRequest(t, r)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid user info request: %s", err.Error()), http.StatusUnauthorized)
			return "", false
		}

		authToken, ok := f.accessTokens.Load(token)
		if !ok {
			t.Errorf("access token user for user_info has no email to indicate which user")
			http.Error(rw, "invalid access token, missing user info", http.StatusUnauthorized)
			return "", false
		}

		if !authToken.exp.IsZero() && authToken.exp.Before(time.Now()) {
			http.Error(rw, "auth token expired", http.StatusUnauthorized)
			return "", false
		}

		return authToken.email, true
	}
	mux.Handle(userInfoPath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		email, ok := validateMW(rw, r)
		f.logger.Info(r.Context(), "http userinfo endpoint",
			append(slogRequestFields(r),
				slog.F("valid", ok),
				slog.F("email", email),
			)...,
		)
		if !ok {
			return
		}

		claims, err := f.hookUserInfo(email)
		if err != nil {
			httpError(rw, http.StatusBadRequest, xerrors.Errorf("user info hook returned error: %w", err))
			return
		}
		_ = json.NewEncoder(rw).Encode(claims)
	}))

	// There is almost no difference between this and /userinfo.
	// The main tweak is that this route is "mounted" vs "handle" because "/userinfo"
	// should be strict, and this one needs to handle sub routes.
	mux.Mount("/external-auth-validate/", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		email, ok := validateMW(rw, r)
		f.logger.Info(r.Context(), "http external auth validate",
			append(slogRequestFields(r),
				slog.F("valid", ok),
				slog.F("email", email),
			)...,
		)
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
		f.logger.Info(r.Context(), "http call idp /keys", slogRequestFields(r)...)
		set := jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{
				{
					Key:       f.locked.PrivateKey().Public(),
					KeyID:     "test-key",
					Algorithm: "RSA",
				},
			},
		}
		_ = json.NewEncoder(rw).Encode(set)
	}))

	mux.Handle(deviceVerify, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "http call device verify", slogRequestFields(r)...)

		inputParam := "user_input"
		userInput := r.URL.Query().Get(inputParam)
		if userInput == "" {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid user input",
				Detail:  fmt.Sprintf("Hit this url again with ?%s=<user_code>", inputParam),
			})
			return
		}

		deviceCode := r.URL.Query().Get("device_code")
		if deviceCode == "" {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid device code",
				Detail:  "Hit this url again with ?device_code=<device_code>",
			})
			return
		}

		flow, ok := f.deviceCode.Load(deviceCode)
		if !ok {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid device code",
				Detail:  "Device code not found.",
			})
			return
		}

		if time.Now().After(flow.exp) {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid device code",
				Detail:  "Device code expired.",
			})
			return
		}

		if strings.TrimSpace(flow.userInput) != strings.TrimSpace(userInput) {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid device code",
				Detail:  "user code does not match",
			})
			return
		}

		f.deviceCode.Store(deviceCode, deviceFlow{
			userInput: flow.userInput,
			exp:       flow.exp,
			granted:   true,
		})
		httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
			Message: "Device authenticated!",
		})
	}))

	mux.Handle(deviceAuth, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "http call device auth", slogRequestFields(r)...)

		p := httpapi.NewQueryParamParser()
		p.RequiredNotEmpty("client_id")
		clientID := p.String(r.URL.Query(), "", "client_id")
		_ = p.String(r.URL.Query(), "", "scopes")
		if len(p.Errors) > 0 {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid query params",
				Validations: p.Errors,
			})
			return
		}

		if clientID != f.clientID {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid client id",
			})
			return
		}

		deviceCode := uuid.NewString()
		lifetime := time.Second * 900
		flow := deviceFlow{
			//nolint:gosec
			userInput: fmt.Sprintf("%d", rand.Intn(9999999)+1e8),
		}
		f.deviceCode.Store(deviceCode, deviceFlow{
			userInput: flow.userInput,
			exp:       time.Now().Add(lifetime),
		})

		verifyURL := f.locked.IssuerURL().ResolveReference(&url.URL{
			Path: deviceVerify,
			RawQuery: url.Values{
				"device_code": {deviceCode},
				"user_input":  {flow.userInput},
			}.Encode(),
		}).String()

		if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Accept")); mediaType == "application/json" {
			httpapi.Write(r.Context(), rw, http.StatusOK, map[string]any{
				"device_code":      deviceCode,
				"user_code":        flow.userInput,
				"verification_uri": verifyURL,
				"expires_in":       int(lifetime.Seconds()),
				"interval":         3,
			})
			return
		}

		// By default, GitHub form encodes these.
		_, _ = fmt.Fprint(rw, url.Values{
			"device_code":      {deviceCode},
			"user_code":        {flow.userInput},
			"verification_uri": {verifyURL},
			"expires_in":       {strconv.Itoa(int(lifetime.Seconds()))},
			"interval":         {"3"},
		}.Encode())
	}))

	mux.NotFound(func(_ http.ResponseWriter, r *http.Request) {
		f.logger.Error(r.Context(), "http call not found", slogRequestFields(r)...)
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
		if rest == nil {
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
				u, _ := url.Parse(f.locked.Issuer())
				if req.URL.Host != u.Host {
					if fakeCoderd := f.locked.FakeCoderd(); fakeCoderd != nil {
						return fakeCoderd(req)
					}
					if rest == nil || rest.Transport == nil {
						return nil, xerrors.Errorf("unexpected network request to %q", req.URL.Host)
					}
					return rest.Transport.RoundTrip(req)
				}
				resp := httptest.NewRecorder()
				f.locked.Handler().ServeHTTP(resp, req)
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
	// no mutex because it's a sync.Map
	f.refreshIDTokenClaims.Store(refreshToken, claims)
}

// SetRedirect is required for the IDP to know where to redirect and call
// Coderd.
func (f *FakeIDP) SetRedirect(t testing.TB, u string) {
	t.Helper()
	f.locked.MutateConfig(func(cfg *oauth2.Config) {
		cfg.RedirectURL = u
	})
}

// SetCoderdCallback is optional and only works if not using the IsServing.
// It will setup a fake "Coderd" for the IDP to call when the IDP redirects
// back after authenticating.
func (f *FakeIDP) SetCoderdCallback(callback func(req *http.Request) (*http.Response, error)) {
	if f.serve {
		panic("cannot set callback handler when using 'WithServing'. Must implement an actual 'Coderd'")
	}
	f.locked.SetFakeCoderd(callback)
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
	//
	// The int,error payload can control the response if set.
	ValidatePayload func(email string) (interface{}, int, error)

	// routes is more advanced usage. This allows the caller to
	// completely customize the response. It captures all routes under the /external-auth-validate/*
	// so the caller can do whatever they want and even add routes.
	routes map[string]func(email string, rw http.ResponseWriter, r *http.Request)

	UseDeviceAuth bool
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
		newPath := strings.TrimPrefix(r.URL.Path, "/external-auth-validate")
		switch newPath {
		// /user is ALWAYS supported under the `/` path too.
		case "/user", "/", "":
			var payload interface{} = "OK"
			if custom.ValidatePayload != nil {
				var err error
				var code int
				payload, code, err = custom.ValidatePayload(email)
				if code == 0 && err == nil {
					code = http.StatusOK
				}
				if code == 0 && err != nil {
					code = http.StatusUnauthorized
				}
				if err != nil {
					http.Error(rw, fmt.Sprintf("failed validation via custom method: %s", err.Error()), code)
					return
				}
				rw.WriteHeader(code)
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
	instrumentF := promoauth.NewFactory(prometheus.NewRegistry())
	oauthCfg := instrumentF.New(f.clientID, f.OIDCConfig(t, nil))
	cfg := &externalauth.Config{
		DisplayName:              id,
		InstrumentedOAuth2Config: oauthCfg,
		ID:                       id,
		// No defaults for these fields by omitting the type
		Type:        "",
		DisplayIcon: f.WellknownConfig().UserInfoURL,
		// Omit the /user for the validate so we can easily append to it when modifying
		// the cfg for advanced tests.
		ValidateURL: f.locked.IssuerURL().ResolveReference(&url.URL{Path: "/external-auth-validate/"}).String(),
		DeviceAuth: &externalauth.DeviceAuth{
			Config:   oauthCfg,
			ClientID: f.clientID,
			TokenURL: f.locked.Provider().TokenURL,
			Scopes:   []string{},
			CodeURL:  f.locked.Provider().DeviceCodeURL,
		},
	}

	if !custom.UseDeviceAuth {
		cfg.DeviceAuth = nil
	}

	for _, opt := range opts {
		opt(cfg)
	}
	f.updateIssuerURL(t, f.locked.Issuer())
	return cfg
}

func (f *FakeIDP) AppCredentials() (clientID string, clientSecret string) {
	return f.clientID, f.clientSecret
}

func (f *FakeIDP) PublicKey() crypto.PublicKey {
	return f.locked.PrivateKey().Public()
}

func (f *FakeIDP) OauthConfig(t testing.TB, scopes []string) *oauth2.Config {
	t.Helper()

	provider := f.locked.Provider()
	f.locked.MutateConfig(func(cfg *oauth2.Config) {
		if len(scopes) == 0 {
			scopes = []string{"openid", "email", "profile"}
		}
		cfg.ClientID = f.clientID
		cfg.ClientSecret = f.clientSecret
		cfg.Endpoint = oauth2.Endpoint{
			AuthURL:   provider.AuthURL,
			TokenURL:  provider.TokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		}
		// If the user is using a real network request, they will need to do
		// 'fake.SetRedirect()'
		cfg.RedirectURL = "https://redirect.com"
		cfg.Scopes = scopes
	})

	return f.locked.Config()
}

func (f *FakeIDP) OIDCConfigSkipIssuerChecks(t testing.TB, scopes []string, opts ...func(cfg *coderd.OIDCConfig)) *coderd.OIDCConfig {
	ctx := oidc.InsecureIssuerURLContext(context.Background(), f.locked.Issuer())

	return f.internalOIDCConfig(ctx, t, scopes, func(config *oidc.Config) {
		config.SkipIssuerCheck = true
	}, opts...)
}

func (f *FakeIDP) OIDCConfig(t testing.TB, scopes []string, opts ...func(cfg *coderd.OIDCConfig)) *coderd.OIDCConfig {
	return f.internalOIDCConfig(context.Background(), t, scopes, nil, opts...)
}

// OIDCConfig returns the OIDC config to use for Coderd.
func (f *FakeIDP) internalOIDCConfig(ctx context.Context, t testing.TB, scopes []string, verifierOpt func(config *oidc.Config), opts ...func(cfg *coderd.OIDCConfig)) *coderd.OIDCConfig {
	t.Helper()

	oauthCfg := f.OauthConfig(t, scopes)

	ctx = oidc.ClientContext(ctx, f.HTTPClient(nil))
	p, err := oidc.NewProvider(ctx, f.locked.Issuer())
	require.NoError(t, err, "failed to create OIDC provider")

	verifierConfig := &oidc.Config{
		ClientID: oauthCfg.ClientID,
		SupportedSigningAlgs: []string{
			"RS256",
		},
		// Todo: add support for Now()
	}
	if verifierOpt != nil {
		verifierOpt(verifierConfig)
	}

	cfg := &coderd.OIDCConfig{
		OAuth2Config: oauthCfg,
		Provider:     p,
		Verifier: oidc.NewVerifier(f.locked.Issuer(), &oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{f.locked.PrivateKey().Public()},
		}, verifierConfig),
		UsernameField:   "preferred_username",
		EmailField:      "email",
		AuthURLParams:   map[string]string{"access_type": "offline"},
		SecondaryClaims: coderd.MergedClaimsSourceUserInfo,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(cfg)
	}

	return cfg
}

func (f *FakeIDP) getClaims(m *syncmap.Map[string, jwt.MapClaims], key string) (jwt.MapClaims, bool) {
	v, ok := m.Load(key)
	if !ok || v == nil {
		if f.defaultIDClaims != nil {
			return f.defaultIDClaims, true
		}
		return nil, false
	}
	return v, true
}

func slogRequestFields(r *http.Request) []any {
	return []any{
		slog.F("url", r.URL.String()),
		slog.F("host", r.Host),
		slog.F("method", r.Method),
	}
}

// httpError handles better formatted custom errors.
func httpError(rw http.ResponseWriter, defaultCode int, err error) {
	status := defaultCode

	var statusErr statusHookError
	if errors.As(err, &statusErr) {
		status = statusErr.HTTPStatusCode
	}

	var oauthErr *oauth2.RetrieveError
	if errors.As(err, &oauthErr) {
		if oauthErr.Response.StatusCode != 0 {
			status = oauthErr.Response.StatusCode
		}

		rw.Header().Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
		form := url.Values{
			"error":             {oauthErr.ErrorCode},
			"error_description": {oauthErr.ErrorDescription},
			"error_uri":         {oauthErr.ErrorURI},
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(form.Encode()))
		return
	}

	http.Error(rw, err.Error(), status)
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

func FakeIDPKey() (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(testRSAPrivateKey))
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
