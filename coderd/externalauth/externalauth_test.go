package externalauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmem"
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
					return xerrors.New("should not be called")
				}),
				// The IDP should not be contacted since the token is expired. An expired
				// token with 'NoRefresh' should early abort.
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					t.Error("token was validated, but it was expired and this should never have happened.")
					return nil, xerrors.New("should not be called")
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.NoRefresh = true
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Expire the link
		link.OAuthExpiry = expired

		_, refreshed, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.False(t, refreshed)
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
					return xerrors.New("should not be called")
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
		_, refreshed, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.True(t, refreshed, "token without expiry is always valid")
		require.True(t, validated, "token should have been validated")
	})

	t.Run("FalseIfTokenSourceFails", func(t *testing.T) {
		t.Parallel()
		config := &externalauth.Config{
			InstrumentedOAuth2Config: &testutil.OAuth2Config{
				TokenSourceFunc: func() (*oauth2.Token, error) {
					return nil, xerrors.New("failure")
				},
			},
		}
		_, refreshed, err := config.RefreshToken(context.Background(), nil, database.ExternalAuthLink{
			OAuthExpiry: expired,
		})
		require.NoError(t, err)
		require.False(t, refreshed)
	})

	t.Run("ValidateServerError", func(t *testing.T) {
		t.Parallel()

		const staticError = "static error"
		validated := false
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validated = true
					return jwt.MapClaims{}, xerrors.New(staticError)
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		link.OAuthExpiry = expired

		_, _, err := config.RefreshToken(ctx, nil, link)
		require.ErrorContains(t, err, staticError)
		require.True(t, validated, "token should have been attempted to be validated")
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
					return jwt.MapClaims{}, oidctest.StatusError(http.StatusUnauthorized, xerrors.New(staticError))
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		link.OAuthExpiry = expired

		_, refreshed, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err, staticError)
		require.False(t, refreshed)
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
					return xerrors.New("should not be called")
				}),
				oidctest.WithDynamicUserInfo(func(_ string) (jwt.MapClaims, error) {
					validateCalls++
					// Make the first call return a 401, subsequent calls should return a 200.
					if validateCalls > 1 {
						return jwt.MapClaims{}, nil
					}
					return jwt.MapClaims{}, oidctest.StatusError(http.StatusUnauthorized, xerrors.New(staticError))
				}),
			},
			ExternalAuthOpt: func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			},
		})

		ctx := oidc.ClientContext(context.Background(), fake.HTTPClient(nil))
		// Unlimited lifetime, this is what GitHub returns tokens as
		link.OAuthExpiry = time.Time{}

		_, ok, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, 2, validateCalls, "token should have been attempted to be validated more than once")
	})

	t.Run("ValidateNoUpdate", func(t *testing.T) {
		t.Parallel()

		validateCalls := 0
		fake, config, link := setupOauth2Test(t, testConfig{
			FakeIDPOpts: []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					t.Error("refresh on the IDP was called, but the token is not expired")
					return xerrors.New("should not be called")
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

		_, ok, err := config.RefreshToken(ctx, nil, link)
		require.NoError(t, err)
		require.True(t, ok)
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

		updated, ok, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err)
		require.True(t, ok)
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

		updated, ok, err := config.RefreshToken(ctx, db, link)
		require.NoError(t, err)
		require.True(t, ok)
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
