package toolsdk_test

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

func TestListOrganizations(t *testing.T) {
	t.Parallel()
	for _, count := range []int{0, 1, 2} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			t.Parallel()
			client, db := coderdtest.NewWithDatabase(t, nil)
			owner := coderdtest.CreateFirstUser(t, client)
			memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
			org := dbgen.Organization(t, db, database.Organization{})
			_ = dbgen.Organization(t, db, database.Organization{})
			if count == 0 {
				require.NoError(t, client.DeleteOrganizationMember(t.Context(), owner.OrganizationID, member.ID.String()))
			}
			if count == 2 {
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: member.ID})
			}
			deps, err := toolsdk.NewDeps(memberClient)
			require.NoError(t, err)
			result, err := testTool(t, toolsdk.ListOrganizations, deps, toolsdk.NoArgs{})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, result, count)
			expected, err := memberClient.OrganizationsByUser(t.Context(), codersdk.Me)
			require.NoError(t, err)
			require.ElementsMatch(t, expected, result)
			for _, org := range result {
				require.NotEqual(t, uuid.Nil, org.ID)
				require.NotEmpty(t, org.Name)
				require.NotEmpty(t, org.DisplayName)
			}
		})
	}
}
