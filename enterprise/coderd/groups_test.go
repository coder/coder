package coderd_test

import (
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
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
			group, err := userAdminClient.Group(ctx, user.OrganizationID)
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

		ggroup, err := userAdminClient.Group(ctx, group.ID)
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

		ggroup, err := userAdminClient.Group(ctx, group.ID)
		require.NoError(t, err)
		normalizeGroupMembers(&group)
		normalizeGroupMembers(&ggroup)

		require.Equal(t, group, ggroup)
	})

	t.Run("RegularUserReadGroup", func(t *testing.T) {
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

		_, err = client1.Group(ctx, group.ID)
		require.Error(t, err, "regular users cannot read groups")
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

		group, err = userAdminClient.Group(ctx, group.ID)
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

		group, err = userAdminClient.Group(ctx, group.ID)
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

		group, err = userAdminClient.Group(ctx, group.ID)
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

		foundIDs := db2sdk.List(found, func(g codersdk.Group) uuid.UUID {
			return g.ID
		})

		require.ElementsMatch(t, []uuid.UUID{groupA.ID, groupB.ID}, foundIDs)
	})

	t.Run("everyoneGroupReturnsEmpty", func(t *testing.T) {
		// TODO (sasswart): this test seems to have drifted from its original intention. evaluate and remove/fix
		t.Parallel()

		// TODO: we should not be returning the prebuilds user in Group, and this is not returned in dbmem.
		if !dbtestutil.WillUsePostgres() {
			t.Skip("This test requires postgres")
		}

		client, user := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		}})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleUserAdmin())
		_, user1 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUser(t, client, user.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		prebuildsUser, err := client.User(ctx, prebuilds.OwnerID.String())
		require.NoError(t, err)
		// The 'Everyone' group always has an ID that matches the organization ID.
		group, err := userAdminClient.Group(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, group.Members, 5)
		require.Equal(t, "Everyone", group.Name)
		require.Equal(t, user.OrganizationID, group.OrganizationID)
		require.Contains(t, group.Members, user1.ReducedUser)
		require.Contains(t, group.Members, user2.ReducedUser)
		require.Contains(t, group.Members, prebuildsUser.ReducedUser)
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
		everyoneGroup, err := userAdminClient.Group(ctx, user.OrganizationID)
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
		normalizeAllGroups(user5Groups)

		// Everyone group and group 2
		require.Len(t, user5View, 2)
		user5ViewIDs := db2sdk.List(user5View, func(g codersdk.Group) uuid.UUID {
			return g.ID
		})

		require.ElementsMatch(t, []uuid.UUID{
			everyoneGroup.ID,
			group2.ID,
		}, user5ViewIDs)
		for _, g := range user5View {
			// Only expect the 1 member, themselves
			require.Len(t, g.Members, 1)
			require.Equal(t, user5.ReducedUser.ID, g.Members[0].MinimalUser.ID)
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

		_, err = userAdminClient.Group(ctx, group1.ID)
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
