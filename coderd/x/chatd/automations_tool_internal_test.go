package chatd

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
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
	return f.callArgs(ctx, t, chatID, args)
}

// callArgs runs the tool for chatID with args and returns the raw result
// and whether it is an error.
func (f manageAutomationsFixture) callArgs(ctx context.Context, t *testing.T, chatID uuid.UUID, args manageAutomationsArgs) (string, bool) {
	t.Helper()
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

// heartbeatArgs creates a schedule automation that targets the calling
// chat by default.
func heartbeatArgs() manageAutomationsArgs {
	return manageAutomationsArgs{
		Action:           "create",
		Name:             ptr.Ref("Heartbeat"),
		Kind:             ptr.Ref("schedule"),
		TargetMode:       ptr.Ref("existing_chat"),
		Prompt:           ptr.Ref("Check the build."),
		ScheduleCron:     ptr.Ref("*/30 * * * *"),
		ScheduleTimeZone: ptr.Ref("UTC"),
	}
}

// mustCreate runs a create call that must succeed and returns the stored
// row and the raw result.
func (f manageAutomationsFixture) mustCreate(ctx context.Context, t *testing.T, chatID uuid.UUID, args manageAutomationsArgs) (database.ChatAutomation, string) {
	t.Helper()
	content, isError := f.callArgs(ctx, t, chatID, args)
	require.False(t, isError, content)
	var result struct {
		Automation codersdk.ChatAutomation `json:"automation"`
	}
	require.NoError(t, json.Unmarshal([]byte(content), &result))
	row, err := f.db.GetChatAutomationByID(ctx, result.Automation.ID)
	require.NoError(t, err)
	return row, content
}

// modelConfig adds a model config to the fixture organization with OpenAI
// web search on or off.
func (f manageAutomationsFixture) modelConfig(t *testing.T, webSearch bool) database.ChatModelConfig {
	t.Helper()
	options, err := json.Marshal(codersdk.ChatModelCallConfig{ProviderOptions: &codersdk.ChatModelProviderOptions{
		OpenAI: &codersdk.ChatModelOpenAIProviderOptions{WebSearchEnabled: ptr.Ref(webSearch)},
	}})
	require.NoError(t, err)
	return dbgen.ChatModelConfig(t, f.db, database.ChatModelConfig{OrganizationID: f.org.ID, AIProviderID: f.model.AIProviderID, Options: options})
}

// otherChat adds another root chat of the fixture owner.
func (f manageAutomationsFixture) otherChat(t *testing.T, modelID uuid.UUID) database.Chat {
	t.Helper()
	return dbgen.Chat(t, f.db, database.Chat{OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: modelID, Status: database.ChatStatusWaiting})
}

func (f manageAutomationsFixture) ownerAutomations(ctx context.Context, t *testing.T) []database.ChatAutomation {
	t.Helper()
	rows, err := f.db.GetChatAutomationsByOrganizationIDAndOwnerID(ctx, database.GetChatAutomationsByOrganizationIDAndOwnerIDParams{
		OrganizationID: f.org.ID,
		OwnerID:        f.owner.ID,
	})
	require.NoError(t, err)
	return rows
}

func (f manageAutomationsFixture) requireUnchanged(ctx context.Context, t *testing.T, want database.ChatAutomation) {
	t.Helper()
	got, err := f.db.GetChatAutomationByID(ctx, want.ID)
	require.NoError(t, err)
	require.Equal(t, want.UpdatedAt, got.UpdatedAt)
	require.Equal(t, want.Enabled, got.Enabled)
	require.Equal(t, want.Prompt, got.Prompt)
	require.Equal(t, want.TargetChatID, got.TargetChatID)
	require.Equal(t, want.NewChatModelConfigID, got.NewChatModelConfigID)
}

// requireNoWebhookSecret fails if content carries the secret hash of row
// or, when set, the plaintext secret.
func requireNoWebhookSecret(t *testing.T, content string, row database.ChatAutomation, secret string) {
	t.Helper()
	require.NotEmpty(t, row.WebhookSecretHash)
	require.NotContains(t, content, hex.EncodeToString(row.WebhookSecretHash))
	require.NotContains(t, content, base64.StdEncoding.EncodeToString(row.WebhookSecretHash))
	if secret != "" {
		require.NotContains(t, content, secret)
	}
}

const errNotContained = "managed by this tool must"

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
		schedule := f.newChat(ctx, t, f.server, "0 9 * * *", "UTC")

		require.ElementsMatch(t, []uuid.UUID{webhook.ID, schedule.ID}, f.listIDs(ctx, t, f.chat.ID))
		content, isError := f.call(ctx, t, f.chat.ID, "list", uuid.Nil)
		require.False(t, isError, content)
		requireNoWebhookSecret(t, content, webhook, secret)
		require.NotContains(t, content, webhook.Prompt, "list leaves prompts out")

		content, isError = f.call(ctx, t, f.chat.ID, "get", webhook.ID)
		require.False(t, isError, content)
		require.Contains(t, content, webhook.ID.String())
		requireNoWebhookSecret(t, content, webhook, secret)

		content, isError = f.call(ctx, t, f.chat.ID, "disable", webhook.ID)
		require.False(t, isError, content)
		requireNoWebhookSecret(t, content, webhook, secret)
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
			for _, action := range []string{"get", "update", "enable", "disable", "delete", "run_now"} {
				args := manageAutomationsArgs{Action: action, AutomationID: automation.ID.String()}
				if action == "update" {
					args.Prompt = ptr.Ref("Changed.")
				}
				content, isError := f.callArgs(ctx, t, f.chat.ID, args)
				require.True(t, isError)
				require.Equal(t, "automation not found", content)
			}
			f.requireUnchanged(ctx, t, automation)
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

		// Write actions are refused before any other work, even for an
		// automation this turn sees and that targets this chat.
		inputs := f.inputs(ctx, t)
		for _, args := range []manageAutomationsArgs{
			heartbeatArgs(),
			{Action: "update", AutomationID: targetsChat.ID.String(), Prompt: ptr.Ref("Changed.")},
			{Action: "enable", AutomationID: targetsChat.ID.String()},
			{Action: "run_now", AutomationID: targetsChat.ID.String()},
		} {
			content, isError := f.callArgs(ctx, t, f.chat.ID, args)
			require.True(t, isError, content)
			require.Contains(t, content, "not available in a turn that an automation started")
		}
		f.requireUnchanged(ctx, t, targetsChat)
		require.Len(t, f.ownerAutomations(ctx, t), 3)
		require.Equal(t, inputs, f.inputs(ctx, t))
		require.Empty(t, f.auditor.AuditLogs())

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

		// Only the triggering automation can be deleted.
		content, isError = f.call(ctx, t, f.chat.ID, "delete", createdChat.ID)
		require.True(t, isError)
		require.Contains(t, content, "delete only removes that automation")
		_, err = f.db.GetChatAutomationByID(ctx, createdChat.ID)
		require.NoError(t, err)
		content, isError = f.call(ctx, t, f.chat.ID, "delete", targetsChat.ID)
		require.False(t, isError, content)
		_, err = f.db.GetChatAutomationByID(ctx, targetsChat.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)
		require.Len(t, f.auditor.AuditLogs(), 2)
	})

	t.Run("CreateContainment", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		other := f.otherChat(t, f.model.ID)
		search := f.modelConfig(t, true)
		plain := f.modelConfig(t, false)
		newChatArgs := func(modelID *uuid.UUID) manageAutomationsArgs {
			args := heartbeatArgs()
			args.TargetMode = ptr.Ref("new_chat")
			if modelID != nil {
				args.NewChatModelConfigID = ptr.Ref(modelID.String())
			}
			return args
		}

		// A heartbeat: an existing_chat automation without a target targets
		// this chat.
		heartbeat, _ := f.mustCreate(ctx, t, f.chat.ID, heartbeatArgs())
		require.Equal(t, uuid.NullUUID{UUID: f.chat.ID, Valid: true}, heartbeat.TargetChatID)
		require.Equal(t, uuid.NullUUID{UUID: f.chat.ID, Valid: true}, heartbeat.CreatedByChatID)
		require.Equal(t, f.owner.ID, heartbeat.OwnerID)
		require.Equal(t, f.org.ID, heartbeat.OrganizationID)
		require.Contains(t, f.listIDs(ctx, t, f.chat.ID), heartbeat.ID)

		defaulted, _ := f.mustCreate(ctx, t, f.chat.ID, newChatArgs(nil))
		require.Equal(t, uuid.NullUUID{UUID: f.model.ID, Valid: true}, defaulted.NewChatModelConfigID)
		withoutTools, _ := f.mustCreate(ctx, t, f.chat.ID, newChatArgs(&plain.ID))
		require.Equal(t, uuid.NullUUID{UUID: plain.ID, Valid: true}, withoutTools.NewChatModelConfigID)
		// A chat that already has web search may give it to new chats.
		searchChat := f.otherChat(t, search.ID)
		f.setSwitch(t, searchChat.ID, true)
		sameConfig, _ := f.mustCreate(ctx, t, searchChat.ID, newChatArgs(&search.ID))
		require.Equal(t, uuid.NullUUID{UUID: search.ID, Valid: true}, sameConfig.NewChatModelConfigID)

		elsewhere := heartbeatArgs()
		elsewhere.TargetChatID = ptr.Ref(other.ID.String())
		for _, args := range []manageAutomationsArgs{elsewhere, newChatArgs(&search.ID)} {
			content, isError := f.callArgs(ctx, t, f.chat.ID, args)
			require.True(t, isError, content)
			require.Contains(t, content, errNotContained)
		}
		require.Len(t, f.ownerAutomations(ctx, t), 4)

		logs := f.auditor.AuditLogs()
		require.Len(t, logs, 4)
		for i, want := range []struct {
			automation database.ChatAutomation
			chatID     uuid.UUID
		}{{heartbeat, f.chat.ID}, {defaulted, f.chat.ID}, {withoutTools, f.chat.ID}, {sameConfig, searchChat.ID}} {
			require.Equal(t, database.AuditActionCreate, logs[i].Action)
			require.Equal(t, want.automation.ID, logs[i].ResourceID)
			requireAuditFields(t, logs[i], map[string]string{"chat_id": want.chatID.String()})
		}
	})

	t.Run("UpdateAndEnableContainment", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		other := f.otherChat(t, f.model.ID)
		search := f.modelConfig(t, true)
		heartbeat, _ := f.mustCreate(ctx, t, f.chat.ID, heartbeatArgs())
		// Created through the management API, so it already targets
		// another chat.
		elsewhere := f.create(ctx, t, f.server, codersdk.CreateChatAutomationRequest{
			TargetMode:   codersdk.ChatAutomationTargetModeExistingChat,
			TargetChatID: &other.ID,
		}, "0 9 * * *", "UTC")
		newChat := f.newChat(ctx, t, f.server, "0 9 * * *", "UTC")
		f.auditor.ResetLogs()

		for _, tc := range []struct {
			name string
			args manageAutomationsArgs
			row  database.ChatAutomation
			want string
		}{
			{"StoredTargetElsewhere", manageAutomationsArgs{AutomationID: elsewhere.ID.String(), Prompt: ptr.Ref("Changed.")}, elsewhere, errNotContained},
			{"StoredTargetMovedHere", manageAutomationsArgs{AutomationID: elsewhere.ID.String(), TargetChatID: ptr.Ref(f.chat.ID.String())}, elsewhere, errNotContained},
			{"TargetMovesElsewhere", manageAutomationsArgs{AutomationID: heartbeat.ID.String(), TargetChatID: ptr.Ref(other.ID.String())}, heartbeat, errNotContained},
			{"ModelGainsProviderTools", manageAutomationsArgs{AutomationID: newChat.ID.String(), NewChatModelConfigID: ptr.Ref(search.ID.String())}, newChat, errNotContained},
			{"KindIsFixed", manageAutomationsArgs{AutomationID: heartbeat.ID.String(), Kind: ptr.Ref("webhook")}, heartbeat, "cannot be changed"},
		} {
			tc.args.Action = "update"
			content, isError := f.callArgs(ctx, t, f.chat.ID, tc.args)
			require.True(t, isError, "%s: %s", tc.name, content)
			require.Contains(t, content, tc.want, tc.name)
			f.requireUnchanged(ctx, t, tc.row)
		}
		require.Empty(t, f.auditor.AuditLogs())

		content, isError := f.callArgs(ctx, t, f.chat.ID, manageAutomationsArgs{
			Action: "update", AutomationID: heartbeat.ID.String(), Prompt: ptr.Ref("Check the deploy."),
		})
		require.False(t, isError, content)
		updated, err := f.db.GetChatAutomationByID(ctx, heartbeat.ID)
		require.NoError(t, err)
		require.Equal(t, "Check the deploy.", updated.Prompt)

		// enable follows the same rules.
		for _, row := range []database.ChatAutomation{heartbeat, elsewhere} {
			_, _, err := f.server.UpdateAutomation(f.asOwner(ctx, t), f.owner.ID, row.ID, codersdk.UpdateChatAutomationRequest{Enabled: ptr.Ref(false)}, nil)
			require.NoError(t, err)
		}
		content, isError = f.call(ctx, t, f.chat.ID, "enable", elsewhere.ID)
		require.True(t, isError)
		require.Contains(t, content, errNotContained)
		row, err := f.db.GetChatAutomationByID(ctx, elsewhere.ID)
		require.NoError(t, err)
		require.False(t, row.Enabled)
		content, isError = f.call(ctx, t, f.chat.ID, "enable", heartbeat.ID)
		require.False(t, isError, content)
		row, err = f.db.GetChatAutomationByID(ctx, heartbeat.ID)
		require.NoError(t, err)
		require.True(t, row.Enabled)

		logs := f.auditor.AuditLogs()
		require.Len(t, logs, 2)
		for _, log := range logs {
			require.Equal(t, database.AuditActionWrite, log.Action)
			require.Equal(t, heartbeat.ID, log.ResourceID)
			requireAuditFields(t, log, map[string]string{"chat_id": f.chat.ID.String()})
		}
	})

	t.Run("RunNowContainment", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		other := f.otherChat(t, f.model.ID)
		search := f.modelConfig(t, true)
		heartbeat := f.existingChat(ctx, t, f.server, "0 9 * * *", "UTC", codersdk.ChatAutomationWhenBusySkip)
		elsewhere := f.create(ctx, t, f.server, codersdk.CreateChatAutomationRequest{
			TargetMode:   codersdk.ChatAutomationTargetModeExistingChat,
			TargetChatID: &other.ID,
		}, "0 9 * * *", "UTC")
		withSearch := f.create(ctx, t, f.server, codersdk.CreateChatAutomationRequest{
			TargetMode:           codersdk.ChatAutomationTargetModeNewChat,
			NewChatModelConfigID: &search.ID,
		}, "0 9 * * *", "UTC")
		contained := f.newChat(ctx, t, f.server, "0 9 * * *", "UTC")
		chats := len(f.createdChats(ctx, t))

		for _, automation := range []database.ChatAutomation{elsewhere, withSearch} {
			content, isError := f.call(ctx, t, f.chat.ID, "run_now", automation.ID)
			require.True(t, isError, content)
			require.Contains(t, content, errNotContained)
		}
		var otherMessages int
		require.NoError(t, f.sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM chat_messages WHERE chat_id = $1", other.ID).Scan(&otherMessages))
		require.Zero(t, otherMessages)
		require.Len(t, f.createdChats(ctx, t), chats)

		var result struct {
			InputID uuid.UUID `json:"input_id"`
			ChatID  uuid.UUID `json:"chat_id"`
		}
		content, isError := f.call(ctx, t, f.chat.ID, "run_now", contained.ID)
		require.False(t, isError, content)
		require.NoError(t, json.Unmarshal([]byte(content), &result))
		require.Len(t, f.createdChats(ctx, t), chats+1)
		// Like the run endpoint, only the created chat is audited, and the
		// entry also names the calling chat.
		logs := f.auditor.AuditLogs()
		require.Len(t, logs, 1)
		require.Equal(t, database.AuditActionCreate, logs[0].Action)
		require.Equal(t, database.ResourceTypeChat, logs[0].ResourceType)
		require.Equal(t, result.ChatID, logs[0].ResourceID)
		requireAuditFields(t, logs[0], map[string]string{
			"automation_id":      contained.ID.String(),
			"input_id":           result.InputID.String(),
			"created_by_chat_id": f.chat.ID.String(),
		})

		// Last: the heartbeat's message makes later turns of this chat
		// automation-reached.
		content, isError = f.call(ctx, t, f.chat.ID, "run_now", heartbeat.ID)
		require.False(t, isError, content)
		require.NoError(t, json.Unmarshal([]byte(content), &result))
		require.Equal(t, f.chat.ID, result.ChatID)
		require.NotEqual(t, uuid.Nil, result.InputID)
		require.Equal(t, 1, f.inputs(ctx, t))
		require.Len(t, f.auditor.AuditLogs(), 1, "a run to an existing chat changes no configuration")
	})

	t.Run("RejectsFieldsThatDoNotApply", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		heartbeat := f.existingChat(ctx, t, f.server, "0 9 * * *", "UTC", codersdk.ChatAutomationWhenBusySkip)

		// Each call would succeed without the check, so a silently
		// dropped field would go unnoticed.
		createWithID := heartbeatArgs()
		createWithID.AutomationID = heartbeat.ID.String()
		for _, tc := range []struct {
			args manageAutomationsArgs
			want string
		}{
			{createWithID, "create does not take automation_id"},
			{manageAutomationsArgs{Action: "update", AutomationID: heartbeat.ID.String()}, "update needs at least one field to change"},
			{manageAutomationsArgs{Action: "enable", AutomationID: heartbeat.ID.String(), Prompt: ptr.Ref("Changed.")}, "enable does not take prompt"},
		} {
			content, isError := f.callArgs(ctx, t, f.chat.ID, tc.args)
			require.True(t, isError, content)
			require.Equal(t, tc.want, content)
		}
		f.requireUnchanged(ctx, t, heartbeat)
		require.Len(t, f.ownerAutomations(ctx, t), 1)
		require.Empty(t, f.auditor.AuditLogs())
	})

	t.Run("WebhookSecrets", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		webhookArgs := func(use *string, mode string) manageAutomationsArgs {
			return manageAutomationsArgs{
				Action: "create", Name: ptr.Ref("Deploy hook"), Kind: ptr.Ref("webhook"),
				TargetMode: ptr.Ref(mode), WebhookUse: use, Prompt: ptr.Ref("A deploy finished."),
			}
		}
		// Every secret starts with the prefix, so a result without it
		// carries no plaintext secret.
		requireNoPlaintextSecret := func(t *testing.T, content string) {
			t.Helper()
			require.NotContains(t, content, automationWebhookSecretPrefix)
		}

		// No create returns a secret, not even a single-use webhook on this
		// chat in a human turn: the result stays in the chat, where shared
		// readers can see it.
		var rows []database.ChatAutomation
		for _, args := range []manageAutomationsArgs{
			webhookArgs(ptr.Ref("single"), "existing_chat"),
			webhookArgs(ptr.Ref("multi"), "existing_chat"),
			webhookArgs(nil, "existing_chat"),
			webhookArgs(ptr.Ref("single"), "new_chat"),
		} {
			row, content := f.mustCreate(ctx, t, f.chat.ID, args)
			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(content), &result))
			require.NotContains(t, result, "webhook_secret")
			require.Equal(t, manageAutomationsSecretNotShown, result["webhook_secret_note"])
			requireNoPlaintextSecret(t, content)
			requireNoWebhookSecret(t, content, row, "")
			rows = append(rows, row)
		}

		single := rows[0]
		for _, args := range []manageAutomationsArgs{
			{Action: "get", AutomationID: single.ID.String()},
			{Action: "update", AutomationID: single.ID.String(), Prompt: ptr.Ref("Changed.")},
			{Action: "disable", AutomationID: single.ID.String()},
			{Action: "enable", AutomationID: single.ID.String()},
		} {
			content, isError := f.callArgs(ctx, t, f.chat.ID, args)
			require.False(t, isError, content)
			requireNoPlaintextSecret(t, content)
			requireNoWebhookSecret(t, content, single, "")
		}
		content, isError := f.call(ctx, t, f.chat.ID, "list", uuid.Nil)
		require.False(t, isError, content)
		requireNoPlaintextSecret(t, content)
		for _, row := range rows {
			requireNoWebhookSecret(t, content, row, "")
		}
	})
}

// automationReadHook runs a hook once, right after the first read of the
// armed automation through GetChatAutomationByID, so a test can change the
// automation between a caller's read and the service's locked reread.
type automationReadHook struct {
	database.Store
	mu    sync.Mutex
	id    uuid.UUID
	hook  func()
	fired bool
}

func (s *automationReadHook) arm(id uuid.UUID, hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id, s.hook, s.fired = id, hook, false
}

func (s *automationReadHook) GetChatAutomationByID(ctx context.Context, id uuid.UUID) (database.ChatAutomation, error) {
	row, err := s.Store.GetChatAutomationByID(ctx, id)
	s.mu.Lock()
	var hook func()
	if id == s.id && !s.fired {
		hook, s.fired = s.hook, true
	}
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	return row, err
}

func (s *automationReadHook) requireFired(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.True(t, s.fired, "the automation was never read")
}

// TestManageAutomationsToolConcurrentRetarget retargets the automation, as
// the owner through the service, after the tool has read and checked it
// and before the service locks it. The tool must refuse with its
// containment error and write, send, create, and audit nothing.
func TestManageAutomationsToolConcurrentRetarget(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		action       string
		existingChat bool
		disabled     bool
		// targetHere makes the update set the target to the calling chat,
		// which the stored row must still refuse.
		targetHere bool
	}{
		{name: "UpdateExistingChat", action: "update", existingChat: true},
		{name: "UpdateTargetBackHere", action: "update", existingChat: true, targetHere: true},
		{name: "EnableNewChat", action: "enable", disabled: true},
		{name: "RunNowExistingChat", action: "run_now", existingChat: true},
		{name: "RunNowNewChat", action: "run_now"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sf := newScheduleFixture(t, database.ChatStatusWaiting, time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC))
			sf.setSwitch(t, sf.chat.ID, true)
			reads := &automationReadHook{}
			f := manageAutomationsFixture{scheduleFixture: sf, server: sf.newServerWithStore(t, Limits{}, func(store database.Store) database.Store {
				reads.Store = store
				return reads
			})}
			ctx := testutil.Context(t, testutil.WaitLong)
			ownerCtx := f.asOwner(ctx, t)

			var (
				row      database.ChatAutomation
				retarget codersdk.UpdateChatAutomationRequest
			)
			if tc.existingChat {
				row = f.existingChat(ctx, t, f.server, "0 9 * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
				other := f.otherChat(t, f.model.ID)
				retarget.TargetChatID = &other.ID
			} else {
				row = f.newChat(ctx, t, f.server, "0 9 * * *", "UTC")
				search := f.modelConfig(t, true)
				retarget.NewChatModelConfigID = &search.ID
			}
			if tc.disabled {
				var err error
				_, row, err = f.server.UpdateAutomation(ownerCtx, f.owner.ID, row.ID, codersdk.UpdateChatAutomationRequest{Enabled: ptr.Ref(false)}, nil)
				require.NoError(t, err)
			}

			var (
				retargeted  database.ChatAutomation
				retargetErr error
			)
			reads.arm(row.ID, func() {
				_, retargeted, retargetErr = f.server.UpdateAutomation(ownerCtx, f.owner.ID, row.ID, retarget, nil)
			})
			chats := len(f.createdChats(ctx, t))
			inputs := f.ownerInputs(ctx, t)
			f.auditor.ResetLogs()

			args := manageAutomationsArgs{Action: tc.action, AutomationID: row.ID.String()}
			switch {
			case tc.targetHere:
				args.TargetChatID = ptr.Ref(f.chat.ID.String())
			case tc.action == "update":
				args.Prompt = ptr.Ref("Changed.")
			}
			content, isError := f.callArgs(ctx, t, f.chat.ID, args)
			reads.requireFired(t)
			require.NoError(t, retargetErr)
			require.True(t, isError, content)
			require.Contains(t, content, errNotContained)
			f.requireUnchanged(ctx, t, retargeted)
			require.Len(t, f.createdChats(ctx, t), chats)
			require.Equal(t, inputs, f.ownerInputs(ctx, t))
			require.Empty(t, f.auditor.AuditLogs())
		})
	}
}

// TestManageAutomationsToolAuditsLockedRow renames the automation, as the
// owner through the service, after the tool has read it and before the
// service locks it. The tool's change still succeeds, and its audit entry
// must diff against the row the service locked, so the rename, which
// belongs to the owner's own change, is not attributed to the tool.
func TestManageAutomationsToolAuditsLockedRow(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		action   string
		disabled bool
		// changed is the field the tool's change must show in the diff.
		changed string
	}{
		{action: "update", changed: "prompt"},
		{action: "enable", disabled: true, changed: "enabled"},
		{action: "disable", changed: "enabled"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			t.Parallel()
			sf := newScheduleFixture(t, database.ChatStatusWaiting, time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC))
			sf.setSwitch(t, sf.chat.ID, true)
			// The default mock records an empty diff, so record the fields
			// this test compares.
			sf.auditor = audit.NewMockWithDiffFn(func(old, newVal any) audit.Map {
				oldRow, oldOK := old.(database.ChatAutomation)
				newRow, newOK := newVal.(database.ChatAutomation)
				if !oldOK || !newOK {
					return audit.Map{}
				}
				diff := audit.Map{}
				if oldRow.Name != newRow.Name {
					diff["name"] = audit.OldNew{Old: oldRow.Name, New: newRow.Name}
				}
				if oldRow.Prompt != newRow.Prompt {
					diff["prompt"] = audit.OldNew{Old: "", New: "", Secret: true}
				}
				if oldRow.Enabled != newRow.Enabled {
					diff["enabled"] = audit.OldNew{Old: oldRow.Enabled, New: newRow.Enabled}
				}
				return diff
			})
			reads := &automationReadHook{}
			f := manageAutomationsFixture{scheduleFixture: sf, server: sf.newServerWithStore(t, Limits{}, func(store database.Store) database.Store {
				reads.Store = store
				return reads
			})}
			ctx := testutil.Context(t, testutil.WaitLong)
			ownerCtx := f.asOwner(ctx, t)

			row := f.existingChat(ctx, t, f.server, "0 9 * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
			if tc.disabled {
				var err error
				_, row, err = f.server.UpdateAutomation(ownerCtx, f.owner.ID, row.ID, codersdk.UpdateChatAutomationRequest{Enabled: ptr.Ref(false)}, nil)
				require.NoError(t, err)
			}
			var renameErr error
			reads.arm(row.ID, func() {
				_, _, renameErr = f.server.UpdateAutomation(ownerCtx, f.owner.ID, row.ID, codersdk.UpdateChatAutomationRequest{Name: ptr.Ref("Renamed")}, nil)
			})
			f.auditor.ResetLogs()

			args := manageAutomationsArgs{Action: tc.action, AutomationID: row.ID.String()}
			if tc.action == "update" {
				args.Prompt = ptr.Ref("Changed.")
			}
			content, isError := f.callArgs(ctx, t, f.chat.ID, args)
			reads.requireFired(t)
			require.NoError(t, renameErr)
			require.False(t, isError, content)

			logs := f.auditor.AuditLogs()
			require.Len(t, logs, 1)
			var diff audit.Map
			require.NoError(t, json.Unmarshal(logs[0].Diff, &diff))
			require.Contains(t, diff, tc.changed)
			require.NotContains(t, diff, "name", "the rename happened before the lock")
		})
	}
}

// ownerInputs counts the messages and queued messages in all chats of the
// fixture owner.
func (f manageAutomationsFixture) ownerInputs(ctx context.Context, t *testing.T) int {
	t.Helper()
	var count int
	err := f.sqlDB.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM chat_messages m JOIN chats c ON c.id = m.chat_id WHERE c.owner_id = $1) +
		(SELECT COUNT(*) FROM chat_queued_messages q JOIN chats c ON c.id = q.chat_id WHERE c.owner_id = $1)`, f.owner.ID).Scan(&count)
	require.NoError(t, err)
	return count
}

// TestAutomationGuard covers where the services run a caller's guard:
// UpdateAutomation on the row as the update would leave it, and
// RunAutomation at admission on the locked row, after the automation
// changed since the read before the send.
func TestAutomationGuard(t *testing.T) {
	t.Parallel()
	errRefused := xerrors.New("refused by guard")

	t.Run("UpdateChecksResult", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		ownerCtx := f.asOwner(ctx, t)
		other := f.otherChat(t, f.model.ID)
		row := f.existingChat(ctx, t, f.server, "0 9 * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)

		var seen []uuid.NullUUID
		_, _, err := f.server.UpdateAutomation(ownerCtx, f.owner.ID, row.ID, codersdk.UpdateChatAutomationRequest{TargetChatID: &other.ID},
			func(_ database.Store, automation database.ChatAutomation) error {
				seen = append(seen, automation.TargetChatID)
				if automation.TargetChatID.UUID == other.ID {
					return errRefused
				}
				return nil
			})
		require.ErrorIs(t, err, errRefused)
		require.Equal(t, []uuid.NullUUID{row.TargetChatID, {UUID: other.ID, Valid: true}}, seen)
		f.requireUnchanged(ctx, t, row)
	})

	t.Run("RunChecksLockedRowAtAdmission", func(t *testing.T) {
		t.Parallel()
		f := newManageAutomationsFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		ownerCtx := f.asOwner(ctx, t)
		other := f.otherChat(t, f.model.ID)
		row := f.existingChat(ctx, t, f.server, "0 9 * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		inputs := f.ownerInputs(ctx, t)

		var seen []uuid.NullUUID
		_, err := f.server.RunAutomation(ownerCtx, f.owner.ID, row.ID, func(_ database.Store, automation database.ChatAutomation) error {
			seen = append(seen, automation.TargetChatID)
			if len(seen) == 1 {
				// Retarget after the read before the send.
				_, _, err := f.server.UpdateAutomation(ownerCtx, f.owner.ID, row.ID, codersdk.UpdateChatAutomationRequest{TargetChatID: &other.ID}, nil)
				return err
			}
			return errRefused
		})
		require.ErrorIs(t, err, errRefused)
		require.Equal(t, []uuid.NullUUID{row.TargetChatID, {UUID: other.ID, Valid: true}}, seen)
		require.Equal(t, inputs, f.ownerInputs(ctx, t))
	})
}
