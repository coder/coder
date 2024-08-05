package coderd_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestFrobulators(t *testing.T) {
	t.Parallel()

	// Setup for all tests
	api := coderdtest.New(t, nil)

	setupCtx := testutil.Context(t, testutil.WaitShort)
	firstUser := coderdtest.CreateFirstUser(t, api)

	// Create 2 member, add one frobulator each
	memberClient1, otherUser1 := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
	memberClient2, otherUser2 := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

	frobulatorID, err := memberClient1.CreateFrobulator(setupCtx, otherUser1.ID, fmt.Sprintf("model-%s", uuid.NewString()))
	require.NoError(t, err)
	require.NotNil(t, frobulatorID)
	frobulator2ID, err := memberClient2.CreateFrobulator(setupCtx, otherUser2.ID, fmt.Sprintf("model2-%s", uuid.NewString()))
	require.NoError(t, err)
	require.NotNil(t, frobulator2ID)

	t.Run("Read other members' frobulators", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: a new member
		memberClient, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

		// When: attempting to view the frobulators of another user
		frobs, err := memberClient.GetUserFrobulators(ctx, otherUser1.ID)

		// Then: validate that access was denied
		require.Nil(t, frobs)

		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		// Unfortunately, the ExtractUserParam middleware returns a 400 Bad Request not a 403 Forbidden for unauthorized requests.
		require.Equal(t, http.StatusBadRequest, sdkError.StatusCode())
	})

	t.Run("Create and read own frobulators", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: a user which is not an admin
		memberClient, member := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)

		// When: attempting to create a frobulator
		id, err := memberClient.CreateFrobulator(ctx, member.ID, fmt.Sprintf("model-%s", uuid.NewString()))

		// Then: it should succeed and should be queryable
		require.NoError(t, err)
		require.NotNil(t, id)

		frobs, err := memberClient.GetUserFrobulators(ctx, member.ID)
		require.NoError(t, err)
		require.Len(t, frobs, 1)
		require.Equal(t, id, frobs[0].ID)
	})

	t.Run("Access all frobulators as admin", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: an owner - which has access to all other users' frobulators
		adminClient, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID, rbac.RoleOwner())

		// When: accessing all frobulators
		frobs, err := adminClient.GetAllFrobulators(ctx)

		// Then: the expected number of frobulators are returned
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(frobs), 2)
	})

	t.Run("Access specific users' frobulators as admin", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Given: an owner - which has access to all other users' frobulators
		adminClient, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID, rbac.RoleOwner())

		// When: accessing the frobulators of another user in their org
		frobs, err := adminClient.GetUserFrobulators(ctx, otherUser1.ID)

		// Then: the expected frobulator should be returned
		require.NoError(t, err)
		require.Len(t, frobs, 1)

		var found bool
		for _, f := range frobs {
			if f.ID == frobulatorID {
				found = true
			}
		}
		require.True(t, found, "reference frobulator not found")
	})
}
