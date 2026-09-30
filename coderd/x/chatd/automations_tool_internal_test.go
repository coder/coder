package chatd //nolint:testpackage // Exercises the unexported manage_automations tool and turn helper.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAutomationTurnTriggerFromHistory(t *testing.T) {
	t.Parallel()

	automationID := uuid.New()
	inputID := uuid.New()
	laterAutomationID := uuid.New()
	laterInputID := uuid.New()
	human := func() database.ChatMessage {
		return database.ChatMessage{Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth}
	}
	fromAutomation := func(automation, input uuid.UUID) database.ChatMessage {
		m := human()
		m.AutomationID = uuid.NullUUID{UUID: automation, Valid: true}
		m.InputID = uuid.NullUUID{UUID: input, Valid: true}
		return m
	}
	assistant := database.ChatMessage{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth}
	tool := database.ChatMessage{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth}
	deleted := func(m database.ChatMessage) database.ChatMessage {
		m.Deleted = true
		return m
	}
	compressed := func(m database.ChatMessage) database.ChatMessage {
		m.Compressed = true
		return m
	}
	// Compaction replays pending user rows as model-only rows without
	// automation_id. The chat-history query omits them, but the helper
	// must not depend on that.
	replayed := database.ChatMessage{Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityModel}

	reached := automationTurnTrigger{
		AutomationID: uuid.NullUUID{UUID: automationID, Valid: true},
		InputID:      uuid.NullUUID{UUID: inputID, Valid: true},
	}
	for _, tc := range []struct {
		name     string
		messages []database.ChatMessage
		want     automationTurnTrigger
	}{
		{name: "Empty"},
		{name: "HumanTurn", messages: []database.ChatMessage{human(), assistant, human()}},
		{
			name:     "AutomationTurn",
			messages: []database.ChatMessage{human(), assistant, fromAutomation(automationID, inputID), assistant, tool},
			want:     reached,
		},
		{
			// FinishTurn promotes the queued automation message into the
			// history, which starts a new turn the automation reached.
			name:     "PromotedAfterHumanTurn",
			messages: []database.ChatMessage{human(), assistant, tool, assistant, fromAutomation(automationID, inputID)},
			want:     reached,
		},
		{
			name:     "HumanAfterAutomationWithoutResponse",
			messages: []database.ChatMessage{assistant, fromAutomation(automationID, inputID), human()},
			want:     reached,
		},
		{
			name:     "LatestAutomationRowIsTrigger",
			messages: []database.ChatMessage{assistant, fromAutomation(automationID, inputID), fromAutomation(laterAutomationID, laterInputID)},
			want: automationTurnTrigger{
				AutomationID: uuid.NullUUID{UUID: laterAutomationID, Valid: true},
				InputID:      uuid.NullUUID{UUID: laterInputID, Valid: true},
			},
		},
		{name: "HumanTurnAfterAutomationTurn", messages: []database.ChatMessage{fromAutomation(automationID, inputID), assistant, human()}},
		{
			// The original row keeps its automation_id; the replay after
			// the summary boundary does not.
			name: "CompactionReplay",
			messages: []database.ChatMessage{
				human(), assistant, fromAutomation(automationID, inputID), assistant,
				compressed(replayed), compressed(assistant), compressed(tool), replayed,
			},
			want: reached,
		},
		{
			// Editing an automation message deletes it and inserts a human
			// message in its place.
			name:     "EditedAutomationMessage",
			messages: []database.ChatMessage{assistant, deleted(fromAutomation(automationID, inputID)), human()},
		},
		{
			name:     "DeletedAssistantDoesNotJoinTurns",
			messages: []database.ChatMessage{fromAutomation(automationID, inputID), deleted(assistant), human()},
			want:     reached,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, automationTurnTriggerFromHistory(tc.messages))
		})
	}
}

func TestManageAutomationsNotOfferedInPlanOrExploreTurns(t *testing.T) {
	t.Parallel()

	// Offering already requires a root chat outside plan and explore mode;
	// the plan and explore allowlists must not let the tool back in.
	tools := []fantasy.AgentTool{newTestAgentTool("read_file"), newTestAgentTool(manageAutomationsToolName)}
	plan := database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
	require.NotContains(t, activeToolNamesForTurn(tools, plan, uuid.NullUUID{}, nil), manageAutomationsToolName)
	require.NotContains(t, allowedExploreToolNames(tools), manageAutomationsToolName)
}

// manageAutomationsFixture is a schedule fixture whose chat has the
// manage_automations switch on.
type manageAutomationsFixture struct {
	*scheduleFixture
	server *Server
}

func newManageAutomationsFixture(t *testing.T) manageAutomationsFixture {
	t.Helper()
	f := newScheduleFixture(t, database.ChatStatusWaiting, time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC))
	f.setSwitch(t, f.chat.ID, true)
	return manageAutomationsFixture{scheduleFixture: f, server: f.newServer(t, Limits{})}
}

func (f *scheduleFixture) setSwitch(t *testing.T, chatID uuid.UUID, enabled bool) {
	t.Helper()
	_, err := f.db.UpdateChatManageAutomationsEnabledByID(testutil.Context(t, testutil.WaitShort), database.UpdateChatManageAutomationsEnabledByIDParams{
		ID:                       chatID,
		ManageAutomationsEnabled: enabled,
	})
	require.NoError(t, err)
}

// call runs the tool for chatID and returns the raw result and whether it
// is an error.
func (f manageAutomationsFixture) call(ctx context.Context, t *testing.T, chatID uuid.UUID, action string, automationID uuid.UUID) (string, bool) {
	t.Helper()
	args := manageAutomationsArgs{Action: action}
	if automationID != uuid.Nil {
		args.AutomationID = automationID.String()
	}
	input, err := json.Marshal(args)
	require.NoError(t, err)
	resp, err := f.server.manageAutomationsTool(chatID).Run(ctx, fantasy.ToolCall{ID: "call", Name: manageAutomationsToolName, Input: string(input)})
	require.NoError(t, err)
	return resp.Content, resp.IsError
}

func (f manageAutomationsFixture) listIDs(ctx context.Context, t *testing.T, chatID uuid.UUID) []uuid.UUID {
	t.Helper()
	content, isError := f.call(ctx, t, chatID, "list", uuid.Nil)
	require.False(t, isError, content)
	var result struct {
		Automations []codersdk.ChatAutomation `json:"automations"`
	}
	require.NoError(t, json.Unmarshal([]byte(content), &result))
	ids := make([]uuid.UUID, 0, len(result.Automations))
	for _, a := range result.Automations {
		ids = append(ids, a.ID)
	}
	return ids
}

// newChatAutomation inserts an enabled new_chat schedule automation.
func newChatAutomation(t *testing.T, db database.Store, orgID, ownerID, modelID uuid.UUID) database.ChatAutomation {
	t.Helper()
	return dbgen.ChatAutomation(t, db, database.ChatAutomation{
		OrganizationID:       orgID,
		OwnerID:              ownerID,
		Kind:                 database.ChatAutomationKindSchedule,
		TargetMode:           database.ChatAutomationTargetModeNewChat,
		NewChatModelConfigID: uuid.NullUUID{UUID: modelID, Valid: true},
		Enabled:              true,
	})
}

func requireAuditFields(t *testing.T, log database.AuditLog, want map[string]string) {
	t.Helper()
	var got map[string]string
	require.NoError(t, json.Unmarshal(log.AdditionalFields, &got))
	require.Equal(t, want, got)
}

func TestManageAutomationsTool(t *testing.T) {
	t.Parallel()

	t.Run("ActionsAndAudit", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		webhook, secret, err := f.server.CreateAutomation(f.asOwner(ctx, t), CreateAutomationParams{
			OrganizationID: f.org.ID,
			OwnerID:        f.owner.ID,
			Request: codersdk.CreateChatAutomationRequest{
				Name:         "Deploy hook",
				Kind:         codersdk.ChatAutomationKindWebhook,
				TargetMode:   codersdk.ChatAutomationTargetModeExistingChat,
				TargetChatID: &f.chat.ID,
				Prompt:       "A deploy finished.",
			},
		})
		require.NoError(t, err)
		require.NotEmpty(t, secret)
		require.NotEmpty(t, webhook.WebhookSecretHash)
		schedule := f.newChat(ctx, t, f.server, "0 9 * * *", "UTC")
		requireNoSecret := func(content string) {
			t.Helper()
			require.NotContains(t, content, secret)
			require.NotContains(t, content, hex.EncodeToString(webhook.WebhookSecretHash))
			require.NotContains(t, content, base64.StdEncoding.EncodeToString(webhook.WebhookSecretHash))
		}

		require.ElementsMatch(t, []uuid.UUID{webhook.ID, schedule.ID}, f.listIDs(ctx, t, f.chat.ID))
		content, isError := f.call(ctx, t, f.chat.ID, "list", uuid.Nil)
		require.False(t, isError, content)
		requireNoSecret(content)

		content, isError = f.call(ctx, t, f.chat.ID, "get", webhook.ID)
		require.False(t, isError, content)
		require.Contains(t, content, webhook.ID.String())
		requireNoSecret(content)

		content, isError = f.call(ctx, t, f.chat.ID, "disable", webhook.ID)
		require.False(t, isError, content)
		requireNoSecret(content)
		row, err := f.db.GetChatAutomationByID(ctx, webhook.ID)
		require.NoError(t, err)
		require.False(t, row.Enabled)

		content, isError = f.call(ctx, t, f.chat.ID, "delete", schedule.ID)
		require.False(t, isError, content)
		_, err = f.db.GetChatAutomationByID(ctx, schedule.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)

		logs := f.auditor.AuditLogs()
		require.Len(t, logs, 2)
		require.Equal(t, database.AuditActionWrite, logs[0].Action)
		require.Equal(t, webhook.ID, logs[0].ResourceID)
		require.Equal(t, f.owner.ID, logs[0].UserID)
		requireAuditFields(t, logs[0], map[string]string{"chat_id": f.chat.ID.String()})
		require.Equal(t, database.AuditActionDelete, logs[1].Action)
		require.Equal(t, schedule.ID, logs[1].ResourceID)
		requireAuditFields(t, logs[1], map[string]string{"chat_id": f.chat.ID.String()})

		for _, action := range []string{"create", "update", "enable", "run_now"} {
			content, isError = f.call(ctx, t, f.chat.ID, action, webhook.ID)
			require.True(t, isError)
			require.Contains(t, content, "not available in this version")
		}
		row, err = f.db.GetChatAutomationByID(ctx, webhook.ID)
		require.NoError(t, err)
		require.False(t, row.Enabled, "enable must not run in this version")
	})

	t.Run("RechecksEveryCall", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			// revoke changes state after the tool was offered and returns
			// the chat to call the tool from.
			revoke func(ctx context.Context, t *testing.T, f manageAutomationsFixture) uuid.UUID
		}{
			{name: "SwitchOff", revoke: func(_ context.Context, t *testing.T, f manageAutomationsFixture) uuid.UUID {
				f.setSwitch(t, f.chat.ID, false)
				return f.chat.ID
			}},
			{name: "ExperimentOffForOwner", revoke: func(_ context.Context, _ *testing.T, f manageAutomationsFixture) uuid.UUID {
				f.experiment.offFor.Store(&f.owner.ID)
				return f.chat.ID
			}},
			{name: "PlanMode", revoke: func(ctx context.Context, t *testing.T, f manageAutomationsFixture) uuid.UUID {
				_, err := f.db.UpdateChatPlanModeByID(ctx, database.UpdateChatPlanModeByIDParams{
					ID:       f.chat.ID,
					PlanMode: database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
				})
				require.NoError(t, err)
				return f.chat.ID
			}},
			{name: "Archived", revoke: func(ctx context.Context, t *testing.T, f manageAutomationsFixture) uuid.UUID {
				_, err := f.sqlDB.ExecContext(ctx, "UPDATE chats SET archived = true WHERE id = $1", f.chat.ID)
				require.NoError(t, err)
				return f.chat.ID
			}},
			{name: "SubagentChat", revoke: func(_ context.Context, t *testing.T, f manageAutomationsFixture) uuid.UUID {
				return dbgen.Chat(t, f.db, database.Chat{
					OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: f.model.ID,
					ParentChatID:             uuid.NullUUID{UUID: f.chat.ID, Valid: true},
					RootChatID:               uuid.NullUUID{UUID: f.chat.ID, Valid: true},
					ManageAutomationsEnabled: true,
				}).ID
			}},
			{name: "ExploreModeChat", revoke: func(_ context.Context, t *testing.T, f manageAutomationsFixture) uuid.UUID {
				// A root chat, so only the explore rule can reject the call.
				return dbgen.Chat(t, f.db, database.Chat{
					OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: f.model.ID,
					Mode:                     database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true},
					ManageAutomationsEnabled: true,
				}).ID
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newManageAutomationsFixture(t)
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.newChat(ctx, t, f.server, "0 9 * * *", "UTC")
				// The tool works before the change.
				require.Equal(t, []uuid.UUID{automation.ID}, f.listIDs(ctx, t, f.chat.ID))

				chatID := tc.revoke(ctx, t, f)
				for _, action := range []string{"list", "disable", "delete"} {
					content, isError := f.call(ctx, t, chatID, action, automation.ID)
					require.True(t, isError, "%s: %s", action, content)
					require.Contains(t, content, "not available for this chat")
				}
				row, err := f.db.GetChatAutomationByID(ctx, automation.ID)
				require.NoError(t, err)
				require.True(t, row.Enabled)
				require.Empty(t, f.auditor.AuditLogs())
			})
		}
	})

	t.Run("ScopedToChatOwnerAndOrganization", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// The chat owner administers the organization, so it could read,
		// disable, and delete other members' automations directly.
		_, err := f.db.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
			GrantedRoles: []string{rbac.RoleAgentsAccess(), rbac.RoleOrgAdmin()},
			UserID:       f.owner.ID,
			OrgID:        f.org.ID,
		})
		require.NoError(t, err)
		member := dbgen.User(t, f.db, database.User{})
		dbgen.OrganizationMember(t, f.db, database.OrganizationMember{UserID: member.ID, OrganizationID: f.org.ID})
		otherOrg := dbgen.Organization(t, f.db, database.Organization{})
		dbgen.OrganizationMember(t, f.db, database.OrganizationMember{UserID: f.owner.ID, OrganizationID: otherOrg.ID})
		otherModel := dbgen.ChatModelConfig(t, f.db, database.ChatModelConfig{OrganizationID: otherOrg.ID})

		own := newChatAutomation(t, f.db, f.org.ID, f.owner.ID, f.model.ID)
		otherOwner := newChatAutomation(t, f.db, f.org.ID, member.ID, f.model.ID)
		otherOrgOwn := newChatAutomation(t, f.db, otherOrg.ID, f.owner.ID, otherModel.ID)

		require.Equal(t, []uuid.UUID{own.ID}, f.listIDs(ctx, t, f.chat.ID))
		for _, automation := range []database.ChatAutomation{otherOwner, otherOrgOwn} {
			for _, action := range []string{"get", "disable", "delete"} {
				content, isError := f.call(ctx, t, f.chat.ID, action, automation.ID)
				require.True(t, isError)
				require.Equal(t, "automation not found", content)
			}
			row, err := f.db.GetChatAutomationByID(ctx, automation.ID)
			require.NoError(t, err)
			require.True(t, row.Enabled)
		}
		require.Empty(t, f.auditor.AuditLogs())
	})

	t.Run("AutomationReachedTurn", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		targetsChat := f.existingChat(ctx, t, f.server, "0 9 * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		createdChat := newChatAutomation(t, f.db, f.org.ID, f.owner.ID, f.model.ID)
		unrelated := newChatAutomation(t, f.db, f.org.ID, f.owner.ID, f.model.ID)
		_, err := f.db.UpdateChatAutomationIDByID(ctx, database.UpdateChatAutomationIDByIDParams{
			ID:           f.chat.ID,
			AutomationID: createdChat.ID,
		})
		require.NoError(t, err)

		// A human turn sees every automation of the owner.
		dbgen.ChatMessage(t, f.db, database.ChatMessage{ChatID: f.chat.ID, Role: database.ChatMessageRoleUser})
		require.ElementsMatch(t, []uuid.UUID{targetsChat.ID, createdChat.ID, unrelated.ID}, f.listIDs(ctx, t, f.chat.ID))

		dbgen.ChatMessage(t, f.db, database.ChatMessage{ChatID: f.chat.ID, Role: database.ChatMessageRoleAssistant})
		inputID := uuid.New()
		dbgen.ChatMessage(t, f.db, database.ChatMessage{
			ChatID:       f.chat.ID,
			Role:         database.ChatMessageRoleUser,
			AutomationID: uuid.NullUUID{UUID: targetsChat.ID, Valid: true},
			InputID:      uuid.NullUUID{UUID: inputID, Valid: true},
		})

		require.ElementsMatch(t, []uuid.UUID{targetsChat.ID, createdChat.ID}, f.listIDs(ctx, t, f.chat.ID))
		for _, action := range []string{"get", "disable", "delete"} {
			content, isError := f.call(ctx, t, f.chat.ID, action, unrelated.ID)
			require.True(t, isError)
			require.Equal(t, "automation not found", content)
		}

		content, isError := f.call(ctx, t, f.chat.ID, "disable", createdChat.ID)
		require.False(t, isError, content)
		logs := f.auditor.AuditLogs()
		require.Len(t, logs, 1)
		requireAuditFields(t, logs[0], map[string]string{
			"chat_id":       f.chat.ID.String(),
			"automation_id": targetsChat.ID.String(),
			"input_id":      inputID.String(),
		})
	})
}
