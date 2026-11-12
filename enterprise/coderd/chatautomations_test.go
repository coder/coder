package coderd_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// The audit entry of a delete marks the secret and the prompt as changed
// without their values.
func TestChatAutomationDeleteAuditEntry(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentChatAutomations)}
	ownerClient, first := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			DeploymentValues: values, Database: db, Pubsub: ps, ChatWorkerDisabled: true,
			Auditor: entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true)),
		},
		LicenseOptions: &coderdenttest.LicenseOptions{Features: license.Features{codersdk.FeatureAuditLog: 1}},
	})
	memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID)
	secret := "canary-secret-" + strings.Repeat(uuid.NewString(), 2)
	prompt := "canary-prompt-" + uuid.NewString()
	automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{
		OrganizationID: first.OrganizationID, OwnerID: member.ID, Prompt: prompt,
		WebhookSecret: sql.NullString{String: secret, Valid: true},
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	require.NoError(t, codersdk.NewExperimentalClient(memberClient).DeleteChatAutomation(ctx, first.OrganizationID, automation.ID))

	auditorClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.RoleAuditor())
	res, err := auditorClient.Request(ctx, http.MethodGet, "/api/v2/audit?limit=50&q=resource_type:chat_automation", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, 0, bytes.Count(body, []byte(secret)))
	require.Equal(t, 0, bytes.Count(body, []byte(prompt)))

	var logs codersdk.AuditLogResponse
	require.NoError(t, json.Unmarshal(body, &logs))
	require.Len(t, logs.AuditLogs, 1)
	entry := logs.AuditLogs[0]
	require.Equal(t, codersdk.AuditActionDelete, entry.Action)
	require.Equal(t, codersdk.ResourceTypeChatAutomation, entry.ResourceType)
	require.Equal(t, automation.ID, entry.ResourceID)
	require.Equal(t, first.OrganizationID, entry.OrganizationID)
	for _, field := range []string{"webhook_secret", "prompt"} {
		diff, ok := entry.Diff[field]
		require.True(t, ok, field)
		require.True(t, diff.Secret, field)
		require.Empty(t, diff.Old, field)
		require.Empty(t, diff.New, field)
	}
}
