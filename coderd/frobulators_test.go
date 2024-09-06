package coderd_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestFrobulators(t *testing.T) {
	t.Parallel()

	// Setup for all tests
	api, store := coderdtest.NewWithDatabase(t, nil)

	setupCtx := testutil.Context(t, testutil.WaitShort)
	coderdtest.CreateFirstUser(t, api)

	org1 := dbgen.Organization(t, store, database.Organization{
		ID:        uuid.New(),
		Name:      "test-org1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	org2 := dbgen.Organization(t, store, database.Organization{
		ID:        uuid.New(),
		Name:      "test-org2",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	// Create 2 member, add one frobulator each
	memberClient1, member1 := coderdtest.CreateAnotherUser(t, api, org1.ID)
	memberClient2, member2 := coderdtest.CreateAnotherUser(t, api, org2.ID)

	frobulatorID, err := memberClient1.CreateFrobulator(setupCtx, member1.ID, org1.ID, fmt.Sprintf("model-%s", uuid.NewString()))
	require.NoError(t, err)
	require.NotNil(t, frobulatorID)
	frobulator2ID, err := memberClient2.CreateFrobulator(setupCtx, member2.ID, org2.ID, fmt.Sprintf("model2-%s", uuid.NewString()))
	require.NoError(t, err)
	require.NotNil(t, frobulator2ID)

	t.Run("Read other members' frobulators", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			user codersdk.User
			org  database.Organization
		}{
			{
				name: "same org",
				user: member1,
				org:  org1,
			},
			{
				name: "different org",
				user: member1,
				org:  org2,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitShort)

				// Given: a new member in the given group
				memberClient, _ := coderdtest.CreateAnotherUser(t, api, tc.org.ID)

				// When: attempting to view the frobulators of another user
				frobs, err := memberClient.GetFrobulators(ctx, tc.user.ID, tc.org.ID)

				// Then: validate that no frobulators were returned
				require.Nil(t, frobs)

				// Then: validate that access was denied by receiving a 404
				var sdkError *codersdk.Error
				require.Error(t, err)
				require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
				require.Equal(t, http.StatusNotFound, sdkError.StatusCode())
			})
		}
	})

	t.Run("Create and read own frobulators", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		org := org1

		// Given: a user which is not an admin
		memberClient, member := coderdtest.CreateAnotherUser(t, api, org.ID)

		// When: attempting to create a frobulator
		id, err := memberClient.CreateFrobulator(ctx, member.ID, org.ID, fmt.Sprintf("model-%s", uuid.NewString()))

		// Then: it should succeed and should be queryable
		require.NoError(t, err)
		require.NotNil(t, id)

		frobs, err := memberClient.GetFrobulators(ctx, member.ID, org.ID)
		require.NoError(t, err)
		require.Len(t, frobs, 1)
		require.Equal(t, id, frobs[0].ID)
	})

	t.Run("Access users' frobulators as admin", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name         string
			member       codersdk.User
			memberOrg    database.Organization
			adminOrg     database.Organization
			role         rbac.RoleIdentifier
			expectedFrob uuid.UUID
			expectedErr  string
		}{
			{
				name:         "owner, same org",
				member:       member1,
				memberOrg:    org1,
				adminOrg:     org1,
				role:         rbac.RoleOwner(),
				expectedFrob: frobulatorID,
			},
			{
				name:         "owner, different org",
				member:       member2,
				memberOrg:    org2,
				adminOrg:     org1,
				role:         rbac.RoleOwner(),
				expectedFrob: frobulator2ID,
			},
			{
				name:         "org admin, same org",
				member:       member2,
				memberOrg:    org2,
				adminOrg:     org2,
				role:         rbac.ScopedRoleOrgAdmin(org2.ID),
				expectedFrob: frobulator2ID,
			},
			{
				// Org admins do not have permission outside of their own org.
				name:        "org admin, diff org",
				member:      member2,
				memberOrg:   org2,
				adminOrg:    org1,
				role:        rbac.ScopedRoleOrgAdmin(org1.ID),
				expectedErr: "404: Resource not found",
			},
			{
				// User admins do not have permissions even inside their own org.
				name:        "user admin, same org",
				member:      member2,
				memberOrg:   org2,
				adminOrg:    org2,
				role:        rbac.ScopedRoleOrgUserAdmin(org2.ID),
				expectedErr: "500: An internal server error occurred",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitShort)

				// Given: a new user of the defined role
				client, _ := coderdtest.CreateAnotherUser(t, api, tc.adminOrg.ID, tc.role)

				// When: accessing the frobulators of a user in an org
				frobs, err := client.GetFrobulators(ctx, tc.member.ID, tc.memberOrg.ID)

				// Then: if an error is expected, validate that we received what we expect
				if tc.expectedErr != "" {
					require.ErrorContains(t, err, tc.expectedErr)
					return
				}

				// Then: the expected frobulator should be returned
				require.NoError(t, err)
				require.Len(t, frobs, 1)

				var found bool
				for _, f := range frobs {
					if f.ID == tc.expectedFrob {
						found = true
					}
				}
				require.True(t, found, "reference frobulator not found")
			})
		}
	})

	t.Run("Create and delete own frobulators", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		org := org1

		// Given: a user which is not an admin
		memberClient, member := coderdtest.CreateAnotherUser(t, api, org.ID)

		// When: attempting to create a frobulator
		id, err := memberClient.CreateFrobulator(ctx, member.ID, org.ID, fmt.Sprintf("model-%s", uuid.NewString()))

		// Then: it should succeed and should be queryable
		require.NoError(t, err)
		require.NotNil(t, id)
		frobs, err := memberClient.GetFrobulators(ctx, member.ID, org.ID)
		require.NoError(t, err)
		require.Len(t, frobs, 1)
		require.Equal(t, id, frobs[0].ID)

		// When: attempting to delete a frobulator
		err = memberClient.DeleteFrobulator(ctx, id, member.ID, org.ID)

		// Then: it should succeed and the frobulator will no longer be present
		require.NoError(t, err)
		frobs, err = memberClient.GetFrobulators(ctx, member.ID, org.ID)
		require.NoError(t, err)
		require.Len(t, frobs, 0)
	})
}
