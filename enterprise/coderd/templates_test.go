package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/testutil"
)

func TestTemplateACL(t *testing.T) {
	t.Parallel()

	t.Run("UserRoles", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleUse,
				user3.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		templateUser2 := codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleUse,
		}

		templateUser3 := codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleAdmin,
		}

		require.Len(t, acl.Users, 2)
		require.Contains(t, acl.Users, templateUser2)
		require.Contains(t, acl.Users, templateUser3)
	})

	t.Run("allUsersGroup", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		_, user1 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Groups[0].Members, 2)
		require.Contains(t, acl.Groups[0].Members, user1)
		require.Len(t, acl.Users, 0)
	})

	t.Run("NoGroups", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		client1, _ := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)

		// User should be able to read template due to allUsers group.
		_, err = client1.Template(ctx, template.ID)
		require.NoError(t, err)

		allUsers := acl.Groups[0]

		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				allUsers.ID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 0)
		require.Len(t, acl.Users, 0)

		// User should not be able to read template due to allUsers group being deleted.
		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())
	})

	// Test that we do not return deleted users.
	t.Run("FilterDeletedUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		_, user1 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user1.ID.String(): codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user1,
			Role: codersdk.TemplateRoleUse,
		})

		err = client.DeleteUser(ctx, user1.ID)
		require.NoError(t, err)

		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 0, "deleted users should be filtered")
	})

	// Test that we do not return suspended users.
	t.Run("FilterSuspendedUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		_, user1 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user1.ID.String(): codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user1,
			Role: codersdk.TemplateRoleUse,
		})

		_, err = client.UpdateUserStatus(ctx, user1.ID.String(), codersdk.UserStatusSuspended)
		require.NoError(t, err)

		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 0, "suspended users should be filtered")
	})

	// Test that we do not return deleted groups.
	t.Run("FilterDeletedGroups", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, _ := testutil.Context(t)

		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "test",
		})
		require.NoError(t, err)

		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				group.ID.String(): codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		// Length should be 2 for test group and the implicit allUsers group.
		require.Len(t, acl.Groups, 2)

		require.Contains(t, acl.Groups, codersdk.TemplateGroup{
			Group: group,
			Role:  codersdk.TemplateRoleUse,
		})

		err = client.DeleteGroup(ctx, group.ID)
		require.NoError(t, err)

		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		// Length should be 1 for the allUsers group.
		require.Len(t, acl.Groups, 1)
		require.NotContains(t, acl.Groups, codersdk.TemplateGroup{
			Group: group,
			Role:  codersdk.TemplateRoleUse,
		})
	})

	t.Run("AdminCanPushVersions", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		client1, user1 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user1.ID.String(): codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		data, err := echo.Tar(nil)
		require.NoError(t, err)
		file, err := client1.Upload(context.Background(), codersdk.ContentTypeTar, data)
		require.NoError(t, err)

		_, err = client1.CreateTemplateVersion(ctx, user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			Name:          "testme",
			TemplateID:    template.ID,
			FileID:        file.ID,
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		require.Error(t, err)

		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user1.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		_, err = client1.CreateTemplateVersion(ctx, user.OrganizationID, codersdk.CreateTemplateVersionRequest{
			Name:          "testme",
			TemplateID:    template.ID,
			FileID:        file.ID,
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		require.NoError(t, err)
	})
}

func TestUpdateTemplateACL(t *testing.T) {
	t.Parallel()

	t.Run("UserPerms", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleUse,
				user3.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		templateUser2 := codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleUse,
		}

		templateUser3 := codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleAdmin,
		}

		require.Len(t, acl.Users, 2)
		require.Contains(t, acl.Users, templateUser2)
		require.Contains(t, acl.Users, templateUser3)
	})

	t.Run("DeleteUser", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleUse,
				user3.ID.String(): codersdk.TemplateRoleAdmin,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleUse,
		})
		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleAdmin,
		})

		req = codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleAdmin,
				user3.ID.String(): codersdk.TemplateRoleDeleted,
			},
		}

		err = client.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleAdmin,
		})

		require.NotContains(t, acl.Users, codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleAdmin,
		})
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				"hi": "admin",
			},
		}

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.Error(t, err)
		cerr, _ := codersdk.AsError(err)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("InvalidUser", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				uuid.NewString(): "admin",
			},
		}

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.Error(t, err)
		cerr, _ := codersdk.AsError(err)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("InvalidRole", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): "updater",
			},
		}

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.Error(t, err)
		cerr, _ := codersdk.AsError(err)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("RegularUserCannotUpdatePerms", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		client2, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleUse,
			},
		}

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		req = codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleAdmin,
			},
		}

		err = client2.UpdateTemplateACL(ctx, template.ID, req)
		require.Error(t, err)
		cerr, _ := codersdk.AsError(err)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())
	})

	t.Run("RegularUserWithAdminCanUpdate", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		client2, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleAdmin,
			},
		}

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		req = codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user3.ID.String(): codersdk.TemplateRoleUse,
			},
		}

		err = client2.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		acl, err := client2.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleUse,
		})
	})

	t.Run("allUsersGroup", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)
	})

	t.Run("CustomGroupHasAccess", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		client1, user1 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, _ := testutil.Context(t)

		// Create a group to add to the template.
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "test",
		})
		require.NoError(t, err)

		// Check that the only current group is the allUsers group.
		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)
		require.Len(t, acl.Groups, 1)

		// Update the template to only allow access to the 'test' group.
		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				// The allUsers group shares the same ID as the organization.
				user.OrganizationID.String(): codersdk.TemplateRoleDeleted,
				group.ID.String():            codersdk.TemplateRoleUse,
			},
		})
		require.NoError(t, err)

		// Get the ACL list for the template and assert the test group is
		// present.
		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)
		require.Equal(t, group.ID, acl.Groups[0].ID)

		// Try to get the template as the regular user. This should
		// fail since we haven't been added to the template yet.
		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())

		// Patch the group to add the regular user.
		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user1.ID.String()},
		})
		require.NoError(t, err)
		require.Len(t, group.Members, 1)
		require.Equal(t, user1.ID, group.Members[0].ID)

		// Fetching the template should succeed since our group has view access.
		_, err = client1.Template(ctx, template.ID)
		require.NoError(t, err)
	})

	t.Run("NoAccess", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			TemplateRBAC: true,
		})

		client1, _ := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)

		// User should be able to read template due to allUsers group.
		_, err = client1.Template(ctx, template.ID)
		require.NoError(t, err)

		allUsers := acl.Groups[0]

		err = client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			GroupPerms: map[string]codersdk.TemplateRole{
				allUsers.ID.String(): codersdk.TemplateRoleDeleted,
			},
		})
		require.NoError(t, err)

		acl, err = client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 0)
		require.Len(t, acl.Users, 0)

		// User should not be able to read template due to allUsers group being deleted.
		_, err = client1.Template(ctx, template.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())
	})
}
