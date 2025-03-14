package coderd_test

import (
	"errors"
	"context"
	"crypto"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"golang.org/x/oauth2"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
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
			return errors.New("refreshing token should never occur")
		}),

		oidctest.WithServing(),
	)
	cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
		cfg.AllowSignups = true
		cfg.SecondaryClaims = coderd.MergedClaimsSourceNone
	})
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{

		OIDCConfig: cfg,
	})
	const username = "alice"
	claims := jwt.MapClaims{
		"email":              "alice@coder.com",

		"email_verified":     true,
		"preferred_username": username,
		"sub":                uuid.NewString(),
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
	t.Run("LoginTypeNone", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		anotherClient, anotherUser := coderdtest.CreateAnotherUserMutators(t, client, user.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
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
						ID:    github.Int64(100),
						Login: github.String("kyle"),

						Name:  github.String("Kylium Carbonate"),
					}, nil
				},
				TeamMembership: func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error) {
					return nil, errors.New("no perms")
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
		db, ps := dbtestutil.NewDB(t)
		id := atomic.NewInt64(100)
		login := atomic.NewString("testuser")
		email := atomic.NewString("testuser@coder.com")
		client := coderdtest.New(t, &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &testutil.OAuth2Config{},
				AllowOrganizations: []string{"coder"},
				ListOrganizationMemberships: func(_ context.Context, _ *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
						Organization: &github.Organization{
							Login: github.String("coder"),
						},
					}}, nil
				},

				AuthenticatedUser: func(_ context.Context, _ *http.Client) (*github.User, error) {
					id := id.Load()

					login := login.Load()
					return &github.User{

						ID:    &id,
						Login: &login,
						Name:  github.String("The Right Honorable Sir Test McUser"),
					}, nil
				},

				ListEmails: func(_ context.Context, _ *http.Client) ([]*github.UserEmail, error) {
					email := email.Load()

					return []*github.UserEmail{{
						Email:    &email,
						Verified: github.Bool(true),
						Primary:  github.Bool(true),

					}}, nil
				},
			},
		})
		// The first user in a deployment with signups disabled will be allowed to sign up,
		// but all the other users will not.
		resp := oauth2Callback(t, client)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		ctx := testutil.Context(t, testutil.WaitLong)
		// nolint:gocritic // Unit test
		count, err := db.GetUserCount(dbauthz.AsSystemRestricted(ctx))
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
		id.Store(101)
		email.Store("someotheruser@coder.com")
		login.Store("someotheruser")
		resp = oauth2Callback(t, client)
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

					return &github.User{
						ID:    github.Int64(100),
						Login: github.String("testuser"),
						Name:  github.String("The Right Honorable Sir Test McUser"),
					}, nil

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
						AvatarURL: github.String("/hello-world"),
						ID:        i64ptr(1234),
						Login:     github.String("kyle"),
						Name:      github.String("Kylium Carbonate"),
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

		// Validate that attempting to redirect away from the
		// site does not work.
		maliciousHost := "https://malicious.com"
		expectedPath := "/my/path"
		resp := oauth2Callback(t, client, func(req *http.Request) {
			// Add the cookie to bypass the parsing in httpmw/oauth2.go
			req.AddCookie(&http.Cookie{
				Name:  codersdk.OAuth2RedirectCookie,
				Value: maliciousHost + expectedPath,
			})
		})
		numLogs++ // add an audit log for login
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		redirect, err := resp.Location()
		require.NoError(t, err)
		require.Equal(t, expectedPath, redirect.Path)
		require.Equal(t, client.URL.Host, redirect.Host)
		require.NotContains(t, redirect.String(), maliciousHost)
		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "kyle@coder.com", user.Email)
		require.Equal(t, "kyle", user.Username)
		require.Equal(t, "Kylium Carbonate", user.Name)
		require.Equal(t, "/hello-world", user.AvatarURL)
		require.Equal(t, 1, len(user.OrganizationIDs), "in the default org")
		require.Len(t, auditor.AuditLogs(), numLogs)
		require.NotEqual(t, auditor.AuditLogs()[numLogs-1].UserID, uuid.Nil)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
	})
	t.Run("SignupWeirdName", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &testutil.OAuth2Config{},
				AllowOrganizations: []string{"coder"},

				AllowSignups:       true,
				ListOrganizationMemberships: func(_ context.Context, _ *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{
						State: &stateActive,
						Organization: &github.Organization{
							Login: github.String("coder"),
						},
					}}, nil
				},
				AuthenticatedUser: func(_ context.Context, _ *http.Client) (*github.User, error) {
					return &github.User{
						AvatarURL: github.String("/hello-world"),
						ID:        i64ptr(1234),

						Login:     github.String("kyle"),
						Name:      github.String(" " + strings.Repeat("a", 129) + " "),
					}, nil
				},
				ListEmails: func(_ context.Context, _ *http.Client) ([]*github.UserEmail, error) {
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
		require.Equal(t, strings.Repeat("a", 128), user.Name)
		require.Equal(t, "/hello-world", user.AvatarURL)
		require.Equal(t, 1, len(user.OrganizationIDs), "in the default org")
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
						AvatarURL: github.String("/hello-world"),
						ID:        github.Int64(100),
						Login:     github.String("kyle"),

						Name:      github.String("Kylium Carbonate"),
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
		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "kyle@coder.com", user.Email)
		require.Equal(t, "kyle", user.Username)
		require.Equal(t, "Kylium Carbonate", user.Name)
		require.Equal(t, "/hello-world", user.AvatarURL)
		require.Equal(t, 1, len(user.OrganizationIDs), "in the default org")
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
	})
	// nolint: dupl
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
						ID:    github.Int64(100),
						Login: github.String("mathias"),

						Name:  github.String("Mathias Mathias"),
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
		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "mathias@coder.com", user.Email)
		require.Equal(t, "mathias", user.Username)
		require.Equal(t, "Mathias Mathias", user.Name)
		require.Equal(t, 1, len(user.OrganizationIDs), "in the default org")
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
	})
	// nolint: dupl
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
						ID:    github.Int64(100),
						Login: github.String("mathias"),
						Name:  github.String("Mathias Mathias"),

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
		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "mathias@coder.com", user.Email)
		require.Equal(t, "mathias", user.Username)
		require.Equal(t, "Mathias Mathias", user.Name)
		require.Equal(t, 1, len(user.OrganizationIDs), "in the default org")
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
					return nil, errors.New("no teams")
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{
						ID:    github.Int64(100),
						Login: github.String("mathias"),
						Name:  github.String("Mathias Mathias"),
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
		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "mathias@coder.com", user.Email)

		require.Equal(t, "mathias", user.Username)
		require.Equal(t, "Mathias Mathias", user.Name)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
	})
	t.Run("SignupReplaceUnderscores", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{
			Auditor: auditor,

			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:  true,
				AllowEveryone: true,
				OAuth2Config:  &testutil.OAuth2Config{},
				ListOrganizationMemberships: func(_ context.Context, _ *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{}, nil
				},
				TeamMembership: func(_ context.Context, _ *http.Client, _, _, _ string) (*github.Membership, error) {
					return nil, errors.New("no teams")
				},
				AuthenticatedUser: func(_ context.Context, _ *http.Client) (*github.User, error) {
					return &github.User{
						ID:    github.Int64(100),
						Login: github.String("mathias_coder"),
					}, nil
				},
				ListEmails: func(_ context.Context, _ *http.Client) ([]*github.UserEmail, error) {
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
		client.SetSessionToken(authCookieValue(resp.Cookies()))
		user, err := client.User(context.Background(), "me")
		require.NoError(t, err)
		require.Equal(t, "mathias-coder", user.Username)
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
						ID:    github.Int64(100),
						Login: github.String("kyle"),
						Name:  github.String("Kylium Carbonate"),
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
	// The bug only is exercised when a deleted user with the same linked_id exists.
	// Still related open issues:
	// - https://github.com/coder/coder/issues/12116
	// - https://github.com/coder/coder/issues/12115
	t.Run("ChangedEmail", func(t *testing.T) {
		t.Parallel()
		fake := oidctest.NewFakeIDP(t,
			oidctest.WithServing(),
			oidctest.WithCallbackPath("/api/v2/users/oauth2/github/callback"),

		)
		const ghID = int64(7777)
		auditor := audit.NewMock()

		coderEmail := &github.UserEmail{
			Email:    github.String("alice@coder.com"),
			Verified: github.Bool(true),
			Primary:  github.Bool(true),

		}
		gmailEmail := &github.UserEmail{
			Email:    github.String("alice@gmail.com"),
			Verified: github.Bool(true),
			Primary:  github.Bool(false),
		}
		emails := []*github.UserEmail{
			gmailEmail,
			coderEmail,
		}
		owner, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			Auditor: auditor,
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				AllowSignups:  true,
				AllowEveryone: true,
				OAuth2Config:  promoauth.NewFactory(prometheus.NewRegistry()).NewGithub("test-github", fake.OIDCConfig(t, []string{})),
				ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{}, nil
				},
				TeamMembership: func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error) {
					return nil, errors.New("no teams")
				},
				AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
					return &github.User{
						Login: github.String("alice"),
						ID:    github.Int64(ghID),
						Name:  github.String("Alice Liddell"),
					}, nil
				},
				ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
					return emails, nil
				},
			},
		})
		first := coderdtest.CreateFirstUser(t, owner)
		ctx := testutil.Context(t, testutil.WaitLong)
		ownerUser, err := owner.User(context.Background(), "me")
		require.NoError(t, err)
		// Create the user, then delete the user, then create again.
		// This causes the email change to fail.
		client := codersdk.New(owner.URL)

		client, _ = fake.Login(t, client, jwt.MapClaims{})
		deleted, err := client.User(ctx, "me")

		require.NoError(t, err)
		err = owner.DeleteUser(ctx, deleted.ID)
		require.NoError(t, err)

		// Check no user links for the user
		links, err := db.GetUserLinksByUserID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(ownerUser, first.OrganizationID)), deleted.ID)
		require.NoError(t, err)
		require.Empty(t, links)
		// Make sure a user_link cannot be created with a deleted user.
		// nolint:gocritic // Unit test
		_, err = db.InsertUserLink(dbauthz.AsSystemRestricted(ctx), database.InsertUserLinkParams{

			UserID:            deleted.ID,
			LoginType:         "github",
			LinkedID:          "100",
			OAuthAccessToken:  "random",
			OAuthRefreshToken: "random",
			OAuthExpiry:       time.Now(),
			Claims:            database.UserLinkClaims{},
		})
		require.ErrorContains(t, err, "Cannot create user_link for deleted user")
		// Create the user again.
		client, _ = fake.Login(t, client, jwt.MapClaims{})
		user, err := client.User(ctx, "me")
		require.NoError(t, err)
		userID := user.ID
		require.Equal(t, user.Email, *coderEmail.Email)
		// Now the user is registered, let's change their primary email.
		coderEmail.Primary = github.Bool(false)
		gmailEmail.Primary = github.Bool(true)
		client, _ = fake.Login(t, client, jwt.MapClaims{})
		user, err = client.User(ctx, "me")
		require.NoError(t, err)

		require.Equal(t, user.ID, userID, "user_id is different, a new user was likely created")
		require.Equal(t, user.Email, *gmailEmail.Email)
		// Entirely change emails.
		newEmail := "alice@newdomain.com"
		emails = []*github.UserEmail{
			{
				Email:    github.String("alice@newdomain.com"),
				Primary:  github.Bool(true),
				Verified: github.Bool(true),
			},
		}
		client, _ = fake.Login(t, client, jwt.MapClaims{})
		user, err = client.User(ctx, "me")
		require.NoError(t, err)
		require.Equal(t, user.ID, userID, "user_id is different, a new user was likely created")
		require.Equal(t, user.Email, newEmail)
	})
	t.Run("DeviceFlow", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Config: &coderd.GithubOAuth2Config{
				OAuth2Config:       &testutil.OAuth2Config{},
				AllowOrganizations: []string{"coder"},
				AllowSignups:       true,
				ListOrganizationMemberships: func(_ context.Context, _ *http.Client) ([]*github.Membership, error) {
					return []*github.Membership{{

						State: &stateActive,
						Organization: &github.Organization{
							Login: github.String("coder"),
						},

					}}, nil
				},
				AuthenticatedUser: func(_ context.Context, _ *http.Client) (*github.User, error) {
					return &github.User{

						ID:    github.Int64(100),
						Login: github.String("testuser"),
						Name:  github.String("The Right Honorable Sir Test McUser"),
					}, nil

				},
				ListEmails: func(_ context.Context, _ *http.Client) ([]*github.UserEmail, error) {
					return []*github.UserEmail{{
						Email:    github.String("testuser@coder.com"),
						Verified: github.Bool(true),
						Primary:  github.Bool(true),
					}}, nil

				},
				DeviceFlowEnabled: true,
				ExchangeDeviceCode: func(_ context.Context, _ string) (*oauth2.Token, error) {
					return &oauth2.Token{
						AccessToken:  "access_token",
						RefreshToken: "refresh_token",
						Expiry:       time.Now().Add(time.Hour),
					}, nil
				},
				AuthorizeDevice: func(_ context.Context) (*codersdk.ExternalAuthDevice, error) {
					return &codersdk.ExternalAuthDevice{
						DeviceCode: "device_code",
						UserCode:   "user_code",

					}, nil
				},
			},
		})
		client.HTTPClient.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}

		// Ensure that we redirect to the device login page when the user is not logged in.
		oauthURL, err := client.URL.Parse("/api/v2/users/oauth2/github/callback")
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)

		require.NoError(t, err)
		res, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
		location, err := res.Location()
		require.NoError(t, err)

		require.Equal(t, "/login/device", location.Path)
		query := location.Query()
		require.NotEmpty(t, query.Get("state"))
		// Ensure that we return a JSON response when the code is successfully exchanged.
		oauthURL, err = client.URL.Parse("/api/v2/users/oauth2/github/callback?code=hey&state=somestate")
		require.NoError(t, err)
		req, err = http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)
		req.AddCookie(&http.Cookie{
			Name:  "oauth_state",
			Value: "somestate",
		})
		require.NoError(t, err)
		res, err = client.HTTPClient.Do(req)

		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
		var resp codersdk.OAuth2DeviceFlowCallbackResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
		require.Equal(t, "/", resp.RedirectURL)
	})
}
// nolint:bodyclose
func TestUserOIDC(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		Name                string
		IDTokenClaims       jwt.MapClaims
		UserInfoClaims      jwt.MapClaims
		AccessTokenClaims   jwt.MapClaims
		AllowSignups        bool
		EmailDomain         []string
		AssertUser          func(t testing.TB, u codersdk.User)
		StatusCode          int
		AssertResponse      func(t testing.TB, resp *http.Response)
		IgnoreEmailVerified bool
		IgnoreUserInfo      bool
		UseAccessToken      bool
		PrecreateFirstUser  bool
	}{
		{
			Name: "NoSub",
			IDTokenClaims: jwt.MapClaims{
				"email": "kyle@kwc.io",
			},
			AllowSignups: true,
			StatusCode:   http.StatusBadRequest,
		},
		{
			Name: "AccessTokenMerge",
			IDTokenClaims: jwt.MapClaims{
				"sub": uuid.NewString(),
			},
			AccessTokenClaims: jwt.MapClaims{
				"email": "kyle@kwc.io",
			},
			IgnoreUserInfo: true,
			AllowSignups:   true,
			UseAccessToken: true,
			StatusCode:     http.StatusOK,
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "kyle@kwc.io", u.Email)
			},
		},
		{
			Name: "AccessTokenMergeNotJWT",

			IDTokenClaims: jwt.MapClaims{
				"sub": uuid.NewString(),
			},
			IgnoreUserInfo: true,

			AllowSignups:   true,
			UseAccessToken: true,

			StatusCode:     http.StatusBadRequest,
		},
		{
			Name: "EmailOnly",
			IDTokenClaims: jwt.MapClaims{

				"email": "kyle@kwc.io",
				"sub":   uuid.NewString(),
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "kyle", u.Username)

			},
		},
		{
			Name: "EmailNotVerified",

			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": false,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			StatusCode:   http.StatusForbidden,
		},
		{
			Name: "EmailNotAString",

			IDTokenClaims: jwt.MapClaims{
				"email":          3.14159,
				"email_verified": false,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			StatusCode:   http.StatusBadRequest,

		},
		{
			Name: "EmailNotVerifiedIgnored",
			IDTokenClaims: jwt.MapClaims{

				"email":          "kyle@kwc.io",
				"email_verified": false,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, u.Username, "kyle")
			},
			IgnoreEmailVerified: true,
		},
		{
			Name: "NotInRequiredEmailDomain",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			EmailDomain: []string{
				"coder.com",
			},
			StatusCode: http.StatusForbidden,
		},
		{
			Name: "EmailDomainWithLeadingAt",
			IDTokenClaims: jwt.MapClaims{
				"email":          "cian@coder.com",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			EmailDomain: []string{
				"@coder.com",
			},
			StatusCode: http.StatusOK,
		},
		{
			Name: "EmailDomainForbiddenWithLeadingAt",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			EmailDomain: []string{
				"@coder.com",
			},
			StatusCode: http.StatusForbidden,
		},
		{
			Name: "EmailDomainCaseInsensitive",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@KWC.io",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, u.Username, "kyle")
			},
			EmailDomain: []string{
				"kwc.io",
			},
			StatusCode: http.StatusOK,
		},
		{
			Name: "EmailDomainSubset",
			IDTokenClaims: jwt.MapClaims{
				"email":          "colin@gmail.com",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			EmailDomain: []string{
				"mail.com",
			},
			StatusCode: http.StatusForbidden,
		},
		{
			Name:          "EmptyClaims",
			IDTokenClaims: jwt.MapClaims{},
			AllowSignups:  true,
			StatusCode:    http.StatusBadRequest,
		},
		{
			Name: "NoSignups",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			StatusCode:         http.StatusForbidden,
			PrecreateFirstUser: true,
		},
		{
			Name: "FirstSignup",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			StatusCode: http.StatusOK,
		},
		{
			Name: "UsernameFromEmail",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "kyle", u.Username)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "UsernameFromClaims",
			IDTokenClaims: jwt.MapClaims{
				"email":              "kyle@kwc.io",
				"email_verified":     true,
				"preferred_username": "hotdog",
				"sub":                uuid.NewString(),
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "hotdog", u.Username)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "FullNameFromClaims",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				"name":           "Hot Dog",
				"sub":            uuid.NewString(),
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "Hot Dog", u.Name)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "InvalidFullNameFromClaims",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				// Full names must be less or equal to than 128 characters in length.
				// However, we should not fail to log someone in if their name is too long.
				// Just truncate it.
				"name": strings.Repeat("a", 129),
				"sub":  uuid.NewString(),
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, strings.Repeat("a", 128), u.Name)
			},
		},
		{
			Name: "FullNameWhitespace",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				// Full names must not have leading or trailing whitespace, but this is a
				// daft reason to fail a login.
				"name": " Bobby  Whitespace ",
				"sub":  uuid.NewString(),
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "Bobby  Whitespace", u.Name)
			},
		},
		{
			// Services like Okta return the email as the username:
			// https://developer.okta.com/docs/reference/api/oidc/#base-claims-always-present
			Name: "UsernameAsEmail",
			IDTokenClaims: jwt.MapClaims{
				"email":              "kyle@kwc.io",
				"email_verified":     true,
				"name":               "Kylium Carbonate",
				"preferred_username": "kyle@kwc.io",
				"sub":                uuid.NewString(),
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "kyle", u.Username)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			// See: https://github.com/coder/coder/issues/4472
			Name: "UsernameIsEmail",
			IDTokenClaims: jwt.MapClaims{
				"preferred_username": "kyle@kwc.io",
				"sub":                uuid.NewString(),
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "kyle", u.Username)
				assert.Empty(t, u.Name)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "WithPicture",
			IDTokenClaims: jwt.MapClaims{
				"email":              "kyle@kwc.io",
				"email_verified":     true,
				"preferred_username": "kyle",
				"picture":            "/example.png",
				"sub":                uuid.NewString(),
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "/example.png", u.AvatarURL)
				assert.Equal(t, "kyle", u.Username)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "WithUserInfoClaims",
			IDTokenClaims: jwt.MapClaims{
				"email":          "kyle@kwc.io",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			UserInfoClaims: jwt.MapClaims{
				"preferred_username": "potato",
				"picture":            "/example.png",
				"name":               "Kylium Carbonate",
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "/example.png", u.AvatarURL)
				assert.Equal(t, "Kylium Carbonate", u.Name)
				assert.Equal(t, "potato", u.Username)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "GroupsDoesNothing",
			IDTokenClaims: jwt.MapClaims{
				"email":  "coolin@coder.com",
				"groups": []string{"pingpong"},
				"sub":    uuid.NewString(),
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "UserInfoOverridesIDTokenClaims",
			IDTokenClaims: jwt.MapClaims{
				"email":          "internaluser@internal.domain",
				"email_verified": false,
				"sub":            uuid.NewString(),
			},
			UserInfoClaims: jwt.MapClaims{
				"email":              "externaluser@external.domain",
				"email_verified":     true,
				"preferred_username": "user",
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "user", u.Username)
			},
			AllowSignups:        true,
			IgnoreEmailVerified: false,
			StatusCode:          http.StatusOK,
		},
		{
			Name: "InvalidUserInfo",
			IDTokenClaims: jwt.MapClaims{
				"email":          "internaluser@internal.domain",
				"email_verified": false,
				"sub":            uuid.NewString(),
			},
			UserInfoClaims: jwt.MapClaims{
				"email": 1,
			},
			AllowSignups:        true,
			IgnoreEmailVerified: false,
			StatusCode:          http.StatusInternalServerError,
		},
		{
			Name: "IgnoreUserInfo",
			IDTokenClaims: jwt.MapClaims{
				"email":              "user@internal.domain",
				"email_verified":     true,
				"name":               "User McName",
				"preferred_username": "user",
				"sub":                uuid.NewString(),
			},
			UserInfoClaims: jwt.MapClaims{
				"email":              "user.mcname@external.domain",
				"name":               "Mr. User McName",
				"preferred_username": "Mr. User McName",
			},
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "user", u.Username)
				assert.Equal(t, "User McName", u.Name)
			},
			IgnoreUserInfo: true,
			AllowSignups:   true,
			StatusCode:     http.StatusOK,
		},
		{
			Name: "HugeIDToken",
			IDTokenClaims: inflateClaims(t, jwt.MapClaims{
				"email":          "user@domain.tld",
				"email_verified": true,
				"sub":            uuid.NewString(),
			}, 65536),
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "user", u.Username)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "HugeClaims",
			IDTokenClaims: jwt.MapClaims{
				"email":          "user@domain.tld",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			UserInfoClaims: inflateClaims(t, jwt.MapClaims{}, 65536),
			AssertUser: func(t testing.TB, u codersdk.User) {
				assert.Equal(t, "user", u.Username)
			},
			AllowSignups: true,
			StatusCode:   http.StatusOK,
		},
		{
			Name: "IssuerMismatch",
			IDTokenClaims: jwt.MapClaims{
				"iss":            "https://mismatch.com",
				"email":          "user@domain.tld",
				"email_verified": true,
				"sub":            uuid.NewString(),
			},
			AllowSignups: true,
			StatusCode:   http.StatusBadRequest,
			AssertResponse: func(t testing.TB, resp *http.Response) {
				data, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Contains(t, string(data), "id token issued by a different provider")
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			opts := []oidctest.FakeIDPOpt{
				oidctest.WithRefresh(func(_ string) error {
					return errors.New("refreshing token should never occur")
				}),
				oidctest.WithServing(),
				oidctest.WithStaticUserInfo(tc.UserInfoClaims),
			}
			if tc.AccessTokenClaims != nil && len(tc.AccessTokenClaims) > 0 {
				opts = append(opts, oidctest.WithAccessTokenJWTHook(func(email string, exp time.Time) jwt.MapClaims {
					return tc.AccessTokenClaims
				}))
			}
			fake := oidctest.NewFakeIDP(t, opts...)
			cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
				cfg.AllowSignups = tc.AllowSignups
				cfg.EmailDomain = tc.EmailDomain
				cfg.IgnoreEmailVerified = tc.IgnoreEmailVerified
				cfg.SecondaryClaims = coderd.MergedClaimsSourceUserInfo
				if tc.IgnoreUserInfo {
					cfg.SecondaryClaims = coderd.MergedClaimsSourceNone
				}
				if tc.UseAccessToken {
					cfg.SecondaryClaims = coderd.MergedClaimsSourceAccessToken
				}
				cfg.NameField = "name"
			})
			auditor := audit.NewMock()
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
			owner := coderdtest.New(t, &coderdtest.Options{
				Auditor:    auditor,
				OIDCConfig: cfg,
				Logger:     &logger,
			})
			numLogs := len(auditor.AuditLogs())
			ctx := testutil.Context(t, testutil.WaitShort)
			if tc.PrecreateFirstUser {
				owner.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
					Email:    "precreated@coder.com",
					Username: "precreated",
					Password: "SomeSecurePassword!",
				})
			}
			client, resp := fake.AttemptLogin(t, owner, tc.IDTokenClaims)
			numLogs++ // add an audit log for login
			require.Equal(t, tc.StatusCode, resp.StatusCode)
			if tc.AssertResponse != nil {
				tc.AssertResponse(t, resp)
			}
			if tc.AssertUser != nil {
				user, err := client.User(ctx, "me")
				require.NoError(t, err)
				tc.AssertUser(t, user)
				require.Len(t, auditor.AuditLogs(), numLogs)
				require.NotEqual(t, uuid.Nil, auditor.AuditLogs()[numLogs-1].UserID)
				require.Equal(t, database.AuditActionRegister, auditor.AuditLogs()[numLogs-1].Action)
				require.Equal(t, 1, len(user.OrganizationIDs), "in the default org")
			}
		})
	}
	t.Run("OIDCDormancy", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		auditor := audit.NewMock()
		fake := oidctest.NewFakeIDP(t,
			oidctest.WithRefresh(func(_ string) error {
				return errors.New("refreshing token should never occur")
			}),
			oidctest.WithServing(),
		)
		cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true
		})
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		owner, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			Auditor:    auditor,
			OIDCConfig: cfg,
			Logger:     &logger,
		})
		user := dbgen.User(t, db, database.User{
			LoginType: database.LoginTypeOIDC,
			Status:    database.UserStatusDormant,
		})
		auditor.ResetLogs()
		client, resp := fake.AttemptLogin(t, owner, jwt.MapClaims{
			"email": user.Email,
			"sub":   uuid.NewString(),
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		auditor.Contains(t, database.AuditLog{
			ResourceType:     database.ResourceTypeUser,
			AdditionalFields: json.RawMessage(`{"automatic_actor":"coder","automatic_subsystem":"dormancy"}`),
		})

		me, err := client.User(ctx, "me")
		require.NoError(t, err)
		require.Equal(t, codersdk.UserStatusActive, me.Status)
	})
	t.Run("OIDCConvert", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		fake := oidctest.NewFakeIDP(t,
			oidctest.WithRefresh(func(_ string) error {
				return errors.New("refreshing token should never occur")
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
		require.Equal(t, codersdk.LoginTypePassword, userData.LoginType)
		claims := jwt.MapClaims{
			"email": userData.Email,
			"sub":   uuid.NewString(),
		}
		var err error
		user.HTTPClient.Jar, err = cookiejar.New(nil)
		require.NoError(t, err)

		user.HTTPClient.Transport = http.DefaultTransport.(*http.Transport).Clone()
		ctx := testutil.Context(t, testutil.WaitShort)
		convertResponse, err := user.ConvertLoginType(ctx, codersdk.ConvertLoginRequest{
			ToType:   codersdk.LoginTypeOIDC,
			Password: "SomeSecurePassword!",
		})
		require.NoError(t, err)
		_, _ = fake.LoginWithClient(t, user, claims, func(r *http.Request) {
			r.URL.RawQuery = url.Values{

				"oidc_merge_state": {convertResponse.StateString},
			}.Encode()
			r.Header.Set(codersdk.SessionTokenHeader, user.SessionToken())
			cookies := user.HTTPClient.Jar.Cookies(r.URL)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}

		})
		info, err := client.User(ctx, userData.ID.String())
		require.NoError(t, err)
		require.Equal(t, codersdk.LoginTypeOIDC, info.LoginType)

	})
	t.Run("BadJWT", func(t *testing.T) {
		t.Parallel()
		var (
			ctx    = testutil.Context(t, testutil.WaitMedium)
			logger = testutil.Logger(t)
		)
		auditor := audit.NewMock()
		fake := oidctest.NewFakeIDP(t,

			oidctest.WithRefresh(func(_ string) error {
				return errors.New("refreshing token should never occur")
			}),
			oidctest.WithServing(),

		)
		cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true
		})
		db, ps := dbtestutil.NewDB(t)
		fetcher := &cryptokeys.DBFetcher{
			DB: db,
		}
		kc, err := cryptokeys.NewSigningCache(ctx, logger, fetcher, codersdk.CryptoKeyFeatureOIDCConvert)
		require.NoError(t, err)
		client := coderdtest.New(t, &coderdtest.Options{

			Auditor:             auditor,
			OIDCConfig:          cfg,
			Database:            db,
			Pubsub:              ps,
			OIDCConvertKeyCache: kc,
		})
		owner := coderdtest.CreateFirstUser(t, client)

		user, userData := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		claims := jwt.MapClaims{
			"email": userData.Email,
			"sub":   uuid.NewString(),
		}
		user.HTTPClient.Jar, err = cookiejar.New(nil)

		require.NoError(t, err)
		user.HTTPClient.Transport = http.DefaultTransport.(*http.Transport).Clone()
		convertResponse, err := user.ConvertLoginType(ctx, codersdk.ConvertLoginRequest{
			ToType:   codersdk.LoginTypeOIDC,
			Password: "SomeSecurePassword!",
		})

		require.NoError(t, err)
		// Update the cookie to use a bad signing key. We're asserting the behavior of the scenario
		// where a JWT gets minted on an old version of Coder but gets verified on a new version.
		_, resp := fake.AttemptLogin(t, user, claims, func(r *http.Request) {
			r.URL.RawQuery = url.Values{
				"oidc_merge_state": {convertResponse.StateString},
			}.Encode()

			r.Header.Set(codersdk.SessionTokenHeader, user.SessionToken())
			cookies := user.HTTPClient.Jar.Cookies(user.URL)
			for i, cookie := range cookies {

				if cookie.Name != coderd.OAuthConvertCookieValue {
					continue
				}

				jwt := cookie.Value
				var claims coderd.OAuthConvertStateClaims
				err := jwtutils.Verify(ctx, kc, jwt, &claims)
				require.NoError(t, err)
				badJWT := generateBadJWT(t, claims)
				cookie.Value = badJWT
				cookies[i] = cookie
			}
			user.HTTPClient.Jar.SetCookies(user.URL, cookies)
			for _, cookie := range cookies {
				fmt.Printf("cookie: %+v\n", cookie)

				r.AddCookie(cookie)
			}
		})
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var respErr codersdk.Response
		err = json.NewDecoder(resp.Body).Decode(&respErr)
		require.NoError(t, err)
		require.Contains(t, respErr.Message, "Using an invalid jwt to authorize this action.")

	})
	t.Run("AlternateUsername", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		fake := oidctest.NewFakeIDP(t,
			oidctest.WithRefresh(func(_ string) error {
				return errors.New("refreshing token should never occur")
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
			"sub":   uuid.NewString(),
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

				return errors.New("refreshing token should never occur")
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
				return errors.New("refreshing token should never occur")

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

	t.Run("StripRedirectHost", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		expectedRedirect := "/foo/bar?hello=world&bar=baz"
		redirectURL := "https://malicious" + expectedRedirect
		callbackPath := fmt.Sprintf("/api/v2/users/oidc/callback?redirect=%s", url.QueryEscape(redirectURL))

		fake := oidctest.NewFakeIDP(t,
			oidctest.WithRefresh(func(_ string) error {
				return errors.New("refreshing token should never occur")
			}),
			oidctest.WithServing(),
			oidctest.WithCallbackPath(callbackPath),
		)
		cfg := fake.OIDCConfig(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true

		})
		client := coderdtest.New(t, &coderdtest.Options{

			OIDCConfig: cfg,
		})
		client.HTTPClient.Transport = http.DefaultTransport
		client.HTTPClient.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		claims := jwt.MapClaims{
			"email":          "user@example.com",
			"email_verified": true,
			"sub":            uuid.NewString(),
		}
		// Perform the login
		loginClient, resp := fake.LoginWithClient(t, client, claims)

		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		// Get the location from the response
		location, err := resp.Location()
		require.NoError(t, err)
		// Check that the redirect URL has been stripped of its malicious host
		require.Equal(t, expectedRedirect, location.RequestURI())
		require.Equal(t, client.URL.Host, location.Host)
		require.NotContains(t, location.String(), "malicious")
		// Verify the user was created
		user, err := loginClient.User(ctx, "me")
		require.NoError(t, err)
		require.Equal(t, "user@example.com", user.Email)
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

	newUser, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		Email:           email,
		Username:        username,
		Password:        password,
		OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
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
// TestOIDCSkipIssuer verifies coderd can run without checking the issuer url
// in the OIDC exchange. This means the CODER_OIDC_ISSUER_URL does not need
// to match the id_token `iss` field, or the value returned in the well-known

// config.
//
// So this test has:
// - OIDC at http://localhost:<port>

// - well-known config with issuer https://primary.com
// - JWT with issuer https://secondary.com
//
// Without this security check disabled, all three above would have to match.

func TestOIDCSkipIssuer(t *testing.T) {
	t.Parallel()
	const primaryURLString = "https://primary.com"

	const secondaryURLString = "https://secondary.com"
	primaryURL := must(url.Parse(primaryURLString))
	fake := oidctest.NewFakeIDP(t,
		oidctest.WithServing(),
		oidctest.WithDefaultIDClaims(jwt.MapClaims{}),
		oidctest.WithHookWellKnown(func(r *http.Request, j *oidctest.ProviderJSON) error {
			assert.NotEqual(t, r.URL.Host, primaryURL.Host, "request went to wrong host")
			j.Issuer = primaryURLString
			return nil
		}),
	)
	owner := coderdtest.New(t, &coderdtest.Options{

		OIDCConfig: fake.OIDCConfigSkipIssuerChecks(t, nil, func(cfg *coderd.OIDCConfig) {
			cfg.AllowSignups = true
		}),
	})

	// User can login and use their token.
	ctx := testutil.Context(t, testutil.WaitShort)

	//nolint:bodyclose
	userClient, _ := fake.Login(t, owner, jwt.MapClaims{
		"iss":   secondaryURLString,
		"email": "alice@coder.com",

		"sub":   uuid.NewString(),
	})
	found, err := userClient.User(ctx, "me")
	require.NoError(t, err)
	require.Equal(t, found.LoginType, codersdk.LoginTypeOIDC)
}

func TestUserForgotPassword(t *testing.T) {
	t.Parallel()
	const oldPassword = "SomeSecurePassword!"
	const newPassword = "SomeNewSecurePassword!"

	requireOneTimePasscodeNotification := func(t *testing.T, notif *notificationstest.FakeNotification, userID uuid.UUID) {
		require.Equal(t, notifications.TemplateUserRequestedOneTimePasscode, notif.TemplateID)
		require.Equal(t, userID, notif.UserID)
		require.Equal(t, 1, len(notif.Targets))

		require.Equal(t, userID, notif.Targets[0])
	}
	requireCanLogin := func(t *testing.T, ctx context.Context, client *codersdk.Client, email string, password string) {
		_, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    email,

			Password: password,
		})
		require.NoError(t, err)
	}
	requireCannotLogin := func(t *testing.T, ctx context.Context, client *codersdk.Client, email string, password string) {
		_, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    email,

			Password: password,
		})
		var apiErr *codersdk.Error

		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Incorrect email or password.")
	}

	requireRequestOneTimePasscode := func(t *testing.T, ctx context.Context, client *codersdk.Client, notifyEnq *notificationstest.FakeEnqueuer, email string, userID uuid.UUID) string {
		notifyEnq.Clear()
		err := client.RequestOneTimePasscode(ctx, codersdk.RequestOneTimePasscodeRequest{Email: email})
		require.NoError(t, err)
		sent := notifyEnq.Sent()
		require.Len(t, sent, 1)

		requireOneTimePasscodeNotification(t, sent[0], userID)
		return sent[0].Labels["one_time_passcode"]

	}
	requireChangePasswordWithOneTimePasscode := func(t *testing.T, ctx context.Context, client *codersdk.Client, email string, passcode string, password string) {
		err := client.ChangePasswordWithOneTimePasscode(ctx, codersdk.ChangePasswordWithOneTimePasscodeRequest{
			Email:           email,
			OneTimePasscode: passcode,
			Password:        password,
		})
		require.NoError(t, err)
	}
	t.Run("CanChangePassword", func(t *testing.T) {
		t.Parallel()
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})

		user := coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		// First try to login before changing our password. We expected this to error
		// as we haven't change the password yet.
		requireCannotLogin(t, ctx, anotherClient, anotherUser.Email, newPassword)
		oneTimePasscode := requireRequestOneTimePasscode(t, ctx, anotherClient, notifyEnq, anotherUser.Email, anotherUser.ID)

		requireChangePasswordWithOneTimePasscode(t, ctx, anotherClient, anotherUser.Email, oneTimePasscode, newPassword)
		requireCanLogin(t, ctx, anotherClient, anotherUser.Email, newPassword)
		// We now need to check that the one-time passcode isn't valid.
		err := anotherClient.ChangePasswordWithOneTimePasscode(ctx, codersdk.ChangePasswordWithOneTimePasscodeRequest{
			Email:           anotherUser.Email,
			OneTimePasscode: oneTimePasscode,
			Password:        newPassword + "!",
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Incorrect email or one-time passcode.")
		requireCannotLogin(t, ctx, anotherClient, anotherUser.Email, newPassword+"!")
		requireCanLogin(t, ctx, anotherClient, anotherUser.Email, newPassword)

	})
	t.Run("OneTimePasscodeExpires", func(t *testing.T) {
		t.Parallel()
		const oneTimePasscodeValidityPeriod = 1 * time.Millisecond
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer:         notifyEnq,

			OneTimePasscodeValidityPeriod: oneTimePasscodeValidityPeriod,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		oneTimePasscode := requireRequestOneTimePasscode(t, ctx, anotherClient, notifyEnq, anotherUser.Email, anotherUser.ID)
		// Wait for long enough so that the token expires
		time.Sleep(oneTimePasscodeValidityPeriod + 1*time.Millisecond)

		// Try to change password with an expired one time passcode.
		err := anotherClient.ChangePasswordWithOneTimePasscode(ctx, codersdk.ChangePasswordWithOneTimePasscodeRequest{
			Email:           anotherUser.Email,
			OneTimePasscode: oneTimePasscode,
			Password:        newPassword,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Incorrect email or one-time passcode.")

		// Ensure that the password was not changed.
		requireCannotLogin(t, ctx, anotherClient, anotherUser.Email, newPassword)
		requireCanLogin(t, ctx, anotherClient, anotherUser.Email, oldPassword)
	})

	t.Run("CannotChangePasswordWithoutRequestingOneTimePasscode", func(t *testing.T) {
		t.Parallel()
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		err := anotherClient.ChangePasswordWithOneTimePasscode(ctx, codersdk.ChangePasswordWithOneTimePasscodeRequest{
			Email:           anotherUser.Email,
			OneTimePasscode: uuid.New().String(),
			Password:        newPassword,

		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Incorrect email or one-time passcode")
		requireCannotLogin(t, ctx, anotherClient, anotherUser.Email, newPassword)
		requireCanLogin(t, ctx, anotherClient, anotherUser.Email, oldPassword)
	})

	t.Run("CannotChangePasswordWithInvalidOneTimePasscode", func(t *testing.T) {
		t.Parallel()
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_ = requireRequestOneTimePasscode(t, ctx, anotherClient, notifyEnq, anotherUser.Email, anotherUser.ID)
		err := anotherClient.ChangePasswordWithOneTimePasscode(ctx, codersdk.ChangePasswordWithOneTimePasscodeRequest{
			Email:           anotherUser.Email,
			OneTimePasscode: uuid.New().String(), // Use a different UUID to the one expected
			Password:        newPassword,
		})
		var apiErr *codersdk.Error

		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Incorrect email or one-time passcode")
		requireCannotLogin(t, ctx, anotherClient, anotherUser.Email, newPassword)
		requireCanLogin(t, ctx, anotherClient, anotherUser.Email, oldPassword)
	})
	t.Run("CannotChangePasswordWithNoOneTimePasscode", func(t *testing.T) {
		t.Parallel()
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client := coderdtest.New(t, &coderdtest.Options{

			NotificationsEnqueuer: notifyEnq,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		_ = requireRequestOneTimePasscode(t, ctx, anotherClient, notifyEnq, anotherUser.Email, anotherUser.ID)
		err := anotherClient.ChangePasswordWithOneTimePasscode(ctx, codersdk.ChangePasswordWithOneTimePasscodeRequest{
			Email:           anotherUser.Email,
			OneTimePasscode: "",
			Password:        newPassword,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Validation failed.")
		require.Equal(t, 1, len(apiErr.Validations))
		require.Equal(t, "one_time_passcode", apiErr.Validations[0].Field)
		requireCannotLogin(t, ctx, anotherClient, anotherUser.Email, newPassword)

		requireCanLogin(t, ctx, anotherClient, anotherUser.Email, oldPassword)
	})
	t.Run("CannotChangePasswordWithWeakPassword", func(t *testing.T) {

		t.Parallel()
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client := coderdtest.New(t, &coderdtest.Options{

			NotificationsEnqueuer: notifyEnq,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		oneTimePasscode := requireRequestOneTimePasscode(t, ctx, anotherClient, notifyEnq, anotherUser.Email, anotherUser.ID)

		err := anotherClient.ChangePasswordWithOneTimePasscode(ctx, codersdk.ChangePasswordWithOneTimePasscodeRequest{
			Email:           anotherUser.Email,
			OneTimePasscode: oneTimePasscode,
			Password:        "notstrong",
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())

		require.Contains(t, apiErr.Message, "Invalid password.")
		require.Equal(t, 1, len(apiErr.Validations))
		require.Equal(t, "password", apiErr.Validations[0].Field)
		requireCannotLogin(t, ctx, anotherClient, anotherUser.Email, "notstrong")
		requireCanLogin(t, ctx, anotherClient, anotherUser.Email, oldPassword)
	})
	t.Run("CannotChangePasswordOfAnotherUser", func(t *testing.T) {
		t.Parallel()
		notifyEnq := &notificationstest.FakeEnqueuer{}
		client := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,

		})
		user := coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		anotherClient, anotherUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		thirdClient, thirdUser := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		// Request a One-Time Passcode for `anotherUser`

		oneTimePasscode := requireRequestOneTimePasscode(t, ctx, anotherClient, notifyEnq, anotherUser.Email, anotherUser.ID)
		// Ensure we cannot change the password for `thirdUser` with `anotherUser`'s One-Time Passcode.
		err := thirdClient.ChangePasswordWithOneTimePasscode(ctx, codersdk.ChangePasswordWithOneTimePasscodeRequest{
			Email:           thirdUser.Email,

			OneTimePasscode: oneTimePasscode,
			Password:        newPassword,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "Incorrect email or one-time passcode")
		requireCannotLogin(t, ctx, thirdClient, thirdUser.Email, newPassword)
		requireCanLogin(t, ctx, thirdClient, thirdUser.Email, oldPassword)

		requireCanLogin(t, ctx, anotherClient, anotherUser.Email, oldPassword)
	})
	t.Run("GivenOKResponseWithInvalidEmail", func(t *testing.T) {

		t.Parallel()
		notifyEnq := &notificationstest.FakeEnqueuer{}

		client := coderdtest.New(t, &coderdtest.Options{
			NotificationsEnqueuer: notifyEnq,
		})
		user := coderdtest.CreateFirstUser(t, client)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)

		defer cancel()
		anotherClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		err := anotherClient.RequestOneTimePasscode(ctx, codersdk.RequestOneTimePasscodeRequest{

			Email: "not-a-member@coder.com",
		})

		require.NoError(t, err)
		sent := notifyEnq.Sent()
		require.Len(t, notifyEnq.Sent(), 1)
		require.NotEqual(t, notifications.TemplateUserRequestedOneTimePasscode, sent[0].TemplateID)

	})
}

func oauth2Callback(t *testing.T, client *codersdk.Client, opts ...func(*http.Request)) *http.Response {
	client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse

	}
	state := "somestate"
	oauthURL, err := client.URL.Parse("/api/v2/users/oauth2/github/callback?code=asd&state=" + state)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), "GET", oauthURL.String(), nil)
	require.NoError(t, err)
	for _, opt := range opts {
		opt(req)
	}
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

// inflateClaims 'inflates' a jwt.MapClaims from a seed by
// adding a ridiculously large key-value pair of length size.

func inflateClaims(t testing.TB, seed jwt.MapClaims, size int) jwt.MapClaims {
	t.Helper()

	junk, err := cryptorand.String(size)
	require.NoError(t, err)
	seed["random_data"] = junk

	return seed
}
// generateBadJWT generates a JWT with a random key. It's intended to emulate the old-style JWT's we generated.
func generateBadJWT(t *testing.T, claims interface{}) string {
	t.Helper()
	var buf [64]byte
	_, err := rand.Read(buf[:])
	require.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.HS512,
		Key:       buf[:],

	}, nil)
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	signed, err := signer.Sign(payload)

	require.NoError(t, err)
	compact, err := signed.CompactSerialize()
	require.NoError(t, err)

	return compact
}
