package coderd_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestCheckPermissionsAnyOrg demonstrates that batching authcheck permissions
// through rbac.Filter breaks checks with any_org=true: partial evaluation
// denies AnyOrgOwner objects that full evaluation allows. A single any_org
// check (below the batching threshold) returns true, while the same check
// repeated 55 times (pushing the group over the threshold) returns false.
func TestCheckPermissionsAnyOrg(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	adminClient := coderdtest.New(t, nil)
	adminUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	check := codersdk.AuthorizationCheck{
		Object: codersdk.AuthorizationObject{
			ResourceType: codersdk.ResourceWorkspace,
			OwnerID:      "me",
			AnyOrgOwner:  true,
		},
		Action: "create",
	}

	// Below the batching threshold: full evaluation, allowed.
	single, err := memberClient.AuthCheck(ctx, codersdk.AuthorizationRequest{
		Checks: map[string]codersdk.AuthorizationCheck{"can-create-workspace": check},
	})
	require.NoError(t, err)
	require.True(t, single["can-create-workspace"], "single any_org check should be allowed")

	// Same check, 55 copies: the (create, workspace) group crosses the
	// batching threshold and is evaluated with a prepared partial query,
	// which denies AnyOrgOwner objects.
	grouped := make(map[string]codersdk.AuthorizationCheck)
	for i := 0; i < 55; i++ {
		grouped[fmt.Sprintf("can-create-workspace-%d", i)] = check
	}
	groupedResp, err := memberClient.AuthCheck(ctx, codersdk.AuthorizationRequest{Checks: grouped})
	require.NoError(t, err)
	for key, allowed := range groupedResp {
		require.True(t, allowed, "grouped any_org check %q should be allowed but was denied", key)
	}
}
