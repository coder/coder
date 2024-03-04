package coderd_test

import (
	"context"
	"crypto"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// This test specifically tests logging in with OIDC when an expired
// OIDC session token exists.
// The token refreshing should not happen since we are reauthenticating.
// nolint:bodyclose
func TestOIDCOauthLoginWithExisting(t *testing.T) {
	t.Parallel()

	fake := oidctest.NewFakeIDP(t,
		oidctest.WithRefresh(func(_ string) error {
			return xerrors.New("refreshing token should never occur")
		}),
		oidctest.WithServing(),
	)

	cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
		cfg.AllowSignups = true
		cfg.IgnoreUserInfo = true
	})

	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		OIDCConfig: cfg,
	})

	const username = "alice"
	claims := jwt.MapClaims{
		"email":              "alice@coder.com",
		"email_verified":     true,
		"preferred_username": username,
	}

	helper := oidctest.NewLoginHelper(client, fake)
	// Signup alice
	userClient, _ := helper.Login(t, claims)

	// Expire the link. This will force the client to refresh the token.
	helper.ExpireOauthToken(t, api.Database, userClient)

	// Instead of refreshing, just log in again.
	helper.Login(t, claims)
}

func TestUserLogin(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, err := anotherClient.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    anotherUser.Email,
			Password: "SomeSecurePassword!",
		})
		require.NoError(t, err)
	})
	t.Run("UserDeleted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		client.DeleteUser(context.Background(), anotherUser.ID)
		_, err := anotherClient.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    anotherUser.Email,
			Password: "SomeSecurePassword!",
		})
		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
	})
	// Password auth should fail if the user is made without password login.
	t.Run("DisableLoginDeprecatedField", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		anotherClient, anotherUser := coderdtest.CreateAnotherUserMutators(t, client, user.OrganizationID, nil, func(r *codersdk.CreateUserRequest) {
			r.Password = ""
			r.DisableLogin = true
		})

		_, err := anotherClient.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    anotherUser.Email,
			Password: "SomeSecurePassword!",
		})
		require.Error(t, err)
	})

	t.Run("LoginTypeNone", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		anotherClient, anotherUser := coderdtest.CreateAnotherUserMutators(t, client, user.OrganizationID, nil, func(r *codersdk.CreateUserRequest) {
			r.Password = ""
			r.UserLoginType = codersdk.LoginTypeNone
		})

		_, err := anotherClient.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    anotherUser.Email,
			Password: "SomeSecurePassword!",
		})
		require.Error(t, err)
	})
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
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config: &testutil.OAuth2Config{},
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

		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
	t.Run("NotInAllowedTeam", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowOrganizations: []string{"coder"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"another", "something"}, {"coder", "frontend"}},
				OAuth2Config:       &testutil.OAuth2Config{},
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

		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
	t.Run("UnverifiedEmail", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &testutil.OAuth2Config{},
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

		_ = coderdtest.CreateFirstUser(t, client)

		resp := oauth2Callback(t, client)

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
	t.Run("BlockSignups", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &testutil.OAuth2Config{},
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

		resp := oauth2Callback(t, client)

		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
	t.Run("MultiLoginNotAllowed", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &testutil.OAuth2Config{},
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

		// Creates the first user with login_type 'password'.
		_ = coderdtest.CreateFirstUser(t, client)

		// Attempting to login should give us a 403 since the user
		// already has a login_type of 'password'.
		resp := oauth2Callback(t, client)

		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
	t.Run("Signup", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &testutil.OAuth2Config{},
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
		numLogs := len(auditor.AuditLogs())

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "kyle@coder.com", user.Email)
		require.Equal(t, "kyle", user.Username)
		require.Equal(t, "/hello-world", user.AvatarURL)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.NotEqual(t, auditor.AuditLogs()[numLogs-1].UserID, uuid.Nil)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
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
				OAuth2Config:       &testutil.OAuth2Config{},
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
		numLogs := len(auditor.AuditLogs())

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
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
				OAuth2Config:       &testutil.OAuth2Config{},
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
		numLogs := len(auditor.AuditLogs())

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
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
				OAuth2Config:       &testutil.OAuth2Config{},
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
		numLogs := len(auditor.AuditLogs())

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
	})
	t.Run("SignupAllowEveryone", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:  true,
				AllowEveryone: true,
				OAuth2Config:  &testutil.OAuth2Config{},
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
		numLogs := len(auditor.AuditLogs())

		resp := oauth2Callback(t, client)
		numLogs++ // add an audit log for login

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
	})
	t.Run("SignupFailedInactiveInOrg", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:       true,
				AllowOrganizations: []string{"coder"},
				AllowTeams:         []coderd.GithubOAuth2Team{{"coder", "frontend"}},
				OAuth2Config:       &testutil.OAuth2Config{},
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

		resp := oauth2Callback(t, client)

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
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
		IgnoreUserInfo      bool
	}{{
		Name: "EmailOnly",
		IDTokenClaims: jwt.MapClaims{
			"email": "kyle@kwc.io",
		},
		AllowSignups: true,
		StatusCode:   http.StatusOK,
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
		StatusCode:          http.StatusOK,
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
		StatusCode: http.StatusOK,
	}, {
		Name: "EmailDomainSubset",
		IDTokenClaims: jwt.MapClaims{
			"email":          "colin@gmail.com",
			"email_verified": true,
		},
		AllowSignups: true,
		EmailDomain: []string{
			"mail.com",
		},
		StatusCode: http.StatusForbidden,
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
		StatusCode:   http.StatusOK,
	}, {
		Name: "UsernameFromClaims",
		IDTokenClaims: jwt.MapClaims{
			"email":              "kyle@kwc.io",
			"email_verified":     true,
			"preferred_username": "hotdog",
		},
		Username:     "hotdog",
		AllowSignups: true,
		StatusCode:   http.StatusOK,
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
		StatusCode:   http.StatusOK,
	}, {
		// See: https://github.com/coder/coder/issues/4472
		Name: "UsernameIsEmail",
		IDTokenClaims: jwt.MapClaims{
			"preferred_username": "kyle@kwc.io",
		},
		Username:     "kyle",
		AllowSignups: true,
		StatusCode:   http.StatusOK,
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
		StatusCode:   http.StatusOK,
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
		StatusCode:   http.StatusOK,
	}, {
		Name: "GroupsDoesNothing",
		IDTokenClaims: jwt.MapClaims{
			"email":  "coolin@coder.com",
			"groups": []string{"pingpong"},
		},
		AllowSignups: true,
		StatusCode:   http.StatusOK,
	}, {
		Name: "UserInfoOverridesIDTokenClaims",
		IDTokenClaims: jwt.MapClaims{
			"email":          "internaluser@internal.domain",
			"email_verified": false,
		},
		UserInfoClaims: jwt.MapClaims{
			"email":              "externaluser@external.domain",
			"email_verified":     true,
			"preferred_username": "user",
		},
		Username:            "user",
		AllowSignups:        true,
		IgnoreEmailVerified: false,
		StatusCode:          http.StatusOK,
	}, {
		Name: "InvalidUserInfo",
		IDTokenClaims: jwt.MapClaims{
			"email":          "internaluser@internal.domain",
			"email_verified": false,
		},
		UserInfoClaims: jwt.MapClaims{
			"email": 1,
		},
		AllowSignups:        true,
		IgnoreEmailVerified: false,
		StatusCode:          http.StatusInternalServerError,
	}, {
		Name: "IgnoreUserInfo",
		IDTokenClaims: jwt.MapClaims{
			"email":              "user@internal.domain",
			"email_verified":     true,
			"preferred_username": "user",
		},
		UserInfoClaims: jwt.MapClaims{
			"email":              "user.mcname@external.domain",
			"preferred_username": "Mr. User McName",
		},
		Username:       "user",
		IgnoreUserInfo: true,
		AllowSignups:   true,
		StatusCode:     http.StatusOK,
	}} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			fake := oidctest.NewFakeIDP(t,
				oidctest.WithRefresh(func(_ string) error {
					return xerrors.New("refreshing token should never occur")
				}),
				oidctest.WithServing(),
				oidctest.WithStaticUserInfo(tc.UserInfoClaims),
			)
			cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
				cfg.AllowSignups = tc.AllowSignups
				cfg.EmailDomain = tc.EmailDomain
				cfg.IgnoreEmailVerified = tc.IgnoreEmailVerified
				cfg.IgnoreUserInfo = tc.IgnoreUserInfo
			})

			auditor := audit.NewMock()
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			owner := coderdtest.New(t, &coderdtest.Options{
				Auditor:    auditor,
				OIDCConfig: cfg,
				Logger:     &logger,
			})
			numLogs := len(auditor.AuditLogs())

			client, resp := fake.AttemptLogin(t, owner, tc.IDTokenClaims)
			numLogs++ // add an audit log for login
			require.Equal(t, tc.StatusCode, resp.StatusCode)

			ctx := testutil.Context(t, testutil.WaitLong)

			if tc.Username != "" {
				user, err := client.User(ctx, "me")
				require.NoError(t, err)
				require.Equal(t, tc.Username, user.Username)

				require.Len(t, auditor.AuditLogs(), numLogs)
				require.NotEqual(t, auditor.AuditLogs()[numLogs-1].UserID, uuid.Nil)
				require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
			}

			if tc.AvatarURL != "" {
				user, err := client.User(ctx, "me")
				require.NoError(t, err)
				require.Equal(t, tc.AvatarURL, user.AvatarURL)

				require.Len(t, auditor.AuditLogs(), numLogs)
				require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
			}
		})
	}

	t.Run("OIDCConvert", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		fake := oidctest.NewFakeIDP(t,
			oidctest.WithRefresh(func(_ string) error {
				return xerrors.New("refreshing token should never occur")
			}),
			oidctest.WithServing(),
		)
		cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true
		})

		client := coderdtest.New(t, &coderdtest.Options{
			Auditor:    auditor,
			OIDCConfig: cfg,
		})

		owner := coderdtest.CreateFirstUser(t, client)
		user, userData := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		claims := jwt.MapClaims{
			"email": userData.Email,
		}
		var err error
		user.HTTPClient.Jar, err = cookiejar.New(nil)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitShort)
		convertResponse, err := user.ConvertLoginType(ctx, codersdk.ConvertLoginRequest{
			ToType:   codersdk.LoginTypeOIDC,
			Password: "SomeSecurePassword!",
		})
		require.NoError(t, err)

		fake.LoginWithClient(t, user, claims, func(r *http.Request) {
			r.URL.RawQuery = url.Values{
				"oidc_merge_state": {convertResponse.StateString},
			}.Encode()
			r.Header.Set(codersdk.SessionTokenHeader, user.SessionToken())
			cookies := user.HTTPClient.Jar.Cookies(r.URL)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
		})
	})

	t.Run("AlternateUsername", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		fake := oidctest.NewFakeIDP(t,
			oidctest.WithRefresh(func(_ string) error {
				return xerrors.New("refreshing token should never occur")
			}),
			oidctest.WithServing(),
		)
		cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true
		})

		client := coderdtest.New(t, &coderdtest.Options{
			Auditor:    auditor,
			OIDCConfig: cfg,
		})

		numLogs := len(auditor.AuditLogs())
		claims := jwt.MapClaims{
			"email": "jon@coder.com",
		}

		userClient, _ := fake.Login(t, client, claims)
		numLogs++ // add an audit log for login

		ctx := testutil.Context(t, testutil.WaitLong)
		user, err := userClient.User(ctx, "me")
		require.NoError(t, err)
		require.Equal(t, "jon", user.Username)

		// Pass a different subject field so that we prompt creating a
		// new user
		userClient, _ = fake.Login(t, client, jwt.MapClaims{
			"email": "jon@example2.com",
			"sub":   "diff",
		})
		numLogs++ // add an audit log for login

		user, err = userClient.User(ctx, "me")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(user.Username, "jon-"), "username %q should have prefix %q", user.Username, "jon-")

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
	})

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		oauthURL, err := client.URL.Parse("/api/v2/users/oidc/callback")
		require.NoError(t, err)

		req, err := http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)
		require.NoError(t, err)
		resp, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("NoIDToken", func(t *testing.T) {
		t.Parallel()
		fake := oidctest.NewFakeIDP(t,
			oidctest.WithRefresh(func(_ string) error {
				return xerrors.New("refreshing token should never occur")
			}),
			oidctest.WithServing(),
		)
		cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true
		})

		client := coderdtest.New(t, &coderdtest.Options{
			OIDCConfig: cfg,
		})

		_, resp := fake.AttemptLogin(t, client, jwt.MapClaims{})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("BadVerify", func(t *testing.T) {
		t.Parallel()
		badVerifier := oidc.NewVerifier("", &oidc.StaticKeySet{
			PublicKeys: []crypto.PublicKey{},
		}, &oidc.Config{})
		badProvider := &oidc.Provider{}

		fake := oidctest.NewFakeIDP(t,
			oidctest.WithRefresh(func(_ string) error {
				return xerrors.New("refreshing token should never occur")
			}),
			oidctest.WithServing(),
		)
		cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true
			cfg.Provider = badProvider
			cfg.Verifier = badVerifier
		})

		client := coderdtest.New(t, &coderdtest.Options{
			OIDCConfig: cfg,
		})

		_, resp := fake.AttemptLogin(t, client, jwt.MapClaims{})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestUserLogout(t *testing.T) {
	t.Parallel()

	// Create a custom database so it's easier to make scoped tokens for
	// testing.
	db, pubSub := dbtestutil.NewDB(t)

	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubSub,
	})
	firstUser := coderdtest.CreateFirstUser(t, client)

	ctx := testutil.Context(t, testutil.WaitLong)

	// Create a user with built-in auth.
	const (
		email    = "dean.was.here@test.coder.com"
		username = "dean"
		//nolint:gosec
		password = "SomeSecurePassword123!"
	)
	newUser, err := client.CreateUser(ctx, codersdk.CreateUserRequest{
		Email:          email,
		Username:       username,
		Password:       password,
		OrganizationID: firstUser.OrganizationID,
	})
	require.NoError(t, err)

	// Log in with basic auth and keep the the session token (but don't use it).
	userClient := codersdk.New(client.URL)
	loginRes1, err := userClient.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    email,
		Password: password,
	})
	require.NoError(t, err)

	// Log in again but actually set the token this time.
	loginRes2, err := userClient.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
		Email:    email,
		Password: password,
	})
	require.NoError(t, err)
	userClient.SetSessionToken(loginRes2.SessionToken)

	// Add the user's second session token to the list of API keys that should
	// be deleted.
	shouldBeDeleted := map[string]string{
		"user login 2 (logging out with this)": loginRes2.SessionToken,
	}

	// Add the user's first token, and the admin's session token to the list of
	// API keys that should not be deleted.
	shouldNotBeDeleted := map[string]string{
		"user login 1 (not logging out of)": loginRes1.SessionToken,
		"admin login":                       client.SessionToken(),
	}

	// Create a few application_connect-scoped API keys that should be deleted.
	for i := 0; i < 3; i++ {
		key, _ := dbgen.APIKey(t, db, database.APIKey{
			UserID: newUser.ID,
			Scope:  database.APIKeyScopeApplicationConnect,
		})
		shouldBeDeleted[fmt.Sprintf("application_connect key owned by logout user %d", i)] = key.ID
	}

	// Create a few application_connect-scoped API keys for the admin user that
	// should not be deleted.
	for i := 0; i < 3; i++ {
		key, _ := dbgen.APIKey(t, db, database.APIKey{
			UserID: firstUser.UserID,
			Scope:  database.APIKeyScopeApplicationConnect,
		})
		shouldNotBeDeleted[fmt.Sprintf("application_connect key owned by admin user %d", i)] = key.ID
	}

	// Log out of the new user.
	err = userClient.Logout(ctx)
	require.NoError(t, err)

	// Ensure the new user's session token is no longer valid.
	_, err = userClient.User(ctx, codersdk.Me)
	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusUnauthorized, sdkErr.StatusCode())

	// Check that the deleted keys are gone.
	for name, id := range shouldBeDeleted {
		id := strings.Split(id, "-")[0]
		_, err := db.GetAPIKeyByID(ctx, id)
		require.Error(t, err, name)
	}

	// Check that the other keys are still there.
	for name, id := range shouldNotBeDeleted {
		id := strings.Split(id, "-")[0]
		_, err := db.GetAPIKeyByID(ctx, id)
		require.NoError(t, err, name)
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
