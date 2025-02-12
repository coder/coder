package database

import (
	"testing"
	"time"

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
// This test is a guard rail to prevent developer oversight mistakes.
func TestWorkspaceTableConvert(t *testing.T) {
	t.Parallel()

	staticRandoms := &testutil.Random{
		String:  func() string { return "foo" },
		Bool:    func() bool { return true },
		Int:     func() int64 { return 500 },
		Uint:    func() uint64 { return 126 },
		Float:   func() float64 { return 3.14 },
		Complex: func() complex128 { return 6.24 },
		Time: func() time.Time {
			return time.Date(2020, 5, 2, 5, 19, 21, 30, time.UTC)
		},
	}

	// This feels a bit janky, but it works.
	// If you use 'PopulateStruct' to create 2 workspaces, using the same
	// "random" values for each type. Then they should be identical.
	//
	// So if 'workspace.WorkspaceTable()' was missing any fields in its
	// conversion, the comparison would fail.

	var workspace Workspace
	err := testutil.PopulateStruct(&workspace, staticRandoms)
	require.NoError(t, err)

	var subset WorkspaceTable
	err = testutil.PopulateStruct(&subset, staticRandoms)
	require.NoError(t, err)

	require.Equal(t, workspace.WorkspaceTable(), subset,
		"'workspace.WorkspaceTable()' is not missing at least 1 field when converting to 'WorkspaceTable'. "+
			"To resolve this, go to the 'func (w Workspace) WorkspaceTable()' and ensure all fields are converted.")
}
