package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectInstructionsUpdateOnlyRole(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentChatProjects)}
	ownerClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureCustomRoles: 1},
		},
	})
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitLong)

	project, err := owner.CreateChatProject(ctx, firstUser.OrganizationID, codersdk.CreateChatProjectRequest{
		Name: "Update Only Project",
	})
	require.NoError(t, err)

	role, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
		Name:           "chat-project-update-only",
		OrganizationID: firstUser.OrganizationID.String(),
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceChatProject: {codersdk.ActionUpdate},
		}),
	})
	require.NoError(t, err)
	editorClient, editor := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID,
		rbac.RoleIdentifier{Name: role.Name, OrganizationID: firstUser.OrganizationID})
	updateOnly := codersdk.NewExperimentalClient(editorClient)

	// The handler checks update permission, so a role without read can
	// still save and receives the saved value.
	saved, err := updateOnly.UpdateChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
		Instructions: "Keep answers short.",
	})
	require.NoError(t, err)
	require.Equal(t, "Keep answers short.", saved.Instructions)
	require.NotNil(t, saved.UpdatedBy)
	require.Equal(t, editor.ID, saved.UpdatedBy.ID)
	require.NoError(t, updateOnly.DeleteChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID))
}

func TestChatProjectInstructionsAudit(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true))
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentChatProjects)}
	ownerClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			DeploymentValues: dv,
			Database:         db,
			Pubsub:           ps,
			Auditor:          auditor,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAuditLog: 1},
		},
	})
	owner := codersdk.NewExperimentalClient(ownerClient)
	orgID := firstUser.OrganizationID
	ctx := testutil.Context(t, testutil.WaitLong)

	project, err := owner.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{
		Name: "Audited Project",
	})
	require.NoError(t, err)

	logs := func(t *testing.T) []database.AuditLog {
		t.Helper()
		rows, err := db.GetAuditLogsOffset(dbauthz.AsSystemRestricted(ctx), database.GetAuditLogsOffsetParams{
			ResourceType: string(database.ResourceTypeChatProjectInstructions),
			ResourceID:   project.ID,
			LimitOpt:     10,
		})
		require.NoError(t, err)
		out := make([]database.AuditLog, 0, len(rows))
		for _, row := range rows {
			out = append(out, row.AuditLog)
		}
		return out
	}
	update := func(instructions string) {
		t.Helper()
		_, err := owner.UpdateChatProjectInstructions(ctx, orgID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
			Instructions: instructions,
		})
		require.NoError(t, err)
	}

	update("First")
	update("Second")
	// Saving unchanged instructions and deleting unset ones are not audited.
	update("Second")
	require.NoError(t, owner.DeleteChatProjectInstructions(ctx, orgID, project.ID))
	require.NoError(t, owner.DeleteChatProjectInstructions(ctx, orgID, project.ID))

	got := logs(t)
	// Logs are newest first.
	want := []struct {
		action database.AuditAction
		status int
		diff   audit.Map
	}{
		{action: database.AuditActionDelete, status: http.StatusNoContent, diff: audit.Map{"instructions": {Old: "Second", New: ""}}},
		{action: database.AuditActionWrite, status: http.StatusOK, diff: audit.Map{"instructions": {Old: "First", New: "Second"}}},
		{action: database.AuditActionWrite, status: http.StatusOK, diff: audit.Map{"instructions": {Old: "", New: "First"}}},
	}
	require.Len(t, got, len(want))
	for i, w := range want {
		var diff audit.Map
		require.NoError(t, json.Unmarshal(got[i].Diff, &diff))
		require.Equal(t, w.diff, diff)
		require.Equal(t, w.action, got[i].Action)
		require.EqualValues(t, w.status, got[i].StatusCode)
		require.Equal(t, firstUser.UserID, got[i].UserID)
		require.Equal(t, orgID, got[i].OrganizationID)
		require.Equal(t, project.ID, got[i].ResourceID)
		require.Equal(t, project.Name, got[i].ResourceTarget)
	}
}
