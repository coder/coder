package coderd_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func TestFoundationAuthCheckResourceID(t *testing.T) {
	t.Parallel()
	client, db := newTemplateResourceClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	local := dbgen.Foundation(t, db, database.Foundation{OrganizationID: owner.OrganizationID})
	foreignOrg := dbgen.Organization(t, db, database.Organization{})
	foreign := dbgen.Foundation(t, db, database.Foundation{OrganizationID: foreignOrg.ID})

	checks := make(map[string]codersdk.AuthorizationCheck)
	for _, action := range []string{"read", "update", "use"} {
		for name, id := range map[string]uuid.UUID{"local": local.ID, "foreign": foreign.ID} {
			checks[name+"/"+action] = codersdk.AuthorizationCheck{
				Action: codersdk.RBACAction(action),
				Object: codersdk.AuthorizationObject{
					ResourceType: codersdk.ResourceFoundation,
					ResourceID:   id.String(),
					// The stored organization must override caller-supplied context.
					OrganizationID: owner.OrganizationID.String(),
				},
			}
		}
	}
	checks["missing"] = codersdk.AuthorizationCheck{
		Action: "read",
		Object: codersdk.AuthorizationObject{ResourceType: codersdk.ResourceFoundation, ResourceID: uuid.NewString()},
	}
	for _, tc := range []struct {
		name           string
		client         *codersdk.Client
		local, foreign bool
	}{
		{name: "Owner", client: client, local: true, foreign: true},
		{name: "OrgAdmin", client: orgAdmin, local: true},
		{name: "Member", client: member},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			response, err := tc.client.AuthCheck(t.Context(), codersdk.AuthorizationRequest{Checks: checks})
			require.NoError(t, err)
			expected := codersdk.AuthorizationResponse{"missing": false}
			for _, action := range []string{"read", "update", "use"} {
				expected["local/"+action] = tc.local
				expected["foreign/"+action] = tc.foreign
			}
			require.Equal(t, expected, response)
		})
	}
}
