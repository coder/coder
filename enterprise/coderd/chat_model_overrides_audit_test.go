package coderd_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatModelOverrideAudit(t *testing.T) {
	t.Parallel()

	f := newChatAuditFixture(t)
	model := dbgen.ChatModelConfig(t, f.db, database.ChatModelConfig{
		Options: json.RawMessage(`{"reasoning_effort":{"default":"medium","max":"high"}}`),
	})
	dbgen.AIProviderKey(t, f.db, database.AIProviderKey{ProviderID: model.AIProviderID.UUID})
	orgID := model.OrganizationID

	check := func(t *testing.T, log database.AuditLog, action database.AuditAction, rowID uuid.UUID, target string, diff audit.Map) {
		t.Helper()
		require.Equal(t, action, log.Action)
		require.Equal(t, orgID, log.OrganizationID)
		require.Equal(t, rowID, log.ResourceID)
		require.Equal(t, target, log.ResourceTarget)
		requireChatAuditDiff(t, log, diff)
	}

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()

		f := f
		f.ctx = testutil.Context(t, testutil.WaitLong)
		f.systemCtx = dbauthz.AsSystemRestricted(f.ctx)
		const override = codersdk.ChatModelOverrideContextAdvisor
		write := func(req codersdk.UpdateChatModelOverrideRequest) {
			t.Helper()
			_, err := f.client.UpdateOrganizationChatModelOverride(f.ctx, orgID, override, req)
			require.NoError(t, err)
		}
		req := codersdk.UpdateChatModelOverrideRequest{ModelConfigID: model.ID.String()}
		write(req)
		row, err := f.db.GetChatOrganizationModelOverride(f.systemCtx, database.GetChatOrganizationModelOverrideParams{
			OrganizationID: orgID, Context: string(override),
		})
		require.NoError(t, err)
		req.ReasoningEffort = new("high")
		write(req)
		write(req)
		write(codersdk.UpdateChatModelOverrideRequest{})
		write(codersdk.UpdateChatModelOverrideRequest{})

		logs := f.auditLogs(t, "chat_organization_model_override")
		require.Len(t, logs, 3)
		check(t, logs[2].AuditLog, database.AuditActionCreate, row.ID, string(override), audit.Map{
			"model_config_id": {Old: "", New: model.ID.String()},
		})
		check(t, logs[1].AuditLog, database.AuditActionWrite, row.ID, string(override), audit.Map{
			"reasoning_effort": {Old: "null", New: "high"},
		})
		check(t, logs[0].AuditLog, database.AuditActionDelete, row.ID, string(override), audit.Map{
			"model_config_id":  {Old: model.ID.String(), New: ""},
			"reasoning_effort": {Old: "high", New: "null"},
		})
	})

	t.Run("Personal", func(t *testing.T) {
		t.Parallel()

		f := f
		f.ctx = testutil.Context(t, testutil.WaitLong)
		f.systemCtx = dbauthz.AsSystemRestricted(f.ctx)
		_, member := coderdtest.CreateAnotherUser(t, f.client.Client, orgID)
		const override = codersdk.ChatPersonalModelOverrideContextGeneral
		write := func(req codersdk.UpdateUserChatPersonalModelOverrideRequest) {
			t.Helper()
			require.NoError(t, f.client.UpdateUserChatPersonalModelOverride(f.ctx, orgID, member.ID.String(), override, req))
		}
		write(codersdk.UpdateUserChatPersonalModelOverrideRequest{Mode: codersdk.ChatPersonalModelOverrideModeChatDefault})
		row, err := f.db.GetChatUserModelOverride(f.systemCtx, database.GetChatUserModelOverrideParams{
			UserID: member.ID, OrganizationID: orgID, Context: string(override),
		})
		require.NoError(t, err)
		req := codersdk.UpdateUserChatPersonalModelOverrideRequest{
			Mode: codersdk.ChatPersonalModelOverrideModeModel, ModelConfigID: model.ID.String(), ReasoningEffort: new("medium"),
		}
		write(req)
		write(req)

		logs := f.auditLogs(t, "chat_user_model_override")
		require.Len(t, logs, 2)
		target := member.ID.String() + "/" + string(override)
		check(t, logs[1].AuditLog, database.AuditActionCreate, row.ID, target, audit.Map{
			"mode": {Old: "", New: "chat_default"},
		})
		check(t, logs[0].AuditLog, database.AuditActionWrite, row.ID, target, audit.Map{
			"mode":             {Old: "chat_default", New: "model"},
			"model_config_id":  {Old: "null", New: model.ID.String()},
			"reasoning_effort": {Old: "null", New: "medium"},
		})
	})
}
