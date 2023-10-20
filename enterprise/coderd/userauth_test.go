package coderd_test

import (
	"context"
	"net/http"
	"regexp"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	coderden "github.com/coder/coder/v2/enterprise/coderd"
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
			client, resp := runner.Login(t, claims)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			// User should be in 0 groups.
			runner.AssertRoles(t, "alice", []string{})
			// Force a refresh, and assert nothing has changes
			runner.ForceRefresh(t, client, claims)
			runner.AssertRoles(t, "alice", []string{})
		})

		// A user has some roles, then on an oauth refresh will lose said
		// roles from an updated claim.
		t.Run("NewUserAndRemoveRolesOnRefresh", func(t *testing.T) {
			// TODO: Implement new feature to update roles/groups on OIDC
			// refresh tokens. https://github.com/coder/coder/issues/9312
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
			client, resp := runner.Login(t, jwt.MapClaims{
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
			})
			runner.AssertRoles(t, "alice", []string{})
		})

		// A user has some roles, then on another oauth login will lose said
		// roles from an updated claim.
		t.Run("NewUserAndRemoveRolesOnReAuth", func(t *testing.T) {
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
			_, resp := runner.Login(t, jwt.MapClaims{
				"email": "alice@coder.com",
				"roles": []string{"random", oidcRoleName, rbac.RoleOwner()},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertRoles(t, "alice", []string{rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin(), rbac.RoleOwner()})

			// Now login with oauth again, and check the roles are removed.
			_, resp = runner.Login(t, jwt.MapClaims{
				"email": "alice@coder.com",
				"roles": []string{"random"},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)

			runner.AssertRoles(t, "alice", []string{})
		})

		// All manual role updates should fail when role sync is enabled.
		t.Run("BlockAssignRoles", func(t *testing.T) {
			t.Parallel()

			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.UserRoleField = "roles"
				},
			})

			_, resp := runner.Login(t, jwt.MapClaims{
				"email": "alice@coder.com",
				"roles": []string{},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			// Try to manually update user roles, even though controlled by oidc
			// role sync.
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := runner.AdminClient.UpdateUserRoles(ctx, "alice", codersdk.UpdateRoles{
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

		// Assigns does a simple test of assigning a user to a group based
		// on the oidc claims.
		t.Run("Assigns", func(t *testing.T) {
			t.Parallel()

			const groupClaim = "custom-groups"
			const groupName = "bingbong"
			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.GroupField = groupClaim
				},
			})

			ctx := testutil.Context(t, testutil.WaitShort)
			group, err := runner.AdminClient.CreateGroup(ctx, runner.AdminUser.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: groupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			_, resp := runner.Login(t, jwt.MapClaims{
				"email":    "alice@coder.com",
				groupClaim: []string{groupName},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertGroups(t, "alice", []string{groupName})
		})

		// Tests the group mapping feature.
		t.Run("AssignsMapped", func(t *testing.T) {
			t.Parallel()

			const groupClaim = "custom-groups"

			const oidcGroupName = "pingpong"
			const coderGroupName = "bingbong"
			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.GroupField = groupClaim
					cfg.GroupMapping = map[string]string{oidcGroupName: coderGroupName}
				},
			})

			ctx := testutil.Context(t, testutil.WaitShort)
			group, err := runner.AdminClient.CreateGroup(ctx, runner.AdminUser.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: coderGroupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			_, resp := runner.Login(t, jwt.MapClaims{
				"email":    "alice@coder.com",
				groupClaim: []string{oidcGroupName},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertGroups(t, "alice", []string{coderGroupName})
		})

		// User is in a group, then on an oauth refresh will lose said
		// group.
		t.Run("AddThenRemoveOnRefresh", func(t *testing.T) {
			t.Parallel()

			// TODO: Implement new feature to update roles/groups on OIDC
			// refresh tokens. https://github.com/coder/coder/issues/9312
			t.Skip("Refreshing tokens does not update groups :(")

			const groupClaim = "custom-groups"
			const groupName = "bingbong"
			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.GroupField = groupClaim
				},
			})

			ctx := testutil.Context(t, testutil.WaitShort)
			group, err := runner.AdminClient.CreateGroup(ctx, runner.AdminUser.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: groupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			client, resp := runner.Login(t, jwt.MapClaims{
				"email":    "alice@coder.com",
				groupClaim: []string{groupName},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertGroups(t, "alice", []string{groupName})

			// Refresh without the group claim
			runner.ForceRefresh(t, client, jwt.MapClaims{
				"email": "alice@coder.com",
			})
			runner.AssertGroups(t, "alice", []string{})
		})

		t.Run("AddThenRemoveOnReAuth", func(t *testing.T) {
			t.Parallel()

			const groupClaim = "custom-groups"
			const groupName = "bingbong"
			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.GroupField = groupClaim
				},
			})

			ctx := testutil.Context(t, testutil.WaitShort)
			group, err := runner.AdminClient.CreateGroup(ctx, runner.AdminUser.OrganizationIDs[0], codersdk.CreateGroupRequest{
				Name: groupName,
			})
			require.NoError(t, err)
			require.Len(t, group.Members, 0)

			_, resp := runner.Login(t, jwt.MapClaims{
				"email":    "alice@coder.com",
				groupClaim: []string{groupName},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertGroups(t, "alice", []string{groupName})

			// Refresh without the group claim
			_, resp = runner.Login(t, jwt.MapClaims{
				"email": "alice@coder.com",
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertGroups(t, "alice", []string{})
		})

		// Updating groups where the claimed group does not exist.
		t.Run("NoneMatch", func(t *testing.T) {
			t.Parallel()

			const groupClaim = "custom-groups"
			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.GroupField = groupClaim
				},
			})

			_, resp := runner.Login(t, jwt.MapClaims{
				"email":    "alice@coder.com",
				groupClaim: []string{"not-exists"},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertGroups(t, "alice", []string{})
		})

		// Updating groups where the claimed group does not exist creates
		// the group.
		t.Run("AutoCreate", func(t *testing.T) {
			t.Parallel()

			const groupClaim = "custom-groups"
			const groupName = "make-me"
			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
					cfg.GroupField = groupClaim
					cfg.CreateMissingGroups = true
				},
			})

			_, resp := runner.Login(t, jwt.MapClaims{
				"email":    "alice@coder.com",
				groupClaim: []string{groupName},
			})
			require.Equal(t, http.StatusOK, resp.StatusCode)
			runner.AssertGroups(t, "alice", []string{groupName})
		})
	})

	t.Run("Refresh", func(t *testing.T) {
		t.Run("RefreshTokensMultiple", func(t *testing.T) {
			t.Parallel()

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
			client, resp := runner.Login(t, claims)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Refresh multiple times.
			for i := 0; i < 3; i++ {
				runner.ForceRefresh(t, client, claims)
			}
		})

		t.Run("FailedRefresh", func(t *testing.T) {
			t.Parallel()

			runner := setupOIDCTest(t, oidcTestConfig{
				FakeOpts: []oidctest.FakeIDPOpt{
					oidctest.WithRefresh(func(_ string) error {
						// Always "expired" refresh token.
						return xerrors.New("refresh token is expired")
					}),
				},
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.AllowSignups = true
				},
			})

			claims := jwt.MapClaims{
				"email": "alice@coder.com",
			}
			// Login a new client that signs up
			client, resp := runner.Login(t, claims)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Expire the token, cause a refresh
			runner.ExpireOauthToken(t, client)

			// This should fail because the oauth token refresh should fail.
			_, err := client.User(context.Background(), codersdk.Me)
			require.Error(t, err)
			var apiError *codersdk.Error
			require.ErrorAs(t, err, &apiError)
			require.Equal(t, http.StatusUnauthorized, apiError.StatusCode())
			require.ErrorContains(t, apiError, "refresh")
		})
	})
}

// nolint:bodyclose
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
			runner := setupOIDCTest(t, oidcTestConfig{
				Config: func(cfg *coderd.OIDCConfig) {
					cfg.GroupField = "groups"
					tc.modCfg(cfg)
				},
			})

			// Setup
			ctx := testutil.Context(t, testutil.WaitLong)
			org := runner.AdminUser.OrganizationIDs[0]

			initialGroups := make(map[string]codersdk.Group)
			for _, group := range tc.initialOrgGroups {
				newGroup, err := runner.AdminClient.CreateGroup(ctx, org, codersdk.CreateGroupRequest{
					Name: group,
				})
				require.NoError(t, err)
				require.Len(t, newGroup.Members, 0)
				initialGroups[group] = newGroup
			}

			// Create the user and add them to their initial groups
			_, user := coderdtest.CreateAnotherUser(t, runner.AdminClient, org)
			for _, group := range tc.initialUserGroups {
				_, err := runner.AdminClient.PatchGroup(ctx, initialGroups[group].ID, codersdk.PatchGroupRequest{
					AddUsers: []string{user.ID.String()},
				})
				require.NoError(t, err)
			}

			// nolint:gocritic
			_, err := runner.API.Database.UpdateUserLoginType(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLoginTypeParams{
				NewLoginType: database.LoginTypeOIDC,
				UserID:       user.ID,
			})
			require.NoError(t, err, "user must be oidc type")

			// Log in the new user
			tc.claims["email"] = user.Email
			_, resp := runner.Login(t, tc.claims)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Check group sources
			orgGroups, err := runner.AdminClient.GroupsByOrganization(ctx, org)
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

// oidcTestRunner is just a helper to setup and run oidc tests.
// An actual Coderd instance is used to run the tests.
type oidcTestRunner struct {
	AdminClient *codersdk.Client
	AdminUser   codersdk.User
	API         *coderden.API

	// Login will call the OIDC flow with an unauthenticated client.
	// The IDP will return the idToken claims.
	Login func(t *testing.T, idToken jwt.MapClaims) (*codersdk.Client, *http.Response)
	// ForceRefresh will use an authenticated codersdk.Client, and force their
	// OIDC token to be expired and require a refresh. The refresh will use the claims provided.
	// It just calls the /users/me endpoint to trigger the refresh.
	ForceRefresh     func(t *testing.T, client *codersdk.Client, idToken jwt.MapClaims)
	ExpireOauthToken func(t *testing.T, client *codersdk.Client)
}

type oidcTestConfig struct {
	Userinfo jwt.MapClaims

	// Config allows modifying the Coderd OIDC configuration.
	Config   func(cfg *coderd.OIDCConfig)
	FakeOpts []oidctest.FakeIDPOpt
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
	t.Helper()

	if !slice.Contains(groups, database.EveryoneGroup) {
		var cpy []string
		cpy = append(cpy, groups...)
		// always include everyone group
		cpy = append(cpy, database.EveryoneGroup)
		groups = cpy
	}
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
		append([]oidctest.FakeIDPOpt{
			oidctest.WithStaticUserInfo(settings.Userinfo),
			oidctest.WithLogging(t, nil),
			// Run fake IDP on a real webserver
			oidctest.WithServing(),
		}, settings.FakeOpts...)...,
	)

	ctx := testutil.Context(t, testutil.WaitMedium)
	cfg := fake.OIDCConfig(t, nil, settings.Config)
	owner, _, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			OIDCConfig: cfg,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureUserRoleManagement: 1,
				codersdk.FeatureTemplateRBAC:       1,
			},
		},
	})
	admin, err := owner.User(ctx, "me")
	require.NoError(t, err)

	helper := oidctest.NewLoginHelper(owner, fake)

	return &oidcTestRunner{
		AdminClient: owner,
		AdminUser:   admin,
		API:         api,
		Login:       helper.Login,
		ForceRefresh: func(t *testing.T, client *codersdk.Client, idToken jwt.MapClaims) {
			helper.ForceRefresh(t, api.Database, client, idToken)
		},
		ExpireOauthToken: func(t *testing.T, client *codersdk.Client) {
			helper.ExpireOauthToken(t, api.Database, client)
		},
	}
}
