package coderd_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectSkillsAudit(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentChatProjects)}
	ownerClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			DeploymentValues: dv,
			Database:         db,
			Pubsub:           ps,
			Auditor:          entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true)),
		},
		LicenseOptions: &coderdenttest.LicenseOptions{Features: license.Features{codersdk.FeatureAuditLog: 1}},
	})
	owner := codersdk.NewExperimentalClient(ownerClient)
	orgID := firstUser.OrganizationID
	ctx := testutil.Context(t, testutil.WaitLong)

	project, err := owner.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{Name: "Audited Project"})
	require.NoError(t, err)
	skill, err := owner.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{Content: "---\nname: deploy\ndescription: Deploy\n---\n\nBody.\n"})
	require.NoError(t, err)
	_, err = owner.UpdateChatProjectSkill(ctx, orgID, project.ID, skill.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
	require.NoError(t, err)
	require.NoError(t, owner.DeleteChatProjectSkill(ctx, orgID, project.ID, skill.Name))

	rows, err := db.GetAuditLogsOffset(dbauthz.AsSystemRestricted(ctx), database.GetAuditLogsOffsetParams{
		ResourceType: string(database.ResourceTypeChatProjectSkill), ResourceID: skill.ID, LimitOpt: 10,
	})
	require.NoError(t, err)
	// Logs are newest first.
	require.Len(t, rows, 3)
	for i, action := range []database.AuditAction{database.AuditActionDelete, database.AuditActionWrite, database.AuditActionCreate} {
		require.Equal(t, action, rows[i].AuditLog.Action)
		require.Equal(t, skill.Name, rows[i].AuditLog.ResourceTarget)
		require.Equal(t, orgID, rows[i].AuditLog.OrganizationID)
	}
	var diff audit.Map
	require.NoError(t, json.Unmarshal(rows[1].AuditLog.Diff, &diff))
	require.Equal(t, audit.Map{"enabled": {Old: true, New: false}}, diff)
}
