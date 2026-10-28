package coderd_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestFoundationAudit(t *testing.T) {
	t.Parallel()
	auditor := audit.NewMock()
	values := coderdtest.DeploymentValues(t)
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentTemplateResources)}
	client := coderdtest.New(t, &coderdtest.Options{DeploymentValues: values, Auditor: auditor})
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	f, err := client.CreateFoundation(ctx, owner.OrganizationID, codersdk.CreateFoundationRequest{Name: "audited"})
	require.NoError(t, err)
	revision, err := client.CreateFoundationRevision(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# audited source"})
	require.NoError(t, err)
	run, err := client.CreateFoundationRun(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "plan"})
	require.NoError(t, err)
	require.NoError(t, client.CancelFoundationRun(ctx, owner.OrganizationID, f.ID, run.ID))
	_, err = client.UpdateFoundation(ctx, owner.OrganizationID, f.ID, codersdk.UpdateFoundationRequest{Name: "renamed"})
	require.NoError(t, err)
	require.NoError(t, client.DeleteFoundation(ctx, owner.OrganizationID, f.ID))
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		var logs []database.AuditLog
		for _, log := range auditor.AuditLogs() {
			if log.ResourceType == database.ResourceTypeFoundation {
				logs = append(logs, log)
			}
		}
		require.Len(t, logs, 6)
		for i, action := range []database.AuditAction{database.AuditActionCreate, database.AuditActionWrite, database.AuditActionStart, database.AuditActionStop, database.AuditActionWrite, database.AuditActionDelete} {
			require.Equal(t, action, logs[i].Action)
			require.Equal(t, f.ID, logs[i].ResourceID)
			require.Equal(t, owner.OrganizationID, logs[i].OrganizationID)
		}
		var fields map[string]any
		require.NoError(t, json.Unmarshal(logs[2].AdditionalFields, &fields))
		require.Equal(t, "plan", fields["action"])
		require.Equal(t, revision.ID.String(), fields["revision_id"])
		require.Equal(t, run.ID.String(), fields["run_id"])
	}, testutil.WaitShort, testutil.IntervalFast)
}
