package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestIsAuthorizedQuery(t *testing.T) {
	t.Parallel()

	query := `SELECT true;`
	_, err := insertAuthorizedFilter(query, "")
	require.ErrorContains(t, err, "does not contain authorized replace string", "ensure replace string")
}

// TestWorkspaceTableConvert verifies all workspace fields are converted
// when reducing a `Workspace` to a `WorkspaceTable`.
func TestWorkspaceTableConvert(t *testing.T) {
	t.Parallel()

	var workspace Workspace
	err := testutil.PopulateStruct(&workspace, nil)
	require.NoError(t, err)

	workspace.WorkspaceTable()
	require.JSONEq(t)

	fmt.Println(workspace)

}
