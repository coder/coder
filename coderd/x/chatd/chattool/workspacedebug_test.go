package chattool_test

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

func TestGetWorkspaceBuildLogs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
	}).Seed(database.WorkspaceBuild{
		Transition: database.WorkspaceTransitionStart,
	}).Do()
	build := wsResp.Build
	for _, output := range []string{"Initializing", "Error: image pull denied"} {
		_ = dbgen.ProvisionerJobLog(t, db, database.ProvisionerJobLog{
			JobID:  build.JobID,
			Stage:  "apply",
			Output: output,
		})
	}

	tool := chattool.GetWorkspaceBuildLogs(db, chattool.WorkspaceDebugOptions{OwnerID: user.ID})
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  "get_workspace_build_logs",
		Input: `{"build_id":"` + build.ID.String() + `"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	var result struct {
		BuildID string `json:"build_id"`
		Logs    []struct {
			Output string `json:"output"`
			Stage  string `json:"stage"`
		} `json:"logs"`
		HasMore bool `json:"has_more"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, build.ID.String(), result.BuildID)
	require.False(t, result.HasMore)
	require.Len(t, result.Logs, 2)
	require.Equal(t, "Error: image pull denied", result.Logs[1].Output)
	require.Equal(t, "apply", result.Logs[1].Stage)

	resp, err = tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-3",
		Name:  "get_workspace_build_logs",
		Input: `{"build_id":"` + uuid.NewString() + `"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
}
