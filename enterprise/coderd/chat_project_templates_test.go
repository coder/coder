package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectTemplatesAudit(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true))
	ownerClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			DeploymentValues: coderdtest.DeploymentValues(t),
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

	template := dbgen.Template(t, db, database.Template{
		OrganizationID: orgID,
		CreatedBy:      firstUser.UserID,
		AgentsAllowed:  true,
	})
	project, err := owner.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{Name: "Audited Project"})
	require.NoError(t, err)

	require.NoError(t, owner.AttachChatProjectTemplate(ctx, orgID, project.ID, template.ID))
	// Attaching an attached template changes nothing, so it is not audited.
	require.NoError(t, owner.AttachChatProjectTemplate(ctx, orgID, project.ID, template.ID))
	require.NoError(t, owner.DetachChatProjectTemplate(ctx, orgID, project.ID, template.ID))

	// Templates attached while creating a project are audited too.
	createdWithTemplate, err := owner.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{
		Name:        "Audited From Creation",
		TemplateIDs: []uuid.UUID{template.ID},
	})
	require.NoError(t, err)

	attached, detached := []any{template.ID.String()}, []any{}
	requireTemplateAudits := func(project codersdk.ChatProject, status int, want ...audit.Map) {
		t.Helper()
		//nolint:gocritic // The test reads audit logs directly.
		rows, err := db.GetAuditLogsOffset(dbauthz.AsSystemRestricted(ctx), database.GetAuditLogsOffsetParams{
			ResourceType: string(database.ResourceTypeChatProject),
			ResourceID:   project.ID,
			Action:       string(database.AuditActionWrite),
			LimitOpt:     10,
		})
		require.NoError(t, err)
		require.Len(t, rows, len(want))
		for i, row := range rows {
			var diff audit.Map
			require.NoError(t, json.Unmarshal(row.AuditLog.Diff, &diff))
			require.Equal(t, want[i], diff)
			require.EqualValues(t, status, row.AuditLog.StatusCode)
			require.Equal(t, firstUser.UserID, row.AuditLog.UserID)
			require.Equal(t, orgID, row.AuditLog.OrganizationID)
			require.Equal(t, project.Name, row.AuditLog.ResourceTarget)
		}
	}
	// Logs are newest first.
	requireTemplateAudits(project, http.StatusNoContent,
		audit.Map{"template_ids": {Old: attached, New: detached}},
		audit.Map{"template_ids": {Old: detached, New: attached}},
	)
	requireTemplateAudits(createdWithTemplate, http.StatusCreated,
		audit.Map{"template_ids": {Old: detached, New: attached}},
	)
}
