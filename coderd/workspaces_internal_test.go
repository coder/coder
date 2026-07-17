package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

func TestConvertToWorkspaceRole(t *testing.T) {
	t.Parallel()

	// Round-trip: the exact action sets produced for each role must
	// resolve back to that role.
	require.Equal(t, codersdk.WorkspaceRoleAdmin,
		convertToWorkspaceRole(db2sdk.WorkspaceRoleActions(codersdk.WorkspaceRoleAdmin)))
	require.Equal(t, codersdk.WorkspaceRoleUse,
		convertToWorkspaceRole(db2sdk.WorkspaceRoleActions(codersdk.WorkspaceRoleUse)))

	// Stored admin ACL entries never contain use_shared or delete. This
	// derivation must stay an exact-set match for WorkspaceRoleAdmin: if
	// WorkspaceRoleActions ever includes use_shared (or drops another
	// action), stored entries would stop matching and degrade to
	// WorkspaceRoleDeleted.
	storedAdmin := slice.Omit(rbac.ResourceWorkspace.AvailableActions(),
		policy.ActionDelete, policy.ActionUseShared)
	require.Equal(t, codersdk.WorkspaceRoleAdmin, convertToWorkspaceRole(storedAdmin))

	// Action sets matching no role resolve to deleted.
	require.Equal(t, codersdk.WorkspaceRoleDeleted,
		convertToWorkspaceRole([]policy.Action{policy.ActionRead}))
	require.Equal(t, codersdk.WorkspaceRoleDeleted, convertToWorkspaceRole(nil))
}
