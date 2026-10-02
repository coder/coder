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
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

//nolint:tparallel,paralleltest // Subtests share one organization's prompt and run sequentially.
func TestOrganizationChatSystemPrompt(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(db, entaudit.DefaultFilter, backends.NewPostgres(db, true))
	client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
			Auditor:  auditor,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAuditLog:              1,
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	owner := codersdk.NewExperimentalClient(client)
	orgID := firstUser.OrganizationID
	otherOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

	newClient := func(orgID uuid.UUID, roles ...rbac.RoleIdentifier) (*codersdk.ExperimentalClient, codersdk.User) {
		raw, user := coderdtest.CreateAnotherUser(t, client, orgID, roles...)
		return codersdk.NewExperimentalClient(raw), user
	}
	orgAdmin, _ := newClient(orgID, rbac.ScopedRoleOrgAdmin(orgID))
	orgAuditor, _ := newClient(orgID, rbac.ScopedRoleOrgAuditor(orgID))
	member, _ := newClient(orgID)
	otherOrgAdmin, _ := newClient(otherOrg.ID, rbac.ScopedRoleOrgAdmin(otherOrg.ID))

	t.Run("Authorization", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		cases := []struct {
			name      string
			client    *codersdk.ExperimentalClient
			getStatus int
			putStatus int
		}{
			{name: "Owner", client: owner, getStatus: http.StatusOK, putStatus: http.StatusNoContent},
			{name: "OrgAdmin", client: orgAdmin, getStatus: http.StatusOK, putStatus: http.StatusNoContent},
			{name: "OrgAuditor", client: orgAuditor, getStatus: http.StatusOK, putStatus: http.StatusForbidden},
			{name: "Member", client: member, getStatus: http.StatusNotFound, putStatus: http.StatusForbidden},
			{name: "OtherOrgAdmin", client: otherOrgAdmin, getStatus: http.StatusNotFound, putStatus: http.StatusNotFound},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				prompt := "Prompt set by " + tc.name
				err := tc.client.UpdateOrganizationChatSystemPrompt(ctx, orgID, codersdk.UpdateOrganizationChatSystemPromptRequest{
					SystemPrompt: prompt,
				})
				if tc.putStatus == http.StatusNoContent {
					require.NoError(t, err)
				} else {
					requireStatusCode(t, err, tc.putStatus)
				}

				resp, err := tc.client.OrganizationChatSystemPrompt(ctx, orgID)
				if tc.getStatus != http.StatusOK {
					requireStatusCode(t, err, tc.getStatus)
					return
				}
				require.NoError(t, err)
				if tc.putStatus == http.StatusNoContent {
					require.Equal(t, prompt, resp.SystemPrompt)
				}
			})
		}

		// Denied writes leave the last successful value in place.
		resp, err := owner.OrganizationChatSystemPrompt(ctx, orgID)
		require.NoError(t, err)
		require.Equal(t, "Prompt set by OrgAdmin", resp.SystemPrompt)
	})

	t.Run("SanitizesInvisibleCharacters", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		err := orgAdmin.UpdateOrganizationChatSystemPrompt(ctx, orgID, codersdk.UpdateOrganizationChatSystemPromptRequest{
			SystemPrompt: "  Use the\u200b team\u2060 template.\r\n",
		})
		require.NoError(t, err)
		resp, err := orgAdmin.OrganizationChatSystemPrompt(ctx, orgID)
		require.NoError(t, err)
		require.Equal(t, "Use the team template.", resp.SystemPrompt)
	})

	t.Run("UnsetOrganizationReturnsEmpty", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := otherOrgAdmin.OrganizationChatSystemPrompt(ctx, otherOrg.ID)
		require.NoError(t, err)
		require.Empty(t, resp.SystemPrompt)
	})

	t.Run("Audit", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		auditOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
		auditOrgMember, _ := newClient(auditOrg.ID)
		logs := func(t *testing.T) []database.AuditLog {
			t.Helper()
			rows, err := db.GetAuditLogsOffset(dbauthz.AsSystemRestricted(ctx), database.GetAuditLogsOffsetParams{
				ResourceType: string(database.ResourceTypeChatOrganizationSystemPrompt),
				ResourceID:   auditOrg.ID,
				LimitOpt:     10,
			})
			require.NoError(t, err)
			out := make([]database.AuditLog, 0, len(rows))
			for _, row := range rows {
				out = append(out, row.AuditLog)
			}
			return out
		}

		update := func(c *codersdk.ExperimentalClient, prompt string) error {
			return c.UpdateOrganizationChatSystemPrompt(ctx, auditOrg.ID, codersdk.UpdateOrganizationChatSystemPromptRequest{
				SystemPrompt: prompt,
			})
		}

		require.NoError(t, update(owner, "First"))
		require.NoError(t, update(owner, "Second"))
		got := logs(t)
		require.Len(t, got, 2)
		for i, want := range []audit.Map{
			{"system_prompt": {Old: "First", New: "Second"}},
			{"system_prompt": {Old: "", New: "First"}},
		} {
			var diff audit.Map
			require.NoError(t, json.Unmarshal(got[i].Diff, &diff))
			require.Equal(t, want, diff)
			require.Equal(t, database.AuditActionWrite, got[i].Action)
			require.Equal(t, auditOrg.ID, got[i].OrganizationID)
			require.Equal(t, auditOrg.ID, got[i].ResourceID)
			require.EqualValues(t, http.StatusNoContent, got[i].StatusCode)
		}

		// An unchanged prompt is not audited.
		require.NoError(t, update(owner, "Second"))
		require.Len(t, logs(t), 2)

		// A denied attempt is audited with its status and no diff.
		requireStatusCode(t, update(auditOrgMember, "Denied"), http.StatusForbidden)
		got = logs(t)
		require.Len(t, got, 3)
		require.EqualValues(t, http.StatusForbidden, got[0].StatusCode)
		require.Equal(t, auditOrg.ID, got[0].OrganizationID)
		require.JSONEq(t, "{}", string(got[0].Diff))
	})

	t.Run("AppliedToNewChatsInOrganizationOnly", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		provider := createOpenAIProviderForTest(ctx, t, owner, "test-key", "https://example.com")
		createModel := func(orgID uuid.UUID) uuid.UUID {
			model, err := owner.CreateChatModel(ctx, orgID, codersdk.CreateChatModelRequest{
				AIProviderID:         &provider.ID,
				Model:                "gpt-4o-mini",
				DisplayName:          "Model " + orgID.String(),
				ContextLimit:         new(int64(1000)),
				CompressionThreshold: new(int32(70)),
			})
			require.NoError(t, err)
			return model.ID
		}
		systemTexts := func(orgID, modelID uuid.UUID) []string {
			t.Helper()
			chat, err := owner.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: orgID,
				ModelConfigID:  &modelID,
				Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
			})
			require.NoError(t, err)
			messages, err := db.GetChatMessagesForPromptByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
			require.NoError(t, err)
			var texts []string
			for _, message := range messages {
				if message.Role != database.ChatMessageRoleSystem {
					continue
				}
				parts, err := chatprompt.ParseContent(message)
				require.NoError(t, err)
				require.Len(t, parts, 1)
				texts = append(texts, parts[0].Text)
			}
			return texts
		}

		orgModel := createModel(orgID)
		otherModel := createModel(otherOrg.ID)
		const orgPrompt = "Organization instructions for agents."
		require.NoError(t, owner.UpdateOrganizationChatSystemPrompt(ctx, orgID, codersdk.UpdateOrganizationChatSystemPromptRequest{
			SystemPrompt: orgPrompt,
		}))

		baseline := systemTexts(otherOrg.ID, otherModel)
		require.NotContains(t, baseline, orgPrompt)
		require.GreaterOrEqual(t, len(baseline), 2, "deployment and workspace awareness rows")
		require.Equal(t, chatd.DefaultSystemPrompt, baseline[0])

		// The organization row follows the deployment row and leaves the
		// rest of the initial system rows unchanged.
		want := append([]string{baseline[0], orgPrompt}, baseline[1:]...)
		require.Equal(t, want, systemTexts(orgID, orgModel))

		// Clearing the prompt restores the baseline rows.
		require.NoError(t, owner.UpdateOrganizationChatSystemPrompt(ctx, orgID, codersdk.UpdateOrganizationChatSystemPromptRequest{}))
		require.Equal(t, baseline, systemTexts(orgID, orgModel))
	})
}
