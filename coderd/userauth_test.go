package coderd_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt"
	"github.com/google/go-github/v43/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

type oauth2Config struct {
	token *oauth2.Token
}

func (*oauth2Config) AuthCodeURL(state string, _ ...oauth2.AuthCodeOption) string {
	return "/?state=" + url.QueryEscape(state)
}

func (o *oauth2Config) Exchange(context.Context, string, ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	if o.token != nil {
		return o.token, nil
	}
	return &oauth2.Token{
		AccessToken: "token",
	}, nil
}

func (*oauth2Config) TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource {
	return nil
}

func TestUserAuthMethods(t *testing.T) {
	t.Parallel()
	t.Run("Password", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		methods, err := client.AuthMethods(ctx)
		require.NoError(t, err)
		require.True(t, methods.Password)
		require.False(t, methods.Github)
	})
	t.Run("Github", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{},
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		methods, err := client.AuthMethods(ctx)
		require.NoError(t, err)
		require.True(t, methods.Password)
		require.True(t, methods.Github)
	})
}

// nolint:bodyclose
func TestUserOAuth2Github(t *testing.T) {
	t.Parallel()
	t.Run("NotInAllowedOrganization", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config: &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						Organization: &github.Organization{
							Login: github.String("kyle"),
						},
					}}, nil
				},
			},
		})

		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
	t.Run("NotInAllowedTeam", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowOrganizations: []string{"coder"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"another", "something"}, {"coder", "frontend"}},
				OAuth2Config:       &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						Organization: &github.Organization{
							Login: github.String("coder"),
						},
					}}, nil
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{
						Login: github.String("kyle"),
					}, nil
				},
				TeamMembership: func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error) {
					return nil, xerrors.New("no perms")
				},
			},
		})
		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
	t.Run("UnverifiedEmail", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &oauth2Config{},
				AllowOrganizations: []string{"coder"},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						Organization: &github.Organization{
							Login: github.String("coder"),
						},
					}}, nil
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("testuser@coder.com"),
						Verified: github.Bool(false),
					}}, nil
				},
			},
		})
		_ = coderdtest.CreateFirstUser(t, client)
		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusPreconditionRequired, resp.StatusCode)
	})
	t.Run("BlockSignups", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &oauth2Config{},
				AllowOrganizations: []string{"coder"},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						Organization: &github.Organization{
							Login: github.String("coder"),
						},
					}}, nil
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("testuser@coder.com"),
						Verified: github.Bool(true),
						Primary:  github.Bool(true),
					}}, nil
				},
			},
		})
		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
	t.Run("MultiLoginNotAllowed", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &oauth2Config{},
				AllowOrganizations: []string{"coder"},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						Organization: &github.Organization{
							Login: github.String("coder"),
						},
					}}, nil
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("testuser@coder.com"),
						Verified: github.Bool(true),
						Primary:  github.Bool(true),
					}}, nil
				},
			},
		})
		// Creates the first user with login_type 'password'.
		_ = coderdtest.CreateFirstUser(t, client)
		// Attempting to login should give us a 403 since the user
		// already has a login_type of 'password'.
		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
	t.Run("Signup", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &oauth2Config{},
				AllowOrganizations: []string{"coder"},
				AllowSignups:       true,
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						Organization: &github.Organization{
							Login: github.String("coder"),
						},
					}}, nil
				},
				AuthenticatedUser: func(ctx context.Context, _ *http.Client) (*github.User, error) {
					return &github.User{
						Login:     github.String("kyle"),
						ID:        i64ptr(1234),
						AvatarURL: github.String("/hello-world"),
					}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("kyle@coder.com"),
						Verified: github.Bool(true),
						Primary:  github.Bool(true),
					}}, nil
				},
			},
		})
		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		client.SessionToken = resp.Cookies()[0].Value
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "kyle@coder.com", user.Email)
		require.Equal(t, "kyle", user.Username)
		require.Equal(t, "/hello-world", user.AvatarURL)
	})
	t.Run("SignupAllowedTeam", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:       true,
				AllowOrganizations: []string{"coder"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"coder", "frontend"}},
				OAuth2Config:       &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						Organization: &github.Organization{
							Login: github.String("coder"),
						},
					}}, nil
				},
				TeamMembership: func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error) {
					return &github.Membership{}, nil
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{
						Login: github.String("kyle"),
					}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("kyle@coder.com"),
						Verified: github.Bool(true),
						Primary:  github.Bool(true),
					}}, nil
				},
			},
		})
		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	})
}

// nolint:bodyclose
func TestUserOIDC(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name         string
		Claims       jwt.MapClaims
		AllowSignups bool
		EmailDomain  string
		Username     string
		AvatarURL    string
		StatusCode   int
	}{{
		Name: "EmailNotVerified",
		Claims: jwt.MapClaims{
			"email": "kyle@kwc.io",
		},
		AllowSignups: true,
		StatusCode:   http.StatusForbidden,
	}, {
		Name: "NotInRequiredEmailDomain",
		Claims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": true,
		},
		AllowSignups: true,
		EmailDomain:  "coder.com",
		StatusCode:   http.StatusForbidden,
	}, {
		Name:         "EmptyClaims",
		Claims:       jwt.MapClaims{},
		AllowSignups: true,
		StatusCode:   http.StatusBadRequest,
	}, {
		Name: "NoSignups",
		Claims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": true,
		},
		StatusCode: http.StatusForbidden,
	}, {
		Name: "UsernameFromEmail",
		Claims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": true,
		},
		Username:     "kyle",
		AllowSignups: true,
		StatusCode:   http.StatusTemporaryRedirect,
	}, {
		Name: "UsernameFromClaims",
		Claims: jwt.MapClaims{
			"email":              "kyle@kwc.io",
			"email_verified":     true,
			"preferred_username": "hotdog",
		},
		Username:     "hotdog",
		AllowSignups: true,
		StatusCode:   http.StatusTemporaryRedirect,
	}, {
		// Services like Okta return the email as the username:
		// https://developer.okta.com/docs/reference/api/oidc/#base-claims-always-present
		Name: "UsernameAsEmail",
		Claims: jwt.MapClaims{
			"email":              "kyle@kwc.io",
			"email_verified":     true,
			"preferred_username": "kyle@kwc.io",
		},
		Username:     "kyle",
		AllowSignups: true,
		StatusCode:   http.StatusTemporaryRedirect,
	}, {
		Name: "WithPicture",
		Claims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": true,
			"username":       "kyle",
			"picture":        "/example.png",
		},
		Username:     "kyle",
		AllowSignups: true,
		AvatarURL:    "/example.png",
		StatusCode:   http.StatusTemporaryRedirect,
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			config := createOIDCConfig(t, tc.Claims)
			config.AllowSignups = tc.AllowSignups
			config.EmailDomain = tc.EmailDomain
			client := coderdtest.New(t, &coderdtest.Options{
				OIDCConfig: config,
			})
			resp := oidcCallback(t, client)
			assert.Equal(t, tc.StatusCode, resp.StatusCode)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			if tc.Username != "" {
				client.SessionToken = resp.Cookies()[0].Value
				user, err := client.User(ctx, "me")
				require.NoError(t, err)
				require.Equal(t, tc.Username, user.Username)
			}

			if tc.AvatarURL != "" {
				client.SessionToken = resp.Cookies()[0].Value
				user, err := client.User(ctx, "me")
				require.NoError(t, err)
				require.Equal(t, tc.AvatarURL, user.AvatarURL)
			}
		})
	}

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		resp := oidcCallback(t, client)
		require.Equal(t, http.StatusPreconditionRequired, resp.StatusCode)
	})

	t.Run("NoIDToken", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			OIDCConfig: &coderd.OIDCConfig{
				OAuth2Config: &oauth2Config{},
			},
		})
		resp := oidcCallback(t, client)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("BadVerify", func(t *testing.T) {
		t.Parallel()
		verifier := oidc.NewVerifier("", &oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{},
		}, &oidc.Config{})

		client := coderdtest.New(t, &coderdtest.Options{
			OIDCConfig: &coderd.OIDCConfig{
				OAuth2Config: &oauth2Config{
					token: (&oauth2.Token{
						AccessToken: "token",
					}).WithExtra(map[string]interface{}{
						"id_token": "invalid",
					}),
				},
				Verifier: verifier,
			},
		})
		resp := oidcCallback(t, client)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// createOIDCConfig generates a new OIDCConfig that returns a static token
// with the claims provided.
func createOIDCConfig(t *testing.T, claims jwt.MapClaims) *coderd.OIDCConfig {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// https://datatracker.ietf.org/doc/html/rfc7519#section-4.1
	claims["exp"] = time.Now().Add(time.Hour).UnixMilli()
	claims["iss"] = "https://coder.com"
	claims["sub"] = "hello"

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	require.NoError(t, err)

	verifier := oidc.NewVerifier("https://coder.com", &oidc.StaticKeySet{
		PublicKeys: []crypto.PublicKey{key.Public()},
	}, &oidc.Config{
		SkipClientIDCheck: true,
	})

	return &coderd.OIDCConfig{
		OAuth2Config: &oauth2Config{
			token: (&oauth2.Token{
				AccessToken: "token",
			}).WithExtra(map[string]interface{}{
				"id_token": signed,
			}),
		},
		Verifier: verifier,
	}
}

func oauth2Callback(t *testing.T, client *codersdk.Client) *http.Response {
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	state := "somestate"
	oauthURL, err := client.URL.Parse("/api/v2/users/oauth2/github/callback?code=asd&state=" + state)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{
		Name:  codersdk.OAuth2StateKey,
		Value: state,
	})
	res, err := client.HTTPClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = res.Body.Close()
	})
	return res
}

func oidcCallback(t *testing.T, client *codersdk.Client) *http.Response {
	t.Helper()
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	state := "somestate"
	oauthURL, err := client.URL.Parse("/api/v2/users/oidc/callback?code=asd&state=" + state)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{
		Name:  codersdk.OAuth2StateKey,
		Value: state,
	})
	res, err := client.HTTPClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	t.Log(string(data))
	return res
}

func i64ptr(i int64) *int64 {
	return &i
}
