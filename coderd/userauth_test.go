package coderd_test

import (
	"context"
	"crypto"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
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
		AccessToken:  "token",
		RefreshToken: "refresh",
		Expiry:       database.Now().Add(time.Hour),
	}, nil
}

func (o *oauth2Config) TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource {
	return &oauth2TokenSource{
		token: o.token,
	}
}

type oauth2TokenSource struct {
	token *oauth2.Token
}

func (o *oauth2TokenSource) Token() (*oauth2.Token, error) {
	if o.token != nil {
		return o.token, nil
	}
	return &oauth2.Token{
		AccessToken:  "token",
		RefreshToken: "refresh",
		Expiry:       database.Now().Add(time.Hour),
	}, nil
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
		require.True(t, methods.Password.Enabled)
		require.False(t, methods.Github.Enabled)
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
		require.True(t, methods.Password.Enabled)
		require.True(t, methods.Github.Enabled)
	})
}

// nolint:bodyclose
func TestUserOAuth2Github(t *testing.T) {
	t.Parallel()

	stateActive := "active"
	statePending := "pending"

	t.Run("NotInAllowedOrganization", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config: &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
						Organization: &github.Organization{
							Login: github.String("kyle"),
						},
					}}, nil
				},
			},
		})
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("NotInAllowedTeam", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowOrganizations: []string{"coder"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"another", "something"}, {"coder", "frontend"}},
				OAuth2Config:       &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
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
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("UnverifiedEmail", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &oauth2Config{},
				AllowOrganizations: []string{"coder"},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
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
		numLogs := len(auditor.AuditLogs)

		_ = coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for user create

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("BlockSignups", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &oauth2Config{},
				AllowOrganizations: []string{"coder"},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
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
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("MultiLoginNotAllowed", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &oauth2Config{},
				AllowOrganizations: []string{"coder"},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
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
		numLogs := len(auditor.AuditLogs)

		// Creates the first user with login_type 'password'.
		_ = coderdtest.CreateFirstUser(t, client)
		numLogs++ // add an audit log for user create

		// Attempting to login should give us a 403 since the user
		// already has a login_type of 'password'.
		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("Signup", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &oauth2Config{},
				AllowOrganizations: []string{"coder"},
				AllowSignups:       true,
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
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
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "kyle@coder.com", user.Email)
		require.Equal(t, "kyle", user.Username)
		require.Equal(t, "/hello-world", user.AvatarURL)

		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("SignupAllowedTeam", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:       true,
				AllowOrganizations: []string{"coder"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"coder", "frontend"}},
				OAuth2Config:       &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
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
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("SignupAllowedTeamInFirstOrganization", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:       true,
				AllowOrganizations: []string{"coder", "nil"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"coder", "backend"}},
				OAuth2Config:       &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{
						{
							State: &stateActive,
							Organization: &github.Organization{
								Login: github.String("coder"),
							},
						},
						{
							State: &stateActive,
							Organization: &github.Organization{
								Login: github.String("nil"),
							},
						},
					}, nil
				},
				TeamMembership: func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error) {
					return &github.Membership{}, nil
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{
						Login: github.String("mathias"),
					}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("mathias@coder.com"),
						Verified: github.Bool(true),
						Primary:  github.Bool(true),
					}}, nil
				},
			},
		})
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("SignupAllowedTeamInSecondOrganization", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:       true,
				AllowOrganizations: []string{"coder", "nil"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"nil", "null"}},
				OAuth2Config:       &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{
						{
							State: &stateActive,
							Organization: &github.Organization{
								Login: github.String("coder"),
							},
						},
						{
							State: &stateActive,
							Organization: &github.Organization{
								Login: github.String("nil"),
							},
						},
					}, nil
				},
				TeamMembership: func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error) {
					return &github.Membership{}, nil
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{
						Login: github.String("mathias"),
					}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("mathias@coder.com"),
						Verified: github.Bool(true),
						Primary:  github.Bool(true),
					}}, nil
				},
			},
		})
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("SignupAllowEveryone", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:  true,
				AllowEveryone: true,
				OAuth2Config:  &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{}, nil
				},
				TeamMembership: func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error) {
					return nil, xerrors.New("no teams")
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{
						Login: github.String("mathias"),
					}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("mathias@coder.com"),
						Verified: github.Bool(true),
						Primary:  github.Bool(true),
					}}, nil
				},
			},
		})
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
	t.Run("SignupFailedInactiveInOrg", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:       true,
				AllowOrganizations: []string{"coder"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"coder", "frontend"}},
				OAuth2Config:       &oauth2Config{},
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &statePending,
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
		numLogs := len(auditor.AuditLogs)

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
}

// nolint:bodyclose
func TestUserOIDC(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name                string
		IDTokenClaims       jwt.MapClaims
		UserInfoClaims      jwt.MapClaims
		AllowSignups        bool
		EmailDomain         []string
		Username            string
		AvatarURL           string
		StatusCode          int
		IgnoreEmailVerified bool
	}{{
		Name: "EmailOnly",
		IDTokenClaims: jwt.MapClaims{
			"email": "kyle@kwc.io",
		},
		AllowSignups: true,
		StatusCode:   http.StatusTemporaryRedirect,
		Username:     "kyle",
	}, {
		Name: "EmailNotVerified",
		IDTokenClaims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": false,
		},
		AllowSignups: true,
		StatusCode:   http.StatusForbidden,
	}, {
		Name: "EmailNotAString",
		IDTokenClaims: jwt.MapClaims{
			"email":          3.14159,
			"email_verified": false,
		},
		AllowSignups: true,
		StatusCode:   http.StatusBadRequest,
	}, {
		Name: "EmailNotVerifiedIgnored",
		IDTokenClaims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": false,
		},
		AllowSignups:        true,
		StatusCode:          http.StatusTemporaryRedirect,
		Username:            "kyle",
		IgnoreEmailVerified: true,
	}, {
		Name: "NotInRequiredEmailDomain",
		IDTokenClaims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": true,
		},
		AllowSignups: true,
		EmailDomain: []string{
			"coder.com",
		},
		StatusCode: http.StatusForbidden,
	}, {
		Name: "EmailDomainCaseInsensitive",
		IDTokenClaims: jwt.MapClaims{
			"email":          "kyle@KWC.io",
			"email_verified": true,
		},
		AllowSignups: true,
		EmailDomain: []string{
			"kwc.io",
		},
		StatusCode: http.StatusTemporaryRedirect,
	}, {
		Name:          "EmptyClaims",
		IDTokenClaims: jwt.MapClaims{},
		AllowSignups:  true,
		StatusCode:    http.StatusBadRequest,
	}, {
		Name: "NoSignups",
		IDTokenClaims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": true,
		},
		StatusCode: http.StatusForbidden,
	}, {
		Name: "UsernameFromEmail",
		IDTokenClaims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": true,
		},
		Username:     "kyle",
		AllowSignups: true,
		StatusCode:   http.StatusTemporaryRedirect,
	}, {
		Name: "UsernameFromClaims",
		IDTokenClaims: jwt.MapClaims{
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
		IDTokenClaims: jwt.MapClaims{
			"email":              "kyle@kwc.io",
			"email_verified":     true,
			"preferred_username": "kyle@kwc.io",
		},
		Username:     "kyle",
		AllowSignups: true,
		StatusCode:   http.StatusTemporaryRedirect,
	}, {
		// See: https://github.com/coder/coder/issues/4472
		Name: "UsernameIsEmail",
		IDTokenClaims: jwt.MapClaims{
			"preferred_username": "kyle@kwc.io",
		},
		Username:     "kyle",
		AllowSignups: true,
		StatusCode:   http.StatusTemporaryRedirect,
	}, {
		Name: "WithPicture",
		IDTokenClaims: jwt.MapClaims{
			"email":              "kyle@kwc.io",
			"email_verified":     true,
			"preferred_username": "kyle",
			"picture":            "/example.png",
		},
		Username:     "kyle",
		AllowSignups: true,
		AvatarURL:    "/example.png",
		StatusCode:   http.StatusTemporaryRedirect,
	}, {
		Name: "WithUserInfoClaims",
		IDTokenClaims: jwt.MapClaims{
			"email":          "kyle@kwc.io",
			"email_verified": true,
		},
		UserInfoClaims: jwt.MapClaims{
			"preferred_username": "potato",
			"picture":            "/example.png",
		},
		Username:     "potato",
		AllowSignups: true,
		AvatarURL:    "/example.png",
		StatusCode:   http.StatusTemporaryRedirect,
	}, {
		Name: "GroupsDoesNothing",
		IDTokenClaims: jwt.MapClaims{
			"email":  "coolin@coder.com",
			"groups": []string{"pingpong"},
		},
		AllowSignups: true,
		StatusCode:   http.StatusTemporaryRedirect,
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			auditor := audit.NewMock()
			conf := coderdtest.NewOIDCConfig(t, "")

			config := conf.OIDCConfig(t, tc.UserInfoClaims)
			config.AllowSignups = tc.AllowSignups
			config.EmailDomain = tc.EmailDomain
			config.IgnoreEmailVerified = tc.IgnoreEmailVerified

			client := coderdtest.New(t, &coderdtest.Options{
				Auditor:    auditor,
				OIDCConfig: config,
			})
			numLogs := len(auditor.AuditLogs)

			resp := oidcCallback(t, client, conf.EncodeClaims(t, tc.IDTokenClaims))
			numLogs++ // add an audit log for login
			assert.Equal(t, tc.StatusCode, resp.StatusCode)

			ctx, _ := testutil.Context(t)

			if tc.Username != "" {
				client.SetSessionToken(authCookieValue(resp.Cookies()))
				user, err := client.User(ctx, "me")
				require.NoError(t, err)
				require.Equal(t, tc.Username, user.Username)

				require.Len(t, auditor.AuditLogs, numLogs)
				require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
			}

			if tc.AvatarURL != "" {
				client.SetSessionToken(authCookieValue(resp.Cookies()))
				user, err := client.User(ctx, "me")
				require.NoError(t, err)
				require.Equal(t, tc.AvatarURL, user.AvatarURL)

				require.Len(t, auditor.AuditLogs, numLogs)
				require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
			}
		})
	}

	t.Run("AlternateUsername", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		conf := coderdtest.NewOIDCConfig(t, "")

		config := conf.OIDCConfig(t, nil)
		config.AllowSignups = true

		client := coderdtest.New(t, &coderdtest.Options{
			Auditor:    auditor,
			OIDCConfig: config,
		})
		numLogs := len(auditor.AuditLogs)

		code := conf.EncodeClaims(t, jwt.MapClaims{
			"email": "jon@coder.com",
		})
		resp := oidcCallback(t, client, code)
		numLogs++ // add an audit log for login

		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		ctx, _ := testutil.Context(t)

		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(ctx, "me")
		require.NoError(t, err)
		require.Equal(t, "jon", user.Username)

		// Pass a different subject field so that we prompt creating a
		// new user.
		code = conf.EncodeClaims(t, jwt.MapClaims{
			"email": "jon@example2.com",
			"sub":   "diff",
		})
		resp = oidcCallback(t, client, code)
		numLogs++ // add an audit log for login

		assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err = client.User(ctx, "me")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(user.Username, "jon-"), "username %q should have prefix %q", user.Username, "jon-")

		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		resp := oidcCallback(t, client, "asdf")
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("NoIDToken", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			OIDCConfig: &coderd.OIDCConfig{
				OAuth2Config: &oauth2Config{},
			},
		})
		numLogs := len(auditor.AuditLogs)

		resp := oidcCallback(t, client, "asdf")
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})

	t.Run("BadVerify", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		verifier := oidc.NewVerifier("", &oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{},
		}, &oidc.Config{})
		provider := &oidc.Provider{}

		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			OIDCConfig: &coderd.OIDCConfig{
				OAuth2Config: &oauth2Config{
					token: (&oauth2.Token{
						AccessToken: "token",
					}).WithExtra(map[string]interface{}{
						"id_token": "invalid",
					}),
				},
				Provider: provider,
				Verifier: verifier,
			},
		})
		numLogs := len(auditor.AuditLogs)

		resp := oidcCallback(t, client, "asdf")
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Len(t, auditor.AuditLogs, numLogs)
		require.Equal(t, database.AuditActionLogin, auditor.AuditLogs[numLogs-1].Action)
	})
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
		Name:  codersdk.OAuth2StateCookie,
		Value: state,
	})
	res, err := client.HTTPClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = res.Body.Close()
	})
	return res
}

func oidcCallback(t *testing.T, client *codersdk.Client, code string) *http.Response {
	t.Helper()
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	oauthURL, err := client.URL.Parse(fmt.Sprintf("/api/v2/users/oidc/callback?code=%s&state=somestate", code))
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{
		Name:  codersdk.OAuth2StateCookie,
		Value: "somestate",
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

func authCookieValue(cookies []*http.Cookie) string {
	for _, cookie := range cookies {
		if cookie.Name == codersdk.SessionTokenCookie {
			return cookie.Value
		}
	}
	return ""
}
