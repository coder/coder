package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// nolint:bodyclose
func TestUserOIDC(t *testing.T) {
	t.Parallel()
	t.Run("RoleSync", func(t *testing.T) {
		t.Parallel()

		// NoRoles is the "control group". It has claims with 0 roles
		// assigned, and asserts that the user has no roles.
		t.Run("NoRoles", func(t *testing.T) {
			t.Parallel()

			const oidcRoleName = "TemplateAuthor"
			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.UserRoleField = "roles"
				},
			})

			claims := jwt.MapClaims{
				"email": "alice@coder.com",
			}
			// Login a new client that signs up
			client, resp := runner.Login(claims)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			// User should be in 0 groups.
			runner.AssertRoles(t, "alice", []string{})
			// Force a refresh, and assert nothing has changes
			runner.ForceRefresh(t, client, claims)(t)
			runner.AssertRoles(t, "alice", []string{})
		})

		// A user has some roles, then on an oauth refresh will lose said
		// roles from an updated claim.
		t.Run("NewUserAndRemoveRolesOnRefresh", func(t *testing.T) {
			t.Skip("Refreshing tokens does not update roles :(")
			t.Parallel()

			const oidcRoleName = "TemplateAuthor"
			runner := setupOIDCTest(t, oidcTestConfig{
				Userinfo: jwt.MapClaims{oidcRoleName: []string{rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin()}},
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.UserRoleField = "roles"
					cfg.UserRoleMapping = map[string][]string{
						oidcRoleName: {rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin()},
					}
				},
			})

			// User starts with the owner role
			client, resp := runner.Login(jwt.MapClaims{
				"email": "alice@coder.com",
				"roles": []string{"random", oidcRoleName, rbac.RoleOwner()},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertRoles(t, "alice", []string{rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin(), rbac.RoleOwner()})

			// Now refresh the oauth, and check the roles are removed.
			// Force a refresh, and assert nothing has changes
			runner.ForceRefresh(t, client, jwt.MapClaims{
				"email": "alice@coder.com",
				"roles": []string{"random"},
			})(t)
			runner.AssertRoles(t, "alice", []string{})
		})

		t.Run("BlockAssignRoles", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			conf := coderdtest.NewOIDCConfig(t, "")

			config := conf.OIDCConfig(t, jwt.MapClaims{})
			config.AllowSignups = true
			config.UserRoleField = "roles"

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					OIDCConfig: config,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{codersdk.FeatureUserRoleManagement: 1},
				},
			})

			admin, err := client.User(ctx, "me")
			require.NoError(t, err)
			require.Len(t, admin.OrganizationIDs, 1)

			resp := oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email": "alice@coder.com",
				"roles": []string{},
			}))
			require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
			// Try to manually update user roles, even though controlled by oidc
			// role sync.
			_, err = client.UpdateUserRoles(ctx, "alice", codersdk.UpdateRoles{
				Roles: []string{
					rbac.RoleTemplateAdmin(),
				},
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "Cannot modify roles for OIDC users when role sync is enabled.")
		})
	})

	t.Run("Groups", func(t *testing.T) {
		t.Parallel()
		t.Run("Assigns", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			conf := coderdtest.NewOIDCConfig(t, "")

			const groupClaim = "custom-groups"
			config := conf.OIDCConfig(t, jwt.MapClaims{}, func(cfg *coderd.OIDCConfig) {
				cfg.GroupField = groupClaim
			})
			config.AllowSignups = true

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					OIDCConfig: config,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{codersdk.FeatureTemplateRBAC: 1},
				},
			})

			admin, err := client.User(ctx, "me")
			require.NoError(t, err)
			require.Len(t, admin.OrganizationIDs, 1)

			groupName := "bingbong"
			group, err := client.CreateGroup(ctx, admin.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: groupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			resp := oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email":    "colin@coder.com",
				groupClaim: []string{groupName},
			}))
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

			group, err = client.Group(ctx, group.ID)
			require.NoError(t, err)
			require.Len(t, group.Members, 1)
		})
		t.Run("AssignsMapped", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			conf := coderdtest.NewOIDCConfig(t, "")

			oidcGroupName := "pingpong"
			coderGroupName := "bingbong"

			config := conf.OIDCConfig(t, jwt.MapClaims{}, func(cfg *coderd.OIDCConfig) {
				cfg.GroupMapping = map[string]string{oidcGroupName: coderGroupName}
			})
			config.AllowSignups = true

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					OIDCConfig: config,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{codersdk.FeatureTemplateRBAC: 1},
				},
			})

			admin, err := client.User(ctx, "me")
			require.NoError(t, err)
			require.Len(t, admin.OrganizationIDs, 1)

			group, err := client.CreateGroup(ctx, admin.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: coderGroupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			resp := oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email":  "colin@coder.com",
				"groups": []string{oidcGroupName},
			}))
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

			group, err = client.Group(ctx, group.ID)
			require.NoError(t, err)
			require.Len(t, group.Members, 1)
		})

		t.Run("AddThenRemove", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			conf := coderdtest.NewOIDCConfig(t, "")

			config := conf.OIDCConfig(t, jwt.MapClaims{})
			config.AllowSignups = true

			client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					OIDCConfig: config,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{codersdk.FeatureTemplateRBAC: 1},
				},
			})

			// Add some extra users/groups that should be asserted after.
			// Adding this user as there was a bug that removing 1 user removed
			// all users from the group.
			_, extra := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
			groupName := "bingbong"
			group, err := client.CreateGroup(ctx, firstUser.OrganizationID, codersdk.CreateGroupRequest{
				Name: groupName,
			})
			require.NoError(t, err, "create group")

			group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
				AddUsers: []string{
					firstUser.UserID.String(),
					extra.ID.String(),
				},
			})
			require.NoError(t, err, "patch group")
			require.Len(t, group.Members, 2, "expect both members")

			// Now add OIDC user into the group
			resp := oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email":  "colin@coder.com",
				"groups": []string{groupName},
			}))
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

			group, err = client.Group(ctx, group.ID)
			require.NoError(t, err)
			require.Len(t, group.Members, 3)

			// Login to remove the OIDC user from the group
			resp = oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email":  "colin@coder.com",
				"groups": []string{},
			}))
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

			group, err = client.Group(ctx, group.ID)
			require.NoError(t, err)
			require.Len(t, group.Members, 2)
			var expected []uuid.UUID
			for _, mem := range group.Members {
				expected = append(expected, mem.ID)
			}
			require.ElementsMatchf(t, expected, []uuid.UUID{firstUser.UserID, extra.ID}, "expected members")
		})

		t.Run("NoneMatch", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			conf := coderdtest.NewOIDCConfig(t, "")

			config := conf.OIDCConfig(t, jwt.MapClaims{})
			config.AllowSignups = true

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					OIDCConfig: config,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{codersdk.FeatureTemplateRBAC: 1},
				},
			})

			admin, err := client.User(ctx, "me")
			require.NoError(t, err)
			require.Len(t, admin.OrganizationIDs, 1)

			groupName := "bingbong"
			group, err := client.CreateGroup(ctx, admin.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: groupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			resp := oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email":  "colin@coder.com",
				"groups": []string{"coolin"},
			}))
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

			group, err = client.Group(ctx, group.ID)
			require.NoError(t, err)
			require.Len(t, group.Members, 0)
		})
	})
}

func TestGroupSync(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		modCfg func(cfg *coderd.OIDCConfig)
		// initialOrgGroups is initial groups in the org
		initialOrgGroups []string
		// initialUserGroups is initial groups for the user
		initialUserGroups []string
		// expectedUserGroups is expected groups for the user
		expectedUserGroups []string
		// expectedOrgGroups is expected all groups on the system
		expectedOrgGroups []string
		claims            jwt.MapClaims
	}{
		{
			name: "NoGroups",
			modCfg: func(cfg *coderd.OIDCConfig) {
			},
			initialOrgGroups:   []string{},
			expectedUserGroups: []string{},
			expectedOrgGroups:  []string{},
			claims:             jwt.MapClaims{},
		},
		{
			name: "GroupSyncDisabled",
			modCfg: func(cfg *coderd.OIDCConfig) {
				// Disable group sync
				cfg.GroupField = ""
				cfg.GroupFilter = regexp.MustCompile(".*")
			},
			initialOrgGroups:   []string{"a", "b", "c", "d"},
			initialUserGroups:  []string{"b", "c", "d"},
			expectedUserGroups: []string{"b", "c", "d"},
			expectedOrgGroups:  []string{"a", "b", "c", "d"},
			claims:             jwt.MapClaims{},
		},
		{
			// From a,c,b -> b,c,d
			name: "ChangeUserGroups",
			modCfg: func(cfg *coderd.OIDCConfig) {
				cfg.GroupMapping = map[string]string{
					"D": "d",
				}
			},
			initialOrgGroups:   []string{"a", "b", "c", "d"},
			initialUserGroups:  []string{"a", "b", "c"},
			expectedUserGroups: []string{"b", "c", "d"},
			expectedOrgGroups:  []string{"a", "b", "c", "d"},
			claims: jwt.MapClaims{
				// D -> d mapped
				"groups": []string{"b", "c", "D"},
			},
		},
		{
			// From a,c,b -> []
			name: "RemoveAllGroups",
			modCfg: func(cfg *coderd.OIDCConfig) {
				cfg.GroupFilter = regexp.MustCompile(".*")
			},
			initialOrgGroups:   []string{"a", "b", "c", "d"},
			initialUserGroups:  []string{"a", "b", "c"},
			expectedUserGroups: []string{},
			expectedOrgGroups:  []string{"a", "b", "c", "d"},
			claims:             jwt.MapClaims{
				// No claim == no groups
			},
		},
		{
			// From a,c,b -> b,c,d,e,f
			name: "CreateMissingGroups",
			modCfg: func(cfg *coderd.OIDCConfig) {
				cfg.CreateMissingGroups = true
			},
			initialOrgGroups:   []string{"a", "b", "c", "d"},
			initialUserGroups:  []string{"a", "b", "c"},
			expectedUserGroups: []string{"b", "c", "d", "e", "f"},
			expectedOrgGroups:  []string{"a", "b", "c", "d", "e", "f"},
			claims: jwt.MapClaims{
				"groups": []string{"b", "c", "d", "e", "f"},
			},
		},
		{
			// From a,c,b -> b,c,d,e,f
			name: "CreateMissingGroupsFilter",
			modCfg: func(cfg *coderd.OIDCConfig) {
				cfg.CreateMissingGroups = true
				// Only single letter groups
				cfg.GroupFilter = regexp.MustCompile("^[a-z]$")
				cfg.GroupMapping = map[string]string{
					// Does not match the filter, but does after being mapped!
					"zebra": "z",
				}
			},
			initialOrgGroups:   []string{"a", "b", "c", "d"},
			initialUserGroups:  []string{"a", "b", "c"},
			expectedUserGroups: []string{"b", "c", "d", "e", "f", "z"},
			expectedOrgGroups:  []string{"a", "b", "c", "d", "e", "f", "z"},
			claims: jwt.MapClaims{
				"groups": []string{
					"b", "c", "d", "e", "f",
					// These groups are ignored
					"excess", "ignore", "dumb", "foobar", "zebra",
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			conf := coderdtest.NewOIDCConfig(t, "")

			config := conf.OIDCConfig(t, jwt.MapClaims{}, tc.modCfg)

			client, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					OIDCConfig: config,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{codersdk.FeatureTemplateRBAC: 1},
				},
			})

			admin, err := client.User(ctx, "me")
			require.NoError(t, err)
			require.Len(t, admin.OrganizationIDs, 1)

			// Setup
			initialGroups := make(map[string]codersdk.Group)
			for _, group := range tc.initialOrgGroups {
				newGroup, err := client.CreateGroup(ctx, admin.OrganizationIDs[0], codersdk.CreateGroupRequest{
					Name: group,
				})
				require.NoError(t, err)
				require.Len(t, newGroup.Members, 0)
				initialGroups[group] = newGroup
			}

			// Create the user and add them to their initial groups
			_, user := coderdtest.CreateAnotherUser(t, client, admin.OrganizationIDs[0])
			for _, group := range tc.initialUserGroups {
				_, err := client.PatchGroup(ctx, initialGroups[group].ID, codersdk.PatchGroupRequest{
					AddUsers: []string{user.ID.String()},
				})
				require.NoError(t, err)
			}

			// nolint:gocritic
			_, err = api.Database.UpdateUserLoginType(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLoginTypeParams{
				NewLoginType: database.LoginTypeOIDC,
				UserID:       user.ID,
			})
			require.NoError(t, err, "user must be oidc type")

			// Log in the new user
			tc.claims["email"] = user.Email
			resp := oidcCallback(t, client, conf.EncodeClaims(t, tc.claims))
			assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
			_ = resp.Body.Close()

			orgGroups, err := client.GroupsByOrganization(ctx, admin.OrganizationIDs[0])
			require.NoError(t, err)

			for _, group := range orgGroups {
				if slice.Contains(tc.initialOrgGroups, group.Name) || group.IsEveryone() {
					require.Equal(t, group.Source, codersdk.GroupSourceUser)
				} else {
					require.Equal(t, group.Source, codersdk.GroupSourceOIDC)
				}
			}

			orgGroupsMap := make(map[string]struct{})
			for _, group := range orgGroups {
				orgGroupsMap[group.Name] = struct{}{}
			}

			for _, expected := range tc.expectedOrgGroups {
				if _, ok := orgGroupsMap[expected]; !ok {
					t.Errorf("expected group %s not found", expected)
				}
				delete(orgGroupsMap, expected)
			}
			delete(orgGroupsMap, database.EveryoneGroup)
			require.Empty(t, orgGroupsMap, "unexpected groups found")

			expectedUserGroups := make(map[string]struct{})
			for _, group := range tc.expectedUserGroups {
				expectedUserGroups[group] = struct{}{}
			}

			for _, group := range orgGroups {
				userInGroup := slice.ContainsCompare(group.Members, codersdk.User{Email: user.Email}, func(a, b codersdk.User) bool {
					return a.Email == b.Email
				})
				if group.IsEveryone() {
					require.True(t, userInGroup, "user cannot be removed from 'Everyone' group")
				} else if _, ok := expectedUserGroups[group.Name]; ok {
					require.Truef(t, userInGroup, "user should be in group %s", group.Name)
				} else {
					require.Falsef(t, userInGroup, "user should not be in group %s", group.Name)
				}
			}
		})
	}
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

// oidcTestRunner is just a helper to setup and run oidc tests.
// An actual Coderd instance is used to run the tests.
type oidcTestRunner struct {
	AdminClient *codersdk.Client
	AdminUser   codersdk.User

	// Login will call the OIDC flow with an unauthenticated client.
	// The customer actions will all be taken care of, and the idToken claims
	// will be returned.
	Login func(idToken jwt.MapClaims) (*codersdk.Client, *http.Response)
	// ForceRefresh will use an authenticated codersdk.Client, and force their
	// OIDC token to be expired and require a refresh. The refresh will use the claims provided.
	//
	// The client MUST be used to actually trigger the refresh. This just
	// expires the oauth token so the next authenticated API call will
	// trigger a refresh. The returned function is an example of said call.
	// It just calls the /users/me endpoint to trigger the refresh.
	ForceRefresh func(t *testing.T, client *codersdk.Client, idToken jwt.MapClaims) func(t *testing.T)
}

type oidcTestConfig struct {
	Userinfo jwt.MapClaims

	// Config allows modifying the Coderd OIDC configuration.
	Config func(cfg *coderd.OIDCConfig)
}

func (r *oidcTestRunner) AssertRoles(t *testing.T, userIdent string, roles []string) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitMedium)
	user, err := r.AdminClient.User(ctx, userIdent)
	require.NoError(t, err)

	roleNames := []string{}
	for _, role := range user.Roles {
		roleNames = append(roleNames, role.Name)
	}
	require.ElementsMatch(t, roles, roleNames, "expected roles")
}

func (r *oidcTestRunner) AssertGroups(t *testing.T, userIdent string, groups []string) {
	ctx := testutil.Context(t, testutil.WaitMedium)
	user, err := r.AdminClient.User(ctx, userIdent)
	require.NoError(t, err)

	allGroups, err := r.AdminClient.GroupsByOrganization(ctx, user.OrganizationIDs[0])
	require.NoError(t, err)

	userInGroups := []string{}
	for _, g := range allGroups {
		for _, mem := range g.Members {
			if mem.ID == user.ID {
				userInGroups = append(userInGroups, g.Name)
			}
		}
	}

	require.ElementsMatch(t, groups, userInGroups, "expected groups")
}

func setupOIDCTest(t *testing.T, settings oidcTestConfig) *oidcTestRunner {
	t.Helper()

	fake := oidctest.NewFakeIDP(t,
		oidctest.WithStaticUserInfo(settings.Userinfo),
		oidctest.WithLogging(t, nil),
		// Run fake IDP on a real webserver
		oidctest.WithServing(),
	)

	ctx := testutil.Context(t, testutil.WaitMedium)
	cfg := fake.OIDCConfig(t, nil, settings.Config)
	client, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			OIDCConfig: cfg,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureUserRoleManagement: 1},
		},
	})
	admin, err := client.User(ctx, "me")
	require.NoError(t, err)
	unauthenticatedClient := codersdk.New(client.URL)

	return &oidcTestRunner{
		AdminClient: client,
		AdminUser:   admin,
		Login: func(idToken jwt.MapClaims) (*codersdk.Client, *http.Response) {
			return fake.LoginClient(t, unauthenticatedClient, idToken)
		},
		ForceRefresh: func(t *testing.T, client *codersdk.Client, idToken jwt.MapClaims) (authenticatedCall func(t *testing.T)) {
			t.Helper()

			//nolint:gocritic // Testing
			ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitMedium))

			id, _, err := httpmw.SplitAPIToken(client.SessionToken())
			require.NoError(t, err)

			// We need to get the OIDC link and update it in the database to force
			// it to be expired.
			key, err := api.Database.GetAPIKeyByID(ctx, id)
			require.NoError(t, err, "get api key")

			link, err := api.Database.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
				UserID:    key.UserID,
				LoginType: database.LoginTypeOIDC,
			})
			require.NoError(t, err, "get user link")

			// Updates the claims that the IDP will return. By default, it always
			// uses the original claims for the original oauth token.
			fake.UpdateRefreshClaims(link.OAuthRefreshToken, idToken)

			// Fetch the oauth link for the given user.
			_, err = api.Database.UpdateUserLink(ctx, database.UpdateUserLinkParams{
				OAuthAccessToken:  link.OAuthAccessToken,
				OAuthRefreshToken: link.OAuthRefreshToken,
				OAuthExpiry:       time.Now().Add(time.Hour * -1),
				UserID:            key.UserID,
				LoginType:         database.LoginTypeOIDC,
			})
			require.NoError(t, err, "expire user link")
			t.Cleanup(func() {
				require.True(t, fake.RefreshUsed(link.OAuthRefreshToken), "refresh token must be used, but has not. Did you forget to call the returned function from this call?")
			})

			return func(t *testing.T) {
				t.Helper()

				// Do any authenticated call to force the refresh
				_, err := client.User(testutil.Context(t, testutil.WaitShort), "me")
				require.NoError(t, err, "user must be able to be fetched")
			}
		},
	}
}
