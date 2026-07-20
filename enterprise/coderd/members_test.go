package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// TestBatchAddMembersReadAuthorization verifies that postOrganizationMembers
// resolves target users in the requester's authorization context, so a caller
// that can create organization members but cannot read site users is rejected
// with 403 at the read gate before any insert. The read gate is tamper-evident:
// swapping the handler to AsSystemRestricted does not make this succeed, it
// makes the request fail later at the role-assignment gate (canAssignRoles
// rejects the custom role's implied organization-member grant, a 500), which
// the require.Equal(403) assertion still catches as a non-403.
func TestBatchAddMembersReadAuthorization(t *testing.T) {
	t.Parallel()

	owner, first := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureCustomRoles: 1,
			},
		},
	})

	ctx := testutil.Context(t, testutil.WaitMedium)

	// A custom org role that can create members and assign org roles, but
	// grants no site-wide user:read (custom org roles cannot). This is exactly
	// the caller that could add arbitrary accounts by UUID if the handler
	// skipped the per-user read check.
	//nolint:gocritic // owner is required to create a custom role
	role, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
		Name:           "member-adder",
		DisplayName:    "Member Adder",
		OrganizationID: first.OrganizationID.String(),
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceOrganizationMember: {codersdk.ActionCreate, codersdk.ActionRead},
			codersdk.ResourceAssignOrgRole:      {codersdk.ActionAssign},
			codersdk.ResourceOrganization:       {codersdk.ActionRead},
		}),
	})
	require.NoError(t, err, "create custom role")

	adder, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleIdentifier{
		Name:           role.Name,
		OrganizationID: first.OrganizationID,
	})

	// A target user the adder cannot read.
	_, target := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

	_, err = adder.PostOrganizationMembers(ctx, first.OrganizationID, codersdk.AddOrganizationMembersRequest{
		UserIDs: []uuid.UUID{target.ID},
	})
	require.Error(t, err)
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
}
