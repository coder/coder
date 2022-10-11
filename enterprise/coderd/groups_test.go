package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
)

func TestCreateGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)
		require.Equal(t, "hi", group.Name)
		require.Empty(t, group.Members)
		require.NotEqual(t, uuid.Nil.String(), group.ID.String())
	})

	t.Run("Conflict", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		_, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		_, err = client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusConflict, cerr.StatusCode())
	})

	t.Run("allUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		_, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: database.AllUsersGroup,
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})
}

func TestPatchGroup(t *testing.T) {
	t.Parallel()

	t.Run("Name", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name: "bye",
		})
		require.NoError(t, err)
		require.Equal(t, "bye", group.Name)
	})

	t.Run("AddUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)

		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user2)
		require.Contains(t, group.Members, user3)
	})

	t.Run("RemoveUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user4 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)

		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String(), user4.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user2)
		require.Contains(t, group.Members, user3)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			RemoveUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)
		require.NotContains(t, group.Members, user2)
		require.NotContains(t, group.Members, user3)
		require.Contains(t, group.Members, user4)
	})

	t.Run("UserNotExist", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{uuid.NewString()},
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusPreconditionFailed, cerr.StatusCode())
	})

	t.Run("MalformedUUID", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{"yeet"},
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})

	t.Run("AddDuplicateUser", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user2.ID.String()},
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)

		require.Equal(t, http.StatusPreconditionFailed, cerr.StatusCode())
	})

	t.Run("allUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			Name: database.AllUsersGroup,
		})
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})
}

// TODO: test auth.
func TestGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		ggroup, err := client.Group(ctx, group.ID)
		require.NoError(t, err)
		require.Equal(t, group, ggroup)
	})

	t.Run("WithUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)

		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user2)
		require.Contains(t, group.Members, user3)

		ggroup, err := client.Group(ctx, group.ID)
		require.NoError(t, err)
		require.Equal(t, group, ggroup)
	})

	t.Run("RegularUserReadGroup", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		client1, _ := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)

		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		ggroup, err := client1.Group(ctx, group.ID)
		require.NoError(t, err)
		require.Equal(t, group, ggroup)
	})

	t.Run("FilterDeletedUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})

		_, user1 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)

		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user1.ID.String(), user2.ID.String()},
		})
		require.NoError(t, err)
		require.Contains(t, group.Members, user1)
		require.Contains(t, group.Members, user2)

		err = client.DeleteUser(ctx, user1.ID)
		require.NoError(t, err)

		group, err = client.Group(ctx, group.ID)
		require.NoError(t, err)
		require.NotContains(t, group.Members, user1)
	})

	t.Run("FilterSuspendedUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})

		_, user1 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)

		ctx, _ := testutil.Context(t)
		group, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user1.ID.String(), user2.ID.String()},
		})
		require.NoError(t, err)
		require.Len(t, group.Members, 2)
		require.Contains(t, group.Members, user1)
		require.Contains(t, group.Members, user2)

		user1, err = client.UpdateUserStatus(ctx, user1.ID.String(), codersdk.UserStatusSuspended)
		require.NoError(t, err)

		group, err = client.Group(ctx, group.ID)
		require.NoError(t, err)
		require.Len(t, group.Members, 1)
		require.NotContains(t, group.Members, user1)
		require.Contains(t, group.Members, user2)
	})
}

// TODO: test auth.
func TestGroups(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		_, user2 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user3 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user4 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)
		_, user5 := coderdtest.CreateAnotherUserWithUser(t, client, user.OrganizationID)

		ctx, _ := testutil.Context(t)
		group1, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		group2, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hey",
		})
		require.NoError(t, err)

		group1, err = client.PatchGroup(ctx, group1.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user2.ID.String(), user3.ID.String()},
		})
		require.NoError(t, err)

		group2, err = client.PatchGroup(ctx, group2.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{user4.ID.String(), user5.ID.String()},
		})
		require.NoError(t, err)

		groups, err := client.GroupsByOrganization(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, groups, 2)
		require.Contains(t, groups, group1)
		require.Contains(t, groups, group2)
	})
}

func TestDeleteGroup(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		group1, err := client.CreateGroup(ctx, user.OrganizationID, codersdk.CreateGroupRequest{
			Name: "hi",
		})
		require.NoError(t, err)

		err = client.DeleteGroup(ctx, group1.ID)
		require.NoError(t, err)

		_, err = client.Group(ctx, group1.ID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())
	})

	t.Run("allUsers", func(t *testing.T) {
		t.Parallel()

		client := coderdenttest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		_ = coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			RBACEnabled: true,
		})
		ctx, _ := testutil.Context(t)
		err := client.DeleteGroup(ctx, user.OrganizationID)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusBadRequest, cerr.StatusCode())
	})
}
