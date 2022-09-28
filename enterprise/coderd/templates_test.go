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
	"github.com/coder/coder/testutil"
)

func TestTemplateACL(t *testing.T) {
	t.Parallel()

	t.Run("UserRoles", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, _ := testutil.Context(t)

		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleView,
				user3.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		templateUser2 := codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleView,
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
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)
	})

	t.Run("NoGroups", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
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

func TestUpdateTemplateACL(t *testing.T) {
	t.Parallel()

	t.Run("UserPerms", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleView,
				user3.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		templateUser2 := codersdk.TemplateUser{
			User: user2,
			Role: codersdk.TemplateRoleView,
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
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleView,
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
			Role: codersdk.TemplateRoleView,
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
		client2, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		req := codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				user2.ID.String(): codersdk.TemplateRoleView,
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
				user3.ID.String(): codersdk.TemplateRoleView,
			},
		}

		err = client2.UpdateTemplateACL(ctx, template.ID, req)
		require.NoError(t, err)

		acl, err := client2.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Contains(t, acl.Users, codersdk.TemplateUser{
			User: user3,
			Role: codersdk.TemplateRoleView,
		})
	})

	t.Run("allUsersGroup", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		acl, err := client.TemplateACL(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, acl.Groups, 1)
		require.Len(t, acl.Users, 0)
	})

	t.Run("NoAccess", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
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
