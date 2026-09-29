package coderd_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// TestChatAutomationsEnterprise covers what only an Enterprise deployment
// exercises: the automation routes coexist with the Enterprise
// /api/experimental/organizations/{organization}/ai/spend routes, and the
// real auditor, including its server-log backend, never records the
// webhook secret or its hash.
func TestChatAutomationsEnterprise(t *testing.T) {
	t.Parallel()

	sink := testutil.NewFakeSink(t)
	logger := sink.Logger()
	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(
		db,
		entaudit.DefaultFilter,
		backends.NewPostgres(db, true),
		backends.NewSlog(logger),
	)
	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	dv.Experiments = []string{string(codersdk.ExperimentChatAutomations)}
	client, owner := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			DeploymentValues:   dv,
			Database:           db,
			Pubsub:             ps,
			Auditor:            auditor,
			Logger:             &logger,
			ChatWorkerDisabled: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAIBridge: 1,
				codersdk.FeatureAuditLog: 1,
			},
		},
	})
	exp := codersdk.NewExperimentalClient(client)
	ctx := testutil.Context(t, testutil.WaitLong)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: owner.OrganizationID})

	created, err := exp.CreateChatAutomation(ctx, owner.OrganizationID, codersdk.CreateChatAutomationRequest{
		Name:                 "Deploy hook",
		Kind:                 codersdk.ChatAutomationKindWebhook,
		TargetMode:           codersdk.ChatAutomationTargetModeNewChat,
		NewChatModelConfigID: &modelConfig.ID,
		Prompt:               "A deploy finished.",
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.WebhookSecret)
	_, err = exp.UpdateChatAutomation(ctx, owner.OrganizationID, created.Automation.ID, codersdk.UpdateChatAutomationRequest{Name: ptr.Ref("Renamed")})
	require.NoError(t, err)
	require.NoError(t, exp.DeleteChatAutomation(ctx, owner.OrganizationID, created.Automation.ID))

	// Both experimental organization route groups resolve.
	_, err = exp.OrganizationAISpendUsers(ctx, owner.OrganizationID, codersdk.OrganizationAISpendFilter{}, codersdk.OrganizationAISpendPage{})
	require.NoError(t, err)

	stored, err := db.GetAuditLogsOffset(dbauthz.AsSystemRestricted(ctx), database.GetAuditLogsOffsetParams{
		ResourceType: string(database.ResourceTypeChatAutomation),
		LimitOpt:     10,
	})
	require.NoError(t, err)
	require.Len(t, stored, 3, "create, update, and delete are audited")

	hashes := webhookSecretHashEncodings(created.WebhookSecret)
	forbidden := append([]string{created.WebhookSecret}, hashes...)
	redactedDiffs := 0
	for _, row := range stored {
		text := string(row.AuditLog.Diff) + string(row.AuditLog.AdditionalFields)
		for _, value := range forbidden {
			require.NotContains(t, text, value, "audit %s", row.AuditLog.Action)
		}
		if strings.Contains(string(row.AuditLog.Diff), "webhook_secret_hash") {
			redactedDiffs++
		}
	}
	require.NotZero(t, redactedDiffs, "the hash must reach the diff so its redaction is exercised")
	auditLines := 0
	for _, entry := range sink.Entries() {
		text := logEntryText(entry)
		if entry.Message == "audit_log" {
			auditLines++
		}
		for _, value := range forbidden {
			require.NotContains(t, text, value)
		}
	}
	require.NotZero(t, auditLines, "the slog audit backend must have logged the entries")
}

// webhookSecretHashEncodings returns the encodings a leaked SHA-256 hash
// of secret could appear in.
func webhookSecretHashEncodings(secret string) []string {
	sum := sha256.Sum256([]byte(secret))
	return []string{
		hex.EncodeToString(sum[:]),
		base64.StdEncoding.EncodeToString(sum[:]),
		base64.RawURLEncoding.EncodeToString(sum[:]),
	}
}

// logEntryText renders a log entry with byte slice fields, such as the
// audit diff, as text so that substring checks can see their content.
func logEntryText(entry slog.SinkEntry) string {
	var b strings.Builder
	_, _ = b.WriteString(entry.Message)
	for _, field := range entry.Fields {
		switch value := field.Value.(type) {
		case json.RawMessage:
			_, _ = fmt.Fprintf(&b, " %s=%s", field.Name, value)
		case []byte:
			_, _ = fmt.Fprintf(&b, " %s=%s", field.Name, value)
		default:
			_, _ = fmt.Fprintf(&b, " %s=%+v", field.Name, value)
		}
	}
	return b.String()
}
