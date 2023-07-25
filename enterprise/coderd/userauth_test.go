package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/coder/coder/enterprise/coderd/license"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
)

// nolint:bodyclose
func TestUserOIDC(t *testing.T) {
	t.Parallel()
	t.Run("RoleSync", func(t *testing.T) {
		t.Parallel()

		t.Run("NoRoles", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			conf := coderdtest.NewOIDCConfig(t, "")

			oidcRoleName := "TemplateAuthor"

			config := conf.OIDCConfig(t, jwt.MapClaims{}, func(cfg *coderd.OIDCConfig) {
				cfg.UserRoleMapping = map[string][]string{oidcRoleName: {rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin()}}
			})
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
			}))
			require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
			user, err := client.User(ctx, "alice")
			require.NoError(t, err)

			require.Len(t, user.Roles, 0)
			roleNames := []string{}
			require.ElementsMatch(t, roleNames, []string{})
		})

		t.Run("NewUserAndRemoveRoles", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			conf := coderdtest.NewOIDCConfig(t, "")

			oidcRoleName := "TemplateAuthor"

			config := conf.OIDCConfig(t, jwt.MapClaims{}, func(cfg *coderd.OIDCConfig) {
				cfg.UserRoleMapping = map[string][]string{oidcRoleName: {rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin()}}
			})
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
				"roles": []string{"random", oidcRoleName, rbac.RoleOwner()},
			}))
			require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
			user, err := client.User(ctx, "alice")
			require.NoError(t, err)

			require.Len(t, user.Roles, 3)
			roleNames := []string{user.Roles[0].Name, user.Roles[1].Name, user.Roles[2].Name}
			require.ElementsMatch(t, roleNames, []string{rbac.RoleTemplateAdmin(), rbac.RoleUserAdmin(), rbac.RoleOwner()})

			// Now remove the roles with a new oidc login
			resp = oidcCallback(t, client, conf.EncodeClaims(t, jwt.MapClaims{
				"email": "alice@coder.com",
				"roles": []string{"random"},
			}))
			require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
			user, err = client.User(ctx, "alice")
			require.NoError(t, err)

			require.Len(t, user.Roles, 0)
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
