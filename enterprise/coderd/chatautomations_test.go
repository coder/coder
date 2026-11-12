package coderd_test

import (
	"bytes"
	"database/sql"
	"encoding/base64"
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
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/testutil"
)

// createChatAutomationSchemes creates one webhook automation of each scheme as
// the caller. supplied is the secret of the standard_webhooks automation.
func createChatAutomationSchemes(t *testing.T, client *codersdk.Client, orgID, modelID uuid.UUID, prompt, supplied string) map[codersdk.ChatAutomationWebhookAuth]codersdk.CreateChatAutomationResponse {
	t.Helper()
	created := map[codersdk.ChatAutomationWebhookAuth]codersdk.CreateChatAutomationResponse{}
	for _, auth := range []codersdk.ChatAutomationWebhookAuth{
		codersdk.ChatAutomationWebhookAuthBearer, codersdk.ChatAutomationWebhookAuthGitHub, codersdk.ChatAutomationWebhookAuthStandardWebhooks,
	} {
		req := codersdk.CreateChatAutomationRequest{
			Name: "automation-" + uuid.NewString(), Kind: codersdk.ChatAutomationKindWebhook, Prompt: prompt,
			TargetMode: codersdk.ChatAutomationTargetModeNewChat, NewChatModelConfigID: &modelID, WebhookAuth: auth,
		}
		if auth == codersdk.ChatAutomationWebhookAuthStandardWebhooks {
			req.WebhookSecret = supplied
		}
		res, err := codersdk.NewExperimentalClient(client).CreateChatAutomation(testutil.Context(t, testutil.WaitMedium), orgID, req)
		require.NoError(t, err)
		created[auth] = res
	}
	return created
}

// canaryWhsec returns a secret that passes the standard_webhooks check.
func canaryWhsec() string {
	return "whsec_" + base64.StdEncoding.EncodeToString([]byte("canary-"+uuid.NewString()))
}

// The audit entry of a create marks the secret and the prompt as changed
// without their values, for every scheme.
func TestChatAutomationCreateAuditEntry(t *testing.T) {
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
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.ScopedRoleAgentsAccess(first.OrganizationID))
	auditorClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.RoleAuditor())
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: first.OrganizationID})
	prompt := "canary-prompt-" + uuid.NewString()
	supplied := canaryWhsec()
	created := createChatAutomationSchemes(t, memberClient, first.OrganizationID, model.ID, prompt, supplied)

	ctx := testutil.Context(t, testutil.WaitLong)
	res, err := auditorClient.Request(ctx, http.MethodGet, "/api/v2/audit?limit=50&q=resource_type:chat_automation", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	secrets := []string{prompt, supplied}
	for _, c := range created {
		secrets = append(secrets, c.WebhookSecret)
	}
	for _, secret := range secrets {
		if secret != "" {
			require.Zero(t, bytes.Count(body, []byte(secret)))
		}
	}

	var logs codersdk.AuditLogResponse
	require.NoError(t, json.Unmarshal(body, &logs))
	require.Len(t, logs.AuditLogs, len(created))
	entries := map[uuid.UUID]codersdk.AuditLog{}
	for _, entry := range logs.AuditLogs {
		entries[entry.ResourceID] = entry
	}
	for auth, c := range created {
		entry, ok := entries[c.Automation.ID]
		require.True(t, ok, auth)
		require.Equal(t, codersdk.AuditActionCreate, entry.Action, auth)
		require.Equal(t, codersdk.ResourceTypeChatAutomation, entry.ResourceType, auth)
		require.Equal(t, first.OrganizationID, entry.OrganizationID, auth)
		for _, field := range []string{"webhook_secret", "prompt"} {
			diff, ok := entry.Diff[field]
			require.True(t, ok, "%s %s", auth, field)
			require.True(t, diff.Secret, "%s %s", auth, field)
			require.Empty(t, diff.Old, "%s %s", auth, field)
			require.Empty(t, diff.New, "%s %s", auth, field)
		}
	}
}

// With an encryption key, the stored secret is ciphertext under that key, for
// every scheme, and the create still returns the generated secret.
func TestChatAutomationCreateEncrypted(t *testing.T) {
	t.Parallel()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ciphers, err := dbcrypt.NewCiphers([]byte(strings.ReplaceAll(uuid.NewString(), "-", "")))
	require.NoError(t, err)
	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentChatAutomations)}
	ownerClient, first := coderdenttest.New(t, &coderdenttest.Options{
		ExternalTokenEncryption: ciphers,
		Options:                 &coderdtest.Options{DeploymentValues: values, Database: db, Pubsub: ps, ChatWorkerDisabled: true},
		LicenseOptions:          &coderdenttest.LicenseOptions{Features: license.Features{codersdk.FeatureExternalTokenEncryption: 1}},
	})
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.ScopedRoleAgentsAccess(first.OrganizationID))
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: first.OrganizationID})
	supplied := canaryWhsec()

	created := createChatAutomationSchemes(t, memberClient, first.OrganizationID, model.ID, "Review the alert.", supplied)

	for auth, c := range created {
		var raw, keyID sql.NullString
		require.NoError(t, sqlDB.QueryRowContext(testutil.Context(t, testutil.WaitShort),
			"SELECT webhook_secret, webhook_secret_key_id FROM chat_automations WHERE id = $1", c.Automation.ID).Scan(&raw, &keyID), auth)
		require.True(t, raw.Valid, auth)
		require.Equal(t, ciphers[0].HexDigest(), keyID.String, auth)
		require.Zero(t, strings.Count(raw.String, supplied), auth)
		if auth == codersdk.ChatAutomationWebhookAuthStandardWebhooks {
			require.Empty(t, c.WebhookSecret)
			continue
		}
		require.NotEmpty(t, c.WebhookSecret, auth)
		require.Zero(t, strings.Count(raw.String, c.WebhookSecret), auth)
	}
}
