package oidctest

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/go-jose/go-jose/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
)

type FakeIDP struct {
	issuer   string
	key      *rsa.PrivateKey
	provider providerJSON
	handler  http.Handler

	// clientID to be used by coderd
	clientID     string
	clientSecret string
	logger       slog.Logger

	codeToStateMap sync.Map
	accessTokens   sync.Map
	refreshTokens  sync.Map

	// hooks
	hookUserInfo func(token string) map[string]string
}

func WithLogging(t testing.TB, options *slogtest.Options) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.logger = slogtest.Make(t, options)
	}
}

func WithUserInfoHook(uf func(token string) map[string]string) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.hookUserInfo = uf
	}
}

func WithIssuer(issuer string) func(*FakeIDP) {
	return func(f *FakeIDP) {
		f.issuer = issuer
	}
}

const (
	authorizePath = "/oauth2/authorize"
	tokenPath     = "/oauth2/token"
	keysPath      = "/oauth2/keys"
	userInfoPath  = "/oauth2/userinfo"
)

func NewFakeIDP(t testing.TB, opts ...func(idp *FakeIDP)) *FakeIDP {
	t.Helper()

	block, _ := pem.Decode([]byte(testRSAPrivateKey))
	pkey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)

	idp := &FakeIDP{
		key:            pkey,
		clientID:       uuid.NewString(),
		clientSecret:   uuid.NewString(),
		logger:         slog.Make(),
		codeToStateMap: sync.Map{},
		accessTokens:   sync.Map{},
		refreshTokens:  sync.Map{},
		hookUserInfo: func(token string) map[string]string {
			return map[string]string{}
		},
	}

	for _, opt := range opts {
		opt(idp)
	}

	if idp.issuer == "" {
		idp.issuer = "https://coder.com"
	}

	u, err := url.Parse(idp.issuer)
	require.NoError(t, err, "invalid issuer URL")

	// providerJSON is the JSON representation of the OpenID Connect provider
	// These are all the urls that the IDP will respond to.
	idp.provider = providerJSON{
		Issuer:      idp.issuer,
		AuthURL:     u.ResolveReference(&url.URL{Path: authorizePath}).String(),
		TokenURL:    u.ResolveReference(&url.URL{Path: tokenPath}).String(),
		JWKSURL:     u.ResolveReference(&url.URL{Path: keysPath}).String(),
		UserInfoURL: u.ResolveReference(&url.URL{Path: userInfoPath}).String(),
		Algorithms: []string{
			"RS256",
		},
	}
	idp.handler = idp.httpHandler(t)

	return idp
}

func OIDCCallback(t testing.TB, cfg *coderd.OIDCConfig, cli *http.Client, state string) (*http.Response, error) {
	t.Helper()

	url := cfg.AuthCodeURL(state)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	resp, err := cli.Do(req.WithContext(context.Background()))
	require.NoError(t, err)

	return resp, nil
}

type providerJSON struct {
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
func (f *FakeIDP) newToken(exp time.Time) string {
	accessToken := uuid.NewString()
	f.accessTokens.Store(accessToken, exp)
	return accessToken
}

func (f *FakeIDP) newRefreshTokens(exp time.Time) string {
	refreshToken := uuid.NewString()
	f.refreshTokens.Store(refreshToken, exp)
	return refreshToken
}

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

func (f *FakeIDP) authenticateOIDClientRequest(t testing.TB, req *http.Request) (url.Values, error) {
	t.Helper()

	data, _ := io.ReadAll(req.Body)
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

func (f *FakeIDP) httpHandler(t testing.TB) http.Handler {
	t.Helper()

	mux := chi.NewMux()
	// This endpoint is required to initialize the OIDC provider.
	// It is used to get the OIDC configuration.
	mux.Get("/.well-known/openid-configuration", func(rw http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(rw).Encode(f.provider)
	})

	// Authorize is called when the user is redirected to the IDP to login.
	// This is the browser hitting the IDP and the user logging into Google or
	// w/e and clicking "Allow". They will be redirected back to the redirect
	// when this is done.
	mux.Handle(authorizePath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "HTTP Call Authorize", slog.F("url", string(r.URL.String())))

		clientID := r.URL.Query().Get("client_id")
		if clientID != f.clientID {
			t.Errorf("unexpected client_id %q", clientID)
			http.Error(rw, "invalid client_id", http.StatusBadRequest)
		}

		redirectURI := r.URL.Query().Get("redirect_uri")
		state := r.URL.Query().Get("state")

		scope := r.URL.Query().Get("scope")
		var _ = scope

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

		ru, err := url.Parse(redirectURI)
		if err != nil {
			t.Errorf("invalid redirect_uri %q", redirectURI)
			http.Error(rw, "invalid redirect_uri", http.StatusBadRequest)
			return
		}

		q := ru.Query()
		q.Set("state", state)
		q.Set("code", f.newCode(state))
		ru.RawQuery = q.Encode()

		http.Redirect(rw, r, ru.String(), http.StatusTemporaryRedirect)
	}))

	mux.Handle(tokenPath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		values, err := f.authenticateOIDClientRequest(t, r)
		f.logger.Info(r.Context(), "HTTP Call Token",
			slog.Error(err),
			slog.F("values", values.Encode()),
		)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid token request: %s", err.Error()), http.StatusBadRequest)
			return
		}

		switch values.Get("grant_type") {
		case "authorization_code":
			code := values.Get("code")
			if !assert.NotEmpty(t, code, "code is empty") {
				http.Error(rw, "invalid code", http.StatusBadRequest)
				return
			}
			_, ok := f.codeToStateMap.Load(code)
			if !assert.True(t, ok, "invalid code") {
				http.Error(rw, "invalid code", http.StatusBadRequest)
				return
			}
			// Always invalidate the code after it is used.
			f.codeToStateMap.Delete(code)
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
			// Always invalidate the refresh token after it is used.
			f.refreshTokens.Delete(refreshToken)
		default:
			t.Errorf("unexpected grant_type %q", values.Get("grant_type"))
			http.Error(rw, "invalid grant_type", http.StatusBadRequest)
			return
		}

		exp := time.Now().Add(time.Minute * 5)
		token := oauth2.Token{
			// Sometimes the access token is a jwt. Not going to do that here.
			AccessToken:  f.newToken(time.Now().Add(time.Minute * 5)),
			RefreshToken: f.newRefreshTokens(time.Now().Add(time.Minute * 30)),
			TokenType:    "Bearer",
			Expiry:       exp,
		}

		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(token)
	}))

	mux.Handle(userInfoPath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		token, err := f.authenticateBearerTokenRequest(t, r)
		f.logger.Info(r.Context(), "HTTP Call UserInfo",
			slog.Error(err),
		)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid user info request: %s", err.Error()), http.StatusBadRequest)
			return
		}

		_ = json.NewEncoder(rw).Encode(f.hookUserInfo(token))
	}))

	mux.Handle(keysPath, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		f.logger.Info(r.Context(), "HTTP Call Keys")
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
		f.logger.Error(r.Context(), "HTTP Call NotFound", slog.F("path", r.URL.Path))
		t.Errorf("unexpected request to IDP at path %q. Not supported", r.URL.Path)
	})

	return mux
}

// HTTPClient runs the IDP in memory and returns an http.Client that can be used
// to make requests to the IDP. All requests are handled in memory, and no network
// requests are made.
//
// If a request is not to the IDP, then the passed in client will be used.
// If no client is passed in, then any regular network requests will fail.
func (f *FakeIDP) HTTPClient(rest *http.Client) *http.Client {
	return &http.Client{
		Transport: fakeRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				u, _ := url.Parse(f.issuer)
				if req.URL.Host != u.Host {
					if rest == nil {
						return nil, fmt.Errorf("unexpected request to %q", req.URL.Host)
					}
					return rest.Do(req)
				}
				resp := httptest.NewRecorder()
				f.handler.ServeHTTP(resp, req)
				return resp.Result(), nil
			},
		},
	}
}

func (f *FakeIDP) OIDCConfig(t testing.TB, redirect string, scopes []string, opts ...func(cfg *coderd.OIDCConfig)) *coderd.OIDCConfig {
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
		RedirectURL: redirect,
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
		opt(cfg)
	}

	return cfg
}

type fakeRoundTripper struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (f fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.roundTrip(req)
}

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
