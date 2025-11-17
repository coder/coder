package database

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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

// TestTaskTableConvert verifies all task fields are converted
// when reducing a `Task` to a `TaskTable`.
// This test is a guard rail to prevent developer oversight mistakes.
func TestTaskTableConvert(t *testing.T) {
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

	var task Task
	err := testutil.PopulateStruct(&task, staticRandoms)
	require.NoError(t, err)

	var subset TaskTable
	err = testutil.PopulateStruct(&subset, staticRandoms)
	require.NoError(t, err)

	require.Equal(t, task.TaskTable(), subset,
		"'task.TaskTable()' is not missing at least 1 field when converting to 'TaskTable'. "+
			"To resolve this, go to the 'func (t Task) TaskTable()' and ensure all fields are converted.")
}

// TestAuditLogsQueryConsistency ensures that GetAuditLogsOffset and CountAuditLogs
// have identical WHERE clauses to prevent filtering inconsistencies.
// This test is a guard rail to prevent developer oversight mistakes.
func TestAuditLogsQueryConsistency(t *testing.T) {
	t.Parallel()

	getWhereClause := extractWhereClause(getAuditLogsOffset)
	require.NotEmpty(t, getWhereClause, "failed to extract WHERE clause from GetAuditLogsOffset")

	countWhereClause := extractWhereClause(countAuditLogs)
	require.NotEmpty(t, countWhereClause, "failed to extract WHERE clause from CountAuditLogs")

	// Compare the WHERE clauses
	if diff := cmp.Diff(getWhereClause, countWhereClause); diff != "" {
		t.Errorf("GetAuditLogsOffset and CountAuditLogs WHERE clauses must be identical to ensure consistent filtering.\nDiff:\n%s", diff)
	}
}

// Same as TestAuditLogsQueryConsistency, but for connection logs.
func TestConnectionLogsQueryConsistency(t *testing.T) {
	t.Parallel()

	getWhereClause := extractWhereClause(getConnectionLogsOffset)
	require.NotEmpty(t, getWhereClause, "getConnectionLogsOffset query should have a WHERE clause")

	countWhereClause := extractWhereClause(countConnectionLogs)
	require.NotEmpty(t, countWhereClause, "countConnectionLogs query should have a WHERE clause")

	require.Equal(t, getWhereClause, countWhereClause, "getConnectionLogsOffset and countConnectionLogs queries should have the same WHERE clause")
}

// extractWhereClause extracts the WHERE clause from a SQL query string
func extractWhereClause(query string) string {
	// Find WHERE and get everything after it
	wherePattern := regexp.MustCompile(`(?is)WHERE\s+(.*)`)
	whereMatches := wherePattern.FindStringSubmatch(query)
	if len(whereMatches) < 2 {
		return ""
	}

	whereClause := whereMatches[1]

	// Remove ORDER BY, LIMIT, OFFSET clauses from the end
	whereClause = regexp.MustCompile(`(?is)\s+(ORDER BY|LIMIT|OFFSET).*$`).ReplaceAllString(whereClause, "")

	// Remove SQL comments
	whereClause = regexp.MustCompile(`(?m)--.*$`).ReplaceAllString(whereClause, "")

	return strings.TrimSpace(whereClause)
}
