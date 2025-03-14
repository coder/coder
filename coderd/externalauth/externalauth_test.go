package externalauth_test

import (
	"errors"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)
func TestRefreshToken(t *testing.T) {
	t.Parallel()
	expired := time.Now().Add(time.Hour * -1)

	t.Run("NoRefreshExpired", func(t *testing.T) {
		t.Parallel()
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{

				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but NoRefresh was set")
					return errors.New("should not be called")
				}),
				// The IDP should not be contacted since the token is expired. An expired
				// token with 'NoRefresh' should early abort.
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					t.Error("token was validated, but it was expired and this should never have happened.")
					return nil, errors.New("should not be called")
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.NoRefresh = true
			},
		})
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Expire the link
		link.OAuthExpiry = expired
		_, err := config.RefreshToken(ctx, nil, link)
		require.Error(t, err)

		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Contains(t, err.Error(), "refreshing is either disabled or refreshing failed")
	})
	// NoRefreshNoExpiry tests that an oauth token without an expiry is always valid.

	// The "validate url" should be hit, but the refresh endpoint should not.
	t.Run("NoRefreshNoExpiry", func(t *testing.T) {
		t.Parallel()
		validated := false
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{

				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but NoRefresh was set")
					return errors.New("should not be called")
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {

					validated = true
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.NoRefresh = true
			},
		})
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Zero time used
		link.OAuthExpiry = time.Time{}
		_, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.True(t, validated, "token should have been validated")
	})
	t.Run("FalseIfTokenSourceFails", func(t *testing.T) {
		t.Parallel()

		config := &externalauth.Config{
			InstrumentedOAuth2Config: &testutil.OAuth2Config{

				TokenSourceFunc: func() (*oauth2.Token, error) {
					return nil, errors.New("failure")
				},
			},
		}
		_, err := config.RefreshToken(context.Background(), nil, database.ExternalAuthLink{
			OAuthExpiry: expired,

		})
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Contains(t, err.Error(), "failure")
	})
	t.Run("ValidateServerError", func(t *testing.T) {
		t.Parallel()
		const staticError = "static error"
		validated := false
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validated = true
					return jwt.MapClaims{}, errors.New(staticError)
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {

			},
		})
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))

		link.OAuthExpiry = expired
		_, err := config.RefreshToken(ctx, nil, link)
		require.ErrorContains(t, err, staticError)
		// Unsure if this should be the correct behavior. It's an invalid token because
		// 'ValidateToken()' failed with a runtime error. This was the previous behavior,
		// so not going to change it.
		require.False(t, externalauth.IsInvalidTokenError(err))
		require.True(t, validated, "token should have been attempted to be validated")
	})
	// RefreshRetries tests that refresh token retry behavior works as expected.
	// If a refresh token fails because the token itself is invalid, no more
	// refresh attempts should ever happen. An invalid refresh token does
	// not magically become valid at some point in the future.

	t.Run("RefreshRetries", func(t *testing.T) {
		t.Parallel()
		var refreshErr *oauth2.RetrieveError

		ctrl := gomock.NewController(t)
		mDB := dbmock.NewMockStore(ctrl)
		refreshCount := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					refreshCount++
					return refreshErr
				}),

				// The IDP should not be contacted since the token is expired and
				// refresh attempts will fail.
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					t.Error("token was validated, but it was expired and this should never have happened.")
					return nil, errors.New("should not be called")
				}),
			},

			ExternalAuthOpt: func(cfg *externalauth.Config) {},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Expire the link
		link.OAuthExpiry = expired

		// Make the failure a server internal error. Not related to the token
		refreshErr = &oauth2.RetrieveError{
			Response: &http.Response{
				StatusCode: http.StatusInternalServerError,
			},
			ErrorCode: "internal_error",
		}
		_, err := config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, refreshCount, 1)
		// Try again with a bad refresh token error
		// Expect DB call to remove the refresh token
		mDB.EXPECT().UpdateExternalAuthLinkRefreshToken(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		refreshErr = &oauth2.RetrieveError{ // github error
			Response: &http.Response{
				StatusCode: http.StatusOK,

			},
			ErrorCode: "bad_refresh_token",
		}
		_, err = config.RefreshToken(ctx, mDB, link)

		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, refreshCount, 2)
		// When the refresh token is empty, no api calls should be made
		link.OAuthRefreshToken = "" // mock'd db, so manually set the token to ''
		_, err = config.RefreshToken(ctx, mDB, link)
		require.Error(t, err)
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.Equal(t, refreshCount, 2)
	})
	// ValidateFailure tests if the token is no longer valid with a 401 response.
	t.Run("ValidateFailure", func(t *testing.T) {

		t.Parallel()
		const staticError = "static error"
		validated := false
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validated = true
					return jwt.MapClaims{}, oidctest.StatusError(http.StatusUnauthorized, errors.New(staticError))
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
			},
		})
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))

		link.OAuthExpiry = expired
		_, err := config.RefreshToken(ctx, nil, link)
		require.ErrorContains(t, err, "token failed to validate")
		require.True(t, externalauth.IsInvalidTokenError(err))
		require.True(t, validated, "token should have been attempted to be validated")
	})
	t.Run("ValidateRetryGitHub", func(t *testing.T) {
		t.Parallel()

		const staticError = "static error"
		validateCalls := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{

				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but the token is not expired")
					return errors.New("should not be called")
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validateCalls++
					// Make the first call return a 401, subsequent calls should return a 200.
					if validateCalls > 1 {
						return jwt.MapClaims{}, nil
					}
					return jwt.MapClaims{}, oidctest.StatusError(http.StatusUnauthorized, errors.New(staticError))
				}),
			},

			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},

		})
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Unlimited lifetime, this is what GitHub returns tokens as
		link.OAuthExpiry = time.Time{}
		_, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)

		require.Equal(t, 2, validateCalls, "token should have been attempted to be validated more than once")
	})
	t.Run("ValidateNoUpdate", func(t *testing.T) {

		t.Parallel()
		validateCalls := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but the token is not expired")
					return errors.New("should not be called")
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validateCalls++
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
		})
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		_, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.Equal(t, 1, validateCalls, "token is validated")
	})

	// A token update comes from a refresh.
	t.Run("Updates", func(t *testing.T) {
		t.Parallel()
		db := dbmem.New()

		validateCalls := 0
		refreshCalls := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {

					refreshCalls++
					return nil
				}),

				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validateCalls++
					return jwt.MapClaims{}, nil
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
			DB: db,
		})
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Force a refresh
		link.OAuthExpiry = expired
		updated, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err)
		require.Equal(t, 1, validateCalls, "token is validated")
		require.Equal(t, 1, refreshCalls, "token is refreshed")

		require.NotEqualf(t, link.OAuthAccessToken, updated.OAuthAccessToken, "token is updated")
		//nolint:gocritic // testing

		dbLink, err := db.GetExternalAuthLink(dbauthz.AsSystemRestricted(context.Background()), database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)

		require.Equal(t, updated.OAuthAccessToken, dbLink.OAuthAccessToken, "token is updated in the DB")
	})
	t.Run("WithExtra", func(t *testing.T) {
		t.Parallel()

		db := dbmem.New()
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithMutateToken(func(token map[string]interface{}) {
					token["authed_user"] = map[string]interface{}{
						"access_token": token["access_token"],
					}
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderSlack.String()
				cfg.ExtraTokenKeys = []string{"authed_user"}
				cfg.ValidateURL = ""
			},
			DB: db,
		})
		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Force a refresh
		link.OAuthExpiry = expired
		updated, err := config.RefreshToken(ctx, db, link)

		require.NoError(t, err)
		require.True(t, updated.OAuthExtra.Valid)
		extra := map[string]interface{}{}
		require.NoError(t, json.Unmarshal(updated.OAuthExtra.RawMessage, &extra))

		mapping, ok := extra["authed_user"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, updated.OAuthAccessToken, mapping["access_token"])
	})
}
func TestExchangeWithClientSecret(t *testing.T) {
	t.Parallel()
	instrument := promoauth.NewFactory(prometheus.NewRegistry())
	// This ensures a provider that requires the custom
	// client secret exchange works.
	configs, err := externalauth.ConvertConfig(instrument, []codersdk.ExternalAuthConfig{{
		// JFrog just happens to require this custom type.
		Type:         codersdk.EnhancedExternalAuthProviderJFrog.String(),
		ClientID:     "id",

		ClientSecret: "secret",
	}}, &url.URL{})
	require.NoError(t, err)

	config := configs[0]
	client := &http.Client{
		Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "Bearer secret", req.Header.Get("Authorization"))
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusOK)
			body, err := json.Marshal(&oauth2.Token{
				AccessToken: "bananas",
			})
			if err != nil {
				return nil, err
			}
			_, err = rec.Write(body)
			return rec.Result(), err
		}),
	}
	_, err = config.Exchange(context.WithValue(context.Background(), oauth2.HTTPClient, client), "code")

	require.NoError(t, err)
}
func TestConvertYAML(t *testing.T) {
	t.Parallel()

	instrument := promoauth.NewFactory(prometheus.NewRegistry())
	for _, tc := range []struct {
		Name   string

		Input  []codersdk.ExternalAuthConfig
		Output []*externalauth.Config
		Error  string
	}{{
		Name: "InvalidID",
		Input: []codersdk.ExternalAuthConfig{{
			Type: string(codersdk.EnhancedExternalAuthProviderGitHub),
			ID:   "$hi$",
		}},

		Error: "doesn't have a valid id",
	}, {
		Name: "NoClientID",
		Input: []codersdk.ExternalAuthConfig{{
			Type: string(codersdk.EnhancedExternalAuthProviderGitHub),
		}},
		Error: "client_id must be provided",
	}, {

		Name: "DuplicateType",
		Input: []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitHub),
			ClientID:     "example",
			ClientSecret: "example",
		}, {
			Type:         string(codersdk.EnhancedExternalAuthProviderGitHub),

			ClientID:     "example-2",
			ClientSecret: "example-2",
		}},
		Error: "multiple github external auth providers provided",
	}, {
		Name: "InvalidRegex",
		Input: []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitHub),
			ClientID:     "example",
			ClientSecret: "example",
			Regex:        `\K`,
		}},
		Error: "compile regex for external auth provider",
	}, {
		Name: "NoDeviceURL",
		Input: []codersdk.ExternalAuthConfig{{

			Type:         string(codersdk.EnhancedExternalAuthProviderGitLab),
			ClientID:     "example",
			ClientSecret: "example",
			DeviceFlow:   true,

		}},
		Error: "device auth url must be provided",
	}} {

		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			output, err := externalauth.ConvertConfig(instrument, tc.Input, &url.URL{})
			if tc.Error != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.Error)
				return
			}
			require.Equal(t, tc.Output, output)
		})
	}
	t.Run("CustomScopesAndEndpoint", func(t *testing.T) {
		t.Parallel()
		config, err := externalauth.ConvertConfig(instrument, []codersdk.ExternalAuthConfig{{
			Type:         string(codersdk.EnhancedExternalAuthProviderGitLab),
			ClientID:     "id",
			ClientSecret: "secret",
			AuthURL:      "https://auth.com",
			TokenURL:     "https://token.com",
			Scopes:       []string{"read"},
		}}, &url.URL{})
		require.NoError(t, err)
		require.Equal(t, "https://auth.com?client_id=id&redirect_uri=%2Fexternal-auth%2Fgitlab%2Fcallback&response_type=code&scope=read", config[0].AuthCodeURL(""))
	})
}
// TestConstantQueryParams verifies a constant query parameter can be set in the
// "authenticate" url for external auth applications, and it will be carried forward
// to actual auth requests.
// This unit test was specifically created for Auth0 which can set an
// audience query parameter in it's /authorize endpoint.
func TestConstantQueryParams(t *testing.T) {
	t.Parallel()
	const constantQueryParamKey = "audience"
	const constantQueryParamValue = "foobar"
	constantQueryParam := fmt.Sprintf("%s=%s", constantQueryParamKey, constantQueryParamValue)
	fake, config, _ := setupOauth2Test(t, testConfig{
		FakeIDPOpts: []oidctest.FakeIDPOpt{
			oidctest.WithMiddlewares(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if strings.Contains(request.URL.Path, "authorize") {
						// Assert has the audience query param
						assert.Equal(t, request.URL.Query().Get(constantQueryParamKey), constantQueryParamValue)
					}
					next.ServeHTTP(writer, request)
				})
			}),
		},
		CoderOIDCConfigOpts: []func(cfg *coderd.OIDCConfig){
			func(cfg *coderd.OIDCConfig) {
				// Include a constant query parameter.
				authURL, err := url.Parse(cfg.OAuth2Config.(*oauth2.Config).Endpoint.AuthURL)
				require.NoError(t, err)
				authURL.RawQuery = url.Values{constantQueryParamKey: []string{constantQueryParamValue}}.Encode()
				cfg.OAuth2Config.(*oauth2.Config).Endpoint.AuthURL = authURL.String()
				require.Contains(t, cfg.OAuth2Config.(*oauth2.Config).Endpoint.AuthURL, constantQueryParam)
			},
		},
	})
	callbackCalled := false
	fake.SetCoderdCallbackHandler(func(writer http.ResponseWriter, request *http.Request) {
		// Just record the callback was hit, and the auth succeeded.
		callbackCalled = true

	})
	// Verify the AuthURL endpoint contains the constant query parameter and is a valid URL.
	// It should look something like:
	//	http://127.0.0.1:<port>>/oauth2/authorize?
	//		audience=foobar&
	//		client_id=d<uuid>&
	//		redirect_uri=<redirect>&
	//		response_type=code&
	//		scope=openid+email+profile&
	//		state=state
	const state = "state"
	rawAuthURL := config.AuthCodeURL(state)
	// Parsing the url is not perfect. It allows imperfections like the query
	// params having 2 question marks '?a=foo?b=bar'.
	// So use it to validate, then verify the raw url is as expected.

	authURL, err := url.Parse(rawAuthURL)
	require.NoError(t, err)
	require.Equal(t, authURL.Query().Get(constantQueryParamKey), constantQueryParamValue)
	// We are not using a real server, so it fakes https://coder.com
	require.Equal(t, authURL.Scheme, "https")
	// Validate the raw URL.
	// Double check only 1 '?' exists. Url parsing allows multiple '?' in the query string.
	require.Equal(t, strings.Count(rawAuthURL, "?"), 1)
	// Actually run an auth request. Although it says OIDC, the flow is the same
	// for oauth2.
	//nolint:bodyclose
	resp := fake.OIDCCallback(t, state, jwt.MapClaims{})
	require.True(t, callbackCalled)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
type testConfig struct {
	FakeIDPOpts         []oidctest.FakeIDPOpt
	CoderOIDCConfigOpts []func(cfg *coderd.OIDCConfig)
	ExternalAuthOpt     func(cfg *externalauth.Config)
	// If DB is passed in, the link will be inserted into the DB.
	DB database.Store
}
// setupTest will configure a fake IDP and a externalauth.Config for testing.
// The Fake's userinfo endpoint is used for validating tokens.
// No http servers are started so use the fake IDP's HTTPClient to make requests.
// The returned token is a fully valid token for the IDP. Feel free to manipulate it
// to test different scenarios.
func setupOauth2Test(t *testing.T, settings testConfig) (*oidctest.FakeIDP, *externalauth.Config, database.ExternalAuthLink) {

	t.Helper()
	if settings.ExternalAuthOpt == nil {
		settings.ExternalAuthOpt = func(_ *externalauth.Config) {}
	}
	const providerID = "test-idp"
	fake := oidctest.NewFakeIDP(t,
		append([]oidctest.FakeIDPOpt{}, settings.FakeIDPOpts...)...,

	)
	f := promoauth.NewFactory(prometheus.NewRegistry())
	config := &externalauth.Config{
		InstrumentedOAuth2Config: f.New("test-oauth2",
			fake.OIDCConfig(t, nil, settings.CoderOIDCConfigOpts...)),
		ID:          providerID,

		ValidateURL: fake.WellknownConfig().UserInfoURL,
	}
	settings.ExternalAuthOpt(config)
	oauthToken, err := fake.GenerateAuthenticatedToken(jwt.MapClaims{
		"email": "test@coder.com",
	})
	require.NoError(t, err)
	now := time.Now()
	link := database.ExternalAuthLink{
		ProviderID:        providerID,
		UserID:            uuid.New(),
		CreatedAt:         now,
		UpdatedAt:         now,
		OAuthAccessToken:  oauthToken.AccessToken,
		OAuthRefreshToken: oauthToken.RefreshToken,
		// The caller can manually expire this if they want.
		OAuthExpiry: now.Add(time.Hour),
	}
	if settings.DB != nil {
		// Feel free to insert additional things like the user, etc if required.
		link, err = settings.DB.InsertExternalAuthLink(context.Background(), database.InsertExternalAuthLinkParams{
			ProviderID:        link.ProviderID,
			UserID:            link.UserID,

			CreatedAt:         link.CreatedAt,
			UpdatedAt:         link.UpdatedAt,
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
		})
		require.NoError(t, err, "failed to insert link into DB")
	}

	return fake, config, link
}
type roundTripper func(req *http.Request) (*http.Response, error)
func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}
