package coderd_test

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:      "hi",
			AvatarURL: "https://example.com",
		})
		require.NoError(t, err)
		require.Equal(t, "hi", group.Name)
		require.Equal(t, "https://example.com", group.AvatarURL)
		require.Empty(t, group.Members)
		require.Empty(t, group.DisplayName)
		require.NotEqual(t, uuid.Nil.String(), group.ID.String())
	})

	t.Run("Audit", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging: true,
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Auditor:                  auditor,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
					codersdk.FeatureAuditLog:     1,
				},
			},
		})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())

		ctx := testutil.Context(t, testutil.WaitLong)

		numLogs := len(auditor.AuditLogs())
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)
		numLogs++
		require.Len(t, auditor.AuditLogs(), numLogs)
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:     database.AuditActionCreate,
			ResourceID: group.ID,
		}))
	})

	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		_, err = userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusConflict, cerr.StatusCode())
	})

	t.Run("ReservedName", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "new",
		})

		require.Error(t, err)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("allUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: database.EveryoneGroup,
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})
}

func TestPatchGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		const displayName = "foobar"
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "ff7dcee2-e7c4-4bc4-a9e4-84870770e4c5", // GUID should fit.
			AvatarURL:      "https://example.com",
			QuotaAllowance: 10,
			DisplayName:    "",
		})
		require.NoError(t, err)
		require.Equal(t, 10, group.QuotaAllowance)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name:           "ddd502d2-2984-4724-b5bf-1109a4d7462d", // GUID should fit.
			AvatarURL:      ptr.Ref("https://google.com"),
			QuotaAllowance: ptr.Ref(20),
			DisplayName:    ptr.Ref(displayName),
		})
		require.NoError(t, err)
		require.Equal(t, displayName, group.DisplayName)
		require.Equal(t, "ddd502d2-2984-4724-b5bf-1109a4d7462d", group.Name)
		require.Equal(t, "https://google.com", group.AvatarURL)
		require.Equal(t, 20, group.QuotaAllowance)
	})

	t.Run("DisplayNameUnchanged", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		const displayName = "foobar"
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "hi",
			AvatarURL:      "https://example.com",
			QuotaAllowance: 10,
			DisplayName:    displayName,
		})
		require.NoError(t, err)
		require.Equal(t, 10, group.QuotaAllowance)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name:           "bye",
			AvatarURL:      ptr.Ref("https://google.com"),
			QuotaAllowance: ptr.Ref(20),
		})
		require.NoError(t, err)
		require.Equal(t, displayName, group.DisplayName)
		require.Equal(t, "bye", group.Name)
		require.Equal(t, "https://google.com", group.AvatarURL)
		require.Equal(t, 20, group.QuotaAllowance)
	})

	// The FE sends a request from the edit page where the old name == new name.
	// This should pass since it's not really an error to update a group name
	// to itself.
	t.Run("SameNameOK", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)
		require.Equal(t, "hi", group.Name)
	})

	t.Run("RenameCasingOnlyOK", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "supportshare",
		})
		require.NoError(t, err)

		// GetGroupByOrgAndName now matches names case-insensitively, so the
		// conflict check must exclude the group being renamed. Otherwise it
		// would find itself and reject a rename that only changes casing.
		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name: "SupportShare",
		})
		require.NoError(t, err)
		require.Equal(t, "SupportShare", group.Name)
	})

	t.Run("AddUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user2.ReducedUser)
		require.Contains(t, group.Members, user3.ReducedUser)
	})

	t.Run("RemoveUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user4 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String(), user4.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user2.ReducedUser)
		require.Contains(t, group.Members, user3.ReducedUser)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			RemoveUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)
		require.NotContains(t, group.Members, user2.ReducedUser)
		require.NotContains(t, group.Members, user3.ReducedUser)
		require.Contains(t, group.Members, user4.ReducedUser)
	})

	t.Run("Audit", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging: true,
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Auditor:                  auditor,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
					codersdk.FeatureAuditLog:     1,
				},
			},
		})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)

		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		numLogs := len(auditor.AuditLogs())
		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name: "bye",
		})
		require.NoError(t, err)
		numLogs++

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[numLogs-1].Action)
		require.Equal(t, group.ID, auditor.AuditLogs()[numLogs-1].ResourceID)
	})
	t.Run("NameConflict", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group1, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:      "hi",
			AvatarURL: "https://example.com",
		})
		require.NoError(t, err)

		group2, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "bye",
		})
		require.NoError(t, err)

		group1, err = userAdminClient.PatchGroup(ctx, group1.ID, codersdk.PatchGroupRequest{
			Name:      group2.Name,
			AvatarURL: ptr.Ref("https://google.com"),
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusConflict, cerr.StatusCode())
	})

	t.Run("UserNotExist", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{uuid.NewString()},
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("MalformedUUID", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{"yeet"},
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("AddDuplicateUser", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user2.ID.String()},
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)

		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("ReservedName", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name: database.EveryoneGroup,
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	// For quotas to work with prebuilds, it's currently required to add the
	// prebuilds user into a group with a quota allowance.
	// See: docs/admin/templates/extending-templates/prebuilt-workspaces.md
	t.Run("PrebuildsUser", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:           "prebuilds",
			QuotaAllowance: 123,
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name:     "prebuilds",
			AddUsers: []string{database.PrebuildsSystemUserID.String()},
		})
		require.NoError(t, err)
	})

	t.Run("Everyone", func(t *testing.T) {
		t.Parallel()
		t.Run("NoUpdateName", func(t *testing.T) {
			t.Parallel()

			client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			}})
			userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := userAdminClient.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
				Name: "hi",
			})
			require.Error(t, err)
			cerr, ok := codersdk.AsError(err)
			require.True(t, ok)
			require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
		})

		t.Run("NoUpdateDisplayName", func(t *testing.T) {
			t.Parallel()

			client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			}})
			userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := userAdminClient.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
				DisplayName: ptr.Ref("hi"),
			})
			require.Error(t, err)
			cerr, ok := codersdk.AsError(err)
			require.True(t, ok)
			require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
		})

		t.Run("NoAddUsers", func(t *testing.T) {
			t.Parallel()

			client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			}})
			userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
			_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := userAdminClient.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
				AddUsers: []string{user2.ID.String()},
			})
			require.Error(t, err)
			cerr, ok := codersdk.AsError(err)
			require.True(t, ok)
			require.Equal(t, http.StatusForbidden, cerr.StatusCode())
		})

		t.Run("NoRmUsers", func(t *testing.T) {
			t.Parallel()

			client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			}})
			userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())

			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := userAdminClient.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
				RemoveUsers: []string{user.UserID.String()},
			})
			require.Error(t, err)
			cerr, ok := codersdk.AsError(err)
			require.True(t, ok)
			require.Equal(t, http.StatusForbidden, cerr.StatusCode())
		})

		t.Run("UpdateQuota", func(t *testing.T) {
			t.Parallel()

			client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			}})
			userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())

			ctx := testutil.Context(t, testutil.WaitLong)
			group, err := userAdminClient.Group(ctx, user.OrganizationID, codersdk.GroupRequest{})
			require.NoError(t, err)

			require.Equal(t, 0, group.QuotaAllowance)

			expectedQuota := 123
			group, err = userAdminClient.PatchGroup(ctx, user.OrganizationID, codersdk.PatchGroupRequest{
				QuotaAllowance: ptr.Ref(expectedQuota),
			})
			require.NoError(t, err)
			require.Equal(t, expectedQuota, group.QuotaAllowance)
		})
	})
}

func normalizeAllGroups(groups []codersdk.Group) {
	for i := range groups {
		normalizeGroupMembers(&groups[i])
	}
}

// normalizeGroupMembers removes comparison noise from the group members.
func normalizeGroupMembers(group *codersdk.Group) {
	for i := range group.Members {
		group.Members[i].LastSeenAt = time.Time{}
		group.Members[i].CreatedAt = time.Time{}
		group.Members[i].UpdatedAt = time.Time{}
	}
	sort.Slice(group.Members, func(i, j int) bool {
		return group.Members[i].ID.String() < group.Members[j].ID.String()
	})
}

// TODO: test auth.
func TestGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		ggroup, err := userAdminClient.Group(ctx, group.ID, codersdk.GroupRequest{})
		require.NoError(t, err)
		require.Equal(t, group, ggroup)
	})

	t.Run("ByName", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		ggroup, err := userAdminClient.GroupByOrgAndName(ctx, group.OrganizationID, group.Name)
		require.NoError(t, err)
		require.Equal(t, group, ggroup)
	})

	t.Run("WithUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user2.ReducedUser)
		require.Contains(t, group.Members, user3.ReducedUser)

		ggroup, err := userAdminClient.Group(ctx, group.ID, codersdk.GroupRequest{})
		require.NoError(t, err)
		normalizeGroupMembers(&group)
		normalizeGroupMembers(&ggroup)

		require.Equal(t, group, ggroup)
	})

	t.Run("WithoutMembers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user2.ReducedUser)
		require.Contains(t, group.Members, user3.ReducedUser)

		ggroup, err := userAdminClient.Group(ctx, group.ID, codersdk.GroupRequest{
			ExcludeMembers: true,
		})
		require.NoError(t, err)
		require.Len(t, ggroup.Members, 0)
	})

	t.Run("RegularUserReadGroup", func(t *testing.T) {
		t.Parallel()

		t.Run("WorkspaceSharingEnabled", func(t *testing.T) {
			t.Parallel()

			client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			}})
			client1, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

			ctx := testutil.Context(t, testutil.WaitLong)
			//nolint:gocritic // test setup
			group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
				Name: "hi",
			})
			require.NoError(t, err)

			ggroup, err := client1.Group(ctx, group.ID, codersdk.GroupRequest{})
			require.NoError(t, err, "regular users can read groups unless workspace sharing is disabled")
			normalizeGroupMembers(&group)
			normalizeGroupMembers(&ggroup)
			require.Equal(t, group, ggroup)
		})

		t.Run("WorkspaceSharingDisabled", func(t *testing.T) {
			t.Parallel()

			db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
			client, _, api, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Database: db,
					Pubsub:   ps,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := sqlDB.ExecContext(ctx, "UPDATE organizations SET shareable_workspace_owners = 'none' WHERE id = $1", user.OrganizationID)
			require.NoError(t, err)

			//nolint:gocritic // ReconcileOrgMemberRole needs the system:update
			// permission that the test context doesn't have.
			sysCtx := dbauthz.AsSystemRestricted(ctx)
			_, _, err = rolestore.ReconcileSystemRole(sysCtx, api.Database, database.CustomRole{
				Name: rbac.RoleOrgMember(),
				OrganizationID: uuid.NullUUID{
					UUID:  user.OrganizationID,
					Valid: true,
				},
			}, database.Organization{ShareableWorkspaceOwners: database.ShareableWorkspaceOwnersNone})
			require.NoError(t, err)

			client1, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

			//nolint:gocritic // test setup
			group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
				Name: "hi",
			})
			require.NoError(t, err)

			_, err = client1.Group(ctx, group.ID, codersdk.GroupRequest{})
			require.Error(t, err, "regular users cannot read groups when workspace sharing is disabled")
			cerr, ok := codersdk.AsError(err)
			require.True(t, ok)
			require.Equal(t, http.StatusNotFound, cerr.StatusCode())
		})
	})

	t.Run("FilterDeletedUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())

		_, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user1.ID.String(), user2.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user1.ReducedUser)
		require.Contains(t, group.Members, user2.ReducedUser)

		err = userAdminClient.DeleteUser(ctx, user1.ID)
		require.NoError(t, err)

		group, err = userAdminClient.Group(ctx, group.ID, codersdk.GroupRequest{})
		require.NoError(t, err)
		require.NotContains(t, group.Members, user1.ReducedUser)
	})

	t.Run("IncludeSuspendedAndDormantUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())

		_, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user1.ID.String(), user2.ID.String()},
		})
		require.NoError(t, err)
		require.Len(t, group.Members, 2)
		require.Contains(t, group.Members, user1.ReducedUser)
		require.Contains(t, group.Members, user2.ReducedUser)

		user1, err = userAdminClient.UpdateUserStatus(ctx, user1.ID.String(), codersdk.UserStatusSuspended)
		require.NoError(t, err)

		group, err = userAdminClient.Group(ctx, group.ID, codersdk.GroupRequest{})
		require.NoError(t, err)
		require.Len(t, group.Members, 2)
		require.Contains(t, group.Members, user1.ReducedUser)
		require.Contains(t, group.Members, user2.ReducedUser)

		// cannot explicitly set a dormant user status so must create a new user
		anotherUser, err := userAdminClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "coder@coder.com",
			Username:        "coder",
			Password:        "SomeStrongPassword!",
			OrganizationIDs: []uuid.UUID{user.OrganizationID},
		})
		require.NoError(t, err)

		// Ensure that new user has dormant account
		require.Equal(t, codersdk.UserStatusDormant, anotherUser.Status)

		group, _ = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{anotherUser.ID.String()},
		})

		group, err = userAdminClient.Group(ctx, group.ID, codersdk.GroupRequest{})
		require.NoError(t, err)
		require.Len(t, group.Members, 3)
		require.Contains(t, group.Members, user1.ReducedUser)
		require.Contains(t, group.Members, user2.ReducedUser)
	})

	t.Run("ByIDs", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())

		ctx := testutil.Context(t, testutil.WaitLong)
		groupA, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "group-a",
		})
		require.NoError(t, err)

		groupB, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "group-b",
		})
		require.NoError(t, err)

		// group-c should be omitted from the filter
		_, err = userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "group-c",
		})
		require.NoError(t, err)

		found, err := userAdminClient.Groups(ctx, codersdk.GroupArguments{
			GroupIDs: []uuid.UUID{groupA.ID, groupB.ID},
		})
		require.NoError(t, err)

		foundIDs := slice.List(found, func(g codersdk.Group) uuid.UUID {
			return g.ID
		})

		require.ElementsMatch(t, []uuid.UUID{groupA.ID, groupB.ID}, foundIDs)
	})

	t.Run("everyoneGroupReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		_, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		// nolint:gocritic // "This client is operating as the owner user" is fine in this case.
		prebuildsUser, err := client.User(ctx, database.PrebuildsSystemUserID.String())
		require.NoError(t, err)
		// The 'Everyone' group always has an ID that matches the organization ID.
		group, err := userAdminClient.Group(ctx, user.OrganizationID, codersdk.GroupRequest{})
		require.NoError(t, err)
		require.Len(t, group.Members, 4)
		require.Equal(t, "Everyone", group.Name)
		require.Equal(t, user.OrganizationID, group.OrganizationID)
		require.Contains(t, group.Members, user1.ReducedUser)
		require.Contains(t, group.Members, user2.ReducedUser)
		require.NotContains(t, group.Members, prebuildsUser.ReducedUser)
	})
}

// TODO: test auth.
func TestGroups(t *testing.T) {
	t.Parallel()

	// 5 users
	// 2 custom groups + original org group
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user4 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		user5Client, user5 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group1, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group2, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hey",
		})
		require.NoError(t, err)

		group1, err = userAdminClient.PatchGroup(ctx, group1.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)
		normalizeGroupMembers(&group1)

		group2, err = userAdminClient.PatchGroup(ctx, group2.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user4.ID.String(), user5.ID.String()},
		})
		require.NoError(t, err)
		normalizeGroupMembers(&group2)

		// Fetch everyone group for comparison
		everyoneGroup, err := userAdminClient.Group(ctx, user.OrganizationID, codersdk.GroupRequest{})
		require.NoError(t, err)
		normalizeGroupMembers(&everyoneGroup)

		groups, err := userAdminClient.Groups(ctx, codersdk.GroupArguments{
			Organization: user.OrganizationID.String(),
		})
		require.NoError(t, err)
		normalizeAllGroups(groups)

		// 'Everyone' group + 2 custom groups.
		require.ElementsMatch(t, []codersdk.Group{
			everyoneGroup,
			group1,
			group2,
		}, groups)

		// Filter by user
		user5Groups, err := userAdminClient.Groups(ctx, codersdk.GroupArguments{
			HasMember: user5.Username,
		})
		require.NoError(t, err)
		normalizeAllGroups(user5Groups)
		// Everyone group and group 2
		require.ElementsMatch(t, []codersdk.Group{
			everyoneGroup,
			group2,
		}, user5Groups)

		// Query from the user's perspective
		user5View, err := user5Client.Groups(ctx, codersdk.GroupArguments{})
		require.NoError(t, err)
		normalizeAllGroups(user5View)

		// Org members can read all groups when workspace sharing is not
		// disabled, but group membership is limited to the requesting user.
		// TODO(geokat): add another test with workspace sharing disabled.
		require.Len(t, user5View, 3)
		user5ViewIDs := slice.List(user5View, func(g codersdk.Group) uuid.UUID {
			return g.ID
		})

		require.ElementsMatch(t, []uuid.UUID{
			everyoneGroup.ID,
			group1.ID,
			group2.ID,
		}, user5ViewIDs)
		for _, g := range user5View {
			if g.ID == everyoneGroup.ID || g.ID == group2.ID {
				// Only expect the 1 member, themselves.
				require.Len(t, g.Members, 1)
				require.Equal(t, user5.ReducedUser.ID, g.Members[0].MinimalUser.ID)
				continue
			}

			require.Empty(t, g.Members)
		}
	})
}

func TestDeleteGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group1, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		err = userAdminClient.DeleteGroup(ctx, group1.ID)
		require.NoError(t, err)

		_, err = userAdminClient.Group(ctx, group1.ID, codersdk.GroupRequest{})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())
	})

	t.Run("Audit", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging: true,
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Auditor:                  auditor,
			},
		})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAuditLog:     1,
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		group, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		numLogs := len(auditor.AuditLogs())
		err = userAdminClient.DeleteGroup(ctx, group.ID)
		require.NoError(t, err)
		numLogs++

		require.Len(t, auditor.AuditLogs(), numLogs)
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:     database.AuditActionDelete,
			ResourceID: group.ID,
		}))
	})

	t.Run("allUsers", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		err := userAdminClient.DeleteGroup(ctx, user.OrganizationID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})
}

func TestGetGroupMembersFilter(t *testing.T) {
	t.Parallel()

	client, db, first := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			OIDCConfig: &coderd.OIDCConfig{
				AllowSignups: true,
			},
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC:    1,
				codersdk.FeatureServiceAccounts: 1,
			},
		},
	})

	userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleUserAdmin())

	setupCtx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	group, err := userAdminClient.CreateGroup(setupCtx, first.OrganizationID, codersdk.CreateGroupRequest{
		Name: "filtered",
	})
	require.NoError(t, err)

	setup := func(users []codersdk.User) {
		userIDs := make([]string, len(users))
		for i, user := range users {
			userIDs[i] = user.ID.String()
		}
		group, err = userAdminClient.PatchGroup(setupCtx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: userIDs,
		})
		require.NoError(t, err)
	}
	fetch := func(testCtx context.Context, req codersdk.UsersRequest) []codersdk.ReducedUser {
		res, err := userAdminClient.GroupMembers(testCtx, group.ID, req)
		require.NoError(t, err)
		return res.Users
	}
	options := &coderdtest.UsersFilterOptions{CreateServiceAccounts: true}
	coderdtest.UsersFilter(setupCtx, t, client, db, options, setup, fetch)
}

func TestGetGroupMembersPagination(t *testing.T) {
	t.Parallel()

	client, first := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		},
	})

	userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.RoleUserAdmin())

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	group, err := userAdminClient.CreateGroup(ctx, first.OrganizationID, codersdk.CreateGroupRequest{
		Name: "paginated",
	})
	require.NoError(t, err)

	setup := func(users []codersdk.User) {
		userIDs := make([]string, len(users))
		for i, user := range users {
			userIDs[i] = user.ID.String()
		}
		group, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: userIDs,
		})
		require.NoError(t, err)
	}
	fetch := func(req codersdk.UsersRequest) ([]codersdk.ReducedUser, int) {
		group, err := userAdminClient.GroupMembers(ctx, group.ID, req)
		require.NoError(t, err)
		return group.Users, group.Count
	}
	coderdtest.UsersPagination(ctx, t, client, setup, fetch)
}

func TestPaginatedGroups(t *testing.T) {
	t.Parallel()

	client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureTemplateRBAC:          1,
			codersdk.FeatureMultipleOrganizations: 1,
		},
	}})
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
	ctx := testutil.Context(t, testutil.WaitLong)

	// Create a deterministic set of groups. Names include mixed case, a pair
	// that differs only by case, and one group with a distinct display name so
	// ordering (LOWER(name)), the groups.id tiebreaker, and display-name search
	// are all exercised. The org's implicit "Everyone" group also exists, so
	// account for it in the expected counts.
	type groupSpec struct {
		name        string
		displayName string
	}
	specs := []groupSpec{
		{name: "alpha"},
		{name: "Bravo"},
		{name: "charlie"},
		{name: "Delta"},
		{name: "echo"},
		// "Dev" and "dev" collide once lowercased, forcing the groups.id
		// tiebreaker to produce a deterministic order.
		{name: "Dev"},
		{name: "dev"},
		{name: "zeta", displayName: "Frontend Squad"},
		// A display name with a colon is searchable via a quoted search value.
		{name: "team-fe", displayName: "Team: Frontend"},
	}
	for _, spec := range specs {
		_, err := userAdminClient.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name:        spec.name,
			DisplayName: spec.displayName,
		})
		require.NoError(t, err)
	}

	// The org's implicit "Everyone" group is included in the paginated results.
	totalGroups := len(specs) + 1

	// Add a known member to the "alpha" group so member hydration can be
	// asserted below.
	_, member := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
	alpha, err := userAdminClient.GroupByOrgAndName(ctx, user.OrganizationID, "alpha")
	require.NoError(t, err)
	_, err = userAdminClient.PatchGroup(ctx, alpha.ID, codersdk.PatchGroupRequest{
		AddUsers: []string{member.ID.String()},
	})
	require.NoError(t, err)

	t.Run("AllGroups", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{})
		require.NoError(t, err)
		require.Equal(t, totalGroups, resp.Count)
		require.Len(t, resp.Groups, totalGroups)

		// Verify deterministic ordering: lower(name) ascending, with ties
		// broken by groups.id ascending. uuid string comparison matches
		// Postgres' byte-wise uuid ordering.
		for i := 1; i < len(resp.Groups); i++ {
			prev, cur := resp.Groups[i-1], resp.Groups[i]
			prevName, curName := strings.ToLower(prev.Name), strings.ToLower(cur.Name)
			if prevName == curName {
				require.Less(t, prev.ID.String(), cur.ID.String(),
					"groups with equal lowercased names must be ordered by id")
			} else {
				require.Less(t, prevName, curName,
					"groups must be ordered by lowercased name")
			}
		}
	})

	t.Run("MemberCount", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// The list endpoint returns each group's total member count but does
		// not hydrate the member roster; callers page members separately via
		// the group members endpoint. Assert the count is populated. The
		// roster is omitted entirely: the slim PaginatedGroup type has no
		// Members field, so re-adding roster hydration would fail to compile.
		resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
			SearchQuery: "alpha",
		})
		require.NoError(t, err)
		require.Len(t, resp.Groups, 1)
		require.Equal(t, "alpha", resp.Groups[0].Name)
		require.Equal(t, 1, resp.Groups[0].TotalMemberCount)
	})

	t.Run("Search", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
			SearchQuery: "alpha",
		})
		require.NoError(t, err)
		require.Equal(t, 1, resp.Count)
		require.Len(t, resp.Groups, 1)
		require.Equal(t, "alpha", resp.Groups[0].Name)
	})

	t.Run("SearchNoResults", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
			SearchQuery: "does-not-exist",
		})
		require.NoError(t, err)
		require.Equal(t, 0, resp.Count)
		require.Empty(t, resp.Groups)
	})

	t.Run("SearchSubstring", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// A substring of the name matches, not just a prefix.
		resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
			SearchQuery: "harl",
		})
		require.NoError(t, err)
		require.Equal(t, 1, resp.Count)
		require.Len(t, resp.Groups, 1)
		require.Equal(t, "charlie", resp.Groups[0].Name)
	})

	t.Run("SearchCaseInsensitive", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// An uppercase query matches both "Dev" and "dev" case-insensitively.
		resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
			SearchQuery: "DEV",
		})
		require.NoError(t, err)
		require.Equal(t, 2, resp.Count)
		require.Len(t, resp.Groups, 2)
		for _, g := range resp.Groups {
			require.Equal(t, "dev", strings.ToLower(g.Name))
		}
		// The case-only collision is ordered deterministically by id.
		require.Less(t, resp.Groups[0].ID.String(), resp.Groups[1].ID.String())
	})

	t.Run("SearchDisplayName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Search matches the display name, not just the name.
		resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
			SearchQuery: "squad",
		})
		require.NoError(t, err)
		require.Equal(t, 1, resp.Count)
		require.Len(t, resp.Groups, 1)
		require.Equal(t, "zeta", resp.Groups[0].Name)
	})

	t.Run("SearchColonValue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// A display name containing a colon is searchable when the value is
		// quoted via the search key, since an unquoted colon is a key:value
		// delimiter.
		resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
			SearchQuery: `search:"team: frontend"`,
		})
		require.NoError(t, err)
		require.Equal(t, 1, resp.Count)
		require.Len(t, resp.Groups, 1)
		require.Equal(t, "team-fe", resp.Groups[0].Name)
	})

	t.Run("PageBoundaries", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Page through the results two at a time and ensure the union covers
		// every group exactly once, with a stable Count on each page.
		seen := make(map[string]struct{})
		for offset := 0; offset < totalGroups; offset += 2 {
			resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
				Pagination: codersdk.Pagination{Limit: 2, Offset: offset},
			})
			require.NoError(t, err)
			require.Equal(t, totalGroups, resp.Count)
			require.LessOrEqual(t, len(resp.Groups), 2)
			for _, g := range resp.Groups {
				_, dup := seen[g.Name]
				require.False(t, dup, "group %q appeared on more than one page", g.Name)
				seen[g.Name] = struct{}{}
			}
		}
		require.Len(t, seen, totalGroups)
	})

	t.Run("AfterIDCursor", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Page through the results using after_id as a keyset cursor. The union
		// must cover every group exactly once, in the same deterministic
		// (LOWER(name), id) order, with no duplicates even across the
		// "Dev"/"dev" case collision that relies on the id tiebreaker.
		seen := make(map[uuid.UUID]struct{})
		var after uuid.UUID
		var prevName string
		var prevID uuid.UUID
		havePrev := false
		for {
			resp, err := userAdminClient.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{
				Pagination: codersdk.Pagination{Limit: 2, AfterID: after},
			})
			require.NoError(t, err)
			if len(resp.Groups) == 0 {
				break
			}
			require.LessOrEqual(t, len(resp.Groups), 2)
			for _, g := range resp.Groups {
				_, dup := seen[g.ID]
				require.False(t, dup, "group %q returned on more than one page", g.Name)
				seen[g.ID] = struct{}{}

				name := strings.ToLower(g.Name)
				if havePrev {
					if name == prevName {
						require.Less(t, prevID.String(), g.ID.String(),
							"ties must advance by id")
					} else {
						require.Less(t, prevName, name,
							"groups must stay ordered by lowercased name")
					}
				}
				prevName, prevID, havePrev = name, g.ID, true
			}
			after = resp.Groups[len(resp.Groups)-1].ID
		}
		require.Len(t, seen, totalGroups)
	})

	t.Run("OrganizationIsolation", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// A second organization with its own group must never appear in the
		// first org's results, and the first org's Count must exclude it. This
		// exercises the organization_id filter for exclusion, which a
		// single-org test cannot.
		//nolint:gocritic // Only owners can create organizations.
		otherOrg, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name: "other-org",
		})
		require.NoError(t, err)
		// Reuse a name that also exists in the first org to prove isolation is
		// by organization, not by name.
		otherGroup, err := client.CreateGroup(ctx, otherOrg.ID, codersdk.CreateGroupRequest{
			Name: "alpha",
		})
		require.NoError(t, err)

		resp, err := client.OrganizationGroupsPaginated(ctx, user.OrganizationID, codersdk.PaginatedGroupsRequest{})
		require.NoError(t, err)
		require.Equal(t, totalGroups, resp.Count)
		for _, g := range resp.Groups {
			require.Equal(t, user.OrganizationID, g.OrganizationID)
			require.NotEqual(t, otherGroup.ID, g.ID)
		}

		// The second org returns only its own groups: the created group plus
		// that org's implicit "Everyone" group.
		otherResp, err := client.OrganizationGroupsPaginated(ctx, otherOrg.ID, codersdk.PaginatedGroupsRequest{})
		require.NoError(t, err)
		require.Equal(t, 2, otherResp.Count)
		for _, g := range otherResp.Groups {
			require.Equal(t, otherOrg.ID, g.OrganizationID)
		}
	})
}
