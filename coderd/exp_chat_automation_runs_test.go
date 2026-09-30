package coderd_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// existingChatScheduleRequest returns a schedule automation that sends to
// chatID.
func (e chatAutomationTestEnv) existingChatScheduleRequest(chatID uuid.UUID, whenBusy codersdk.ChatAutomationWhenBusy) codersdk.CreateChatAutomationRequest {
	req := e.scheduleRequest()
	req.TargetMode = codersdk.ChatAutomationTargetModeExistingChat
	req.TargetChatID = &chatID
	req.NewChatModelConfigID = nil
	req.WhenBusy = ptr.Ref(whenBusy)
	return req
}

// userMessages returns the user messages saved in chatID.
func (e chatAutomationTestEnv) userMessages(t *testing.T, chatID uuid.UUID) []database.ChatMessage {
	t.Helper()
	messages, err := e.db.GetChatMessagesByChatID(dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitShort)), database.GetChatMessagesByChatIDParams{ChatID: chatID})
	require.NoError(t, err)
	var users []database.ChatMessage
	for _, message := range messages {
		if message.Role == database.ChatMessageRoleUser {
			users = append(users, message)
		}
	}
	return users
}

func TestChatAutomationRuns(t *testing.T) {
	t.Parallel()

	t.Run("ExistingChatKeepsSchedule", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.existingChatScheduleRequest(env.memberChat.ID, codersdk.ChatAutomationWhenBusyQueue))
		require.NoError(t, err)
		id := created.Automation.ID
		before, err := env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), id)
		require.NoError(t, err)

		res, err := env.member.RunChatAutomation(ctx, env.orgID, id)
		require.NoError(t, err)
		require.Equal(t, env.memberChat.ID, res.ChatID)

		messages := env.userMessages(t, env.memberChat.ID)
		require.Len(t, messages, 1)
		require.Equal(t, uuid.NullUUID{UUID: id, Valid: true}, messages[0].AutomationID)
		require.Equal(t, uuid.NullUUID{UUID: res.InputID, Valid: true}, messages[0].InputID)
		sdkMessages, err := env.member.GetChatMessages(ctx, env.memberChat.ID, nil)
		require.NoError(t, err)
		require.Len(t, sdkMessages.Messages, 1)
		require.Len(t, sdkMessages.Messages[0].Content, 1, "only the saved prompt is sent")
		require.Equal(t, "Summarize yesterday.", sdkMessages.Messages[0].Content[0].Text)

		after, err := env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(ctx), id)
		require.NoError(t, err)
		require.Equal(t, before, after, "running now must not write the automation, so the schedule cursor and revision stay")
	})

	t.Run("OnlyOwnerCanRun", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationTestEnv(t, nil, nil)
		ctx := testutil.Context(t, testutil.WaitLong)
		orgAdminRaw, _ := coderdtest.CreateAnotherUser(t, env.owner.Client, env.orgID, rbac.ScopedRoleOrgAdmin(env.orgID))
		otherRaw, _ := coderdtest.CreateAnotherUser(t, env.owner.Client, env.orgID)
		created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.existingChatScheduleRequest(env.memberChat.ID, codersdk.ChatAutomationWhenBusyQueue))
		require.NoError(t, err)
		webhook, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)

		for name, client := range map[string]*codersdk.ExperimentalClient{
			"OrgAdmin":  codersdk.NewExperimentalClient(orgAdminRaw),
			"SiteOwner": env.owner,
		} {
			_, err := client.RunChatAutomation(ctx, env.orgID, created.Automation.ID)
			sdkErr := requireSDKError(t, err, http.StatusForbidden)
			require.Equal(t, "Only the owner of a chat automation can run it.", sdkErr.Message, name)
			// The owner check comes first, so the kind of an automation
			// someone else owns is not revealed.
			_, err = client.RunChatAutomation(ctx, env.orgID, webhook.Automation.ID)
			requireSDKError(t, err, http.StatusForbidden)
		}
		_, err = codersdk.NewExperimentalClient(otherRaw).RunChatAutomation(ctx, env.orgID, created.Automation.ID)
		requireSDKError(t, err, http.StatusNotFound)
		require.Empty(t, env.userMessages(t, env.memberChat.ID))
		require.Empty(t, env.queuedMessageIDs(t, env.memberChat.ID))
	})

	t.Run("Refused", func(t *testing.T) {
		t.Parallel()
		// A queue of 2 leaves automations a share of 1.
		env := newChatAutomationTestEnv(t, nil, func(o *coderdtest.Options) {
			o.DeploymentValues.AI.Chat.MaxQueuedMessagesPerChat = serpent.Int64(2)
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		busy := dbgen.Chat(t, env.db, database.Chat{
			OrganizationID:    env.orgID,
			OwnerID:           env.memberID,
			LastModelConfigID: env.modelConfig.ID,
			Status:            database.ChatStatusRunning,
		})

		webhook, err := env.member.CreateChatAutomation(ctx, env.orgID, env.webhookRequest())
		require.NoError(t, err)
		_, err = env.member.RunChatAutomation(ctx, env.orgID, webhook.Automation.ID)
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Len(t, sdkErr.Validations, 1, sdkErr.Error())
		require.Equal(t, "kind", sdkErr.Validations[0].Field)

		disabled, err := env.member.CreateChatAutomation(ctx, env.orgID, env.existingChatScheduleRequest(env.memberChat.ID, codersdk.ChatAutomationWhenBusyQueue))
		require.NoError(t, err)
		_, err = env.member.UpdateChatAutomation(ctx, env.orgID, disabled.Automation.ID, codersdk.UpdateChatAutomationRequest{Enabled: ptr.Ref(false)})
		require.NoError(t, err)
		_, err = env.member.RunChatAutomation(ctx, env.orgID, disabled.Automation.ID)
		requireSDKError(t, err, http.StatusConflict)
		require.Empty(t, env.userMessages(t, env.memberChat.ID))

		skip, err := env.member.CreateChatAutomation(ctx, env.orgID, env.existingChatScheduleRequest(busy.ID, codersdk.ChatAutomationWhenBusySkip))
		require.NoError(t, err)
		_, err = env.member.RunChatAutomation(ctx, env.orgID, skip.Automation.ID)
		requireSDKError(t, err, http.StatusConflict)
		require.Empty(t, env.queuedMessageIDs(t, busy.ID))

		queue, err := env.member.CreateChatAutomation(ctx, env.orgID, env.existingChatScheduleRequest(busy.ID, codersdk.ChatAutomationWhenBusyQueue))
		require.NoError(t, err)
		_, err = env.member.RunChatAutomation(ctx, env.orgID, queue.Automation.ID)
		require.NoError(t, err)
		_, err = env.member.RunChatAutomation(ctx, env.orgID, queue.Automation.ID)
		sdkErr = requireSDKError(t, err, http.StatusTooManyRequests)
		require.Equal(t, "At most 1 automation messages can be queued in a chat.", sdkErr.Detail)
		require.Len(t, env.queuedMessageIDs(t, busy.ID), 1)
	})

	t.Run("NewChatTarget", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		env := newChatAutomationTestEnv(t, nil, func(o *coderdtest.Options) { o.Auditor = auditor })
		ctx := testutil.Context(t, testutil.WaitLong)
		created, err := env.member.CreateChatAutomation(ctx, env.orgID, env.scheduleRequest())
		require.NoError(t, err)

		start := time.Now().Truncate(time.Minute)
		res, err := env.member.RunChatAutomation(ctx, env.orgID, created.Automation.ID)
		require.NoError(t, err)
		require.NotEqual(t, env.memberChat.ID, res.ChatID)

		chat, err := env.member.GetChat(ctx, res.ChatID)
		require.NoError(t, err)
		require.Equal(t, env.memberID, chat.OwnerID)
		require.True(t, strings.HasPrefix(chat.Title, "Nightly report "), chat.Title)
		berlin, err := time.LoadLocation("Europe/Berlin")
		require.NoError(t, err)
		titled, err := time.ParseInLocation("2006-01-02 15:04 MST", strings.TrimPrefix(chat.Title, "Nightly report "), berlin)
		require.NoError(t, err, "the title ends with the run time in the schedule's time zone")
		require.Contains(t, []string{"CET", "CEST"}, titled.Format("MST"))
		require.False(t, titled.Before(start), "title time %s", titled)
		require.False(t, titled.After(time.Now()), "title time %s", titled)
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:         database.AuditActionCreate,
			ResourceType:   database.ResourceTypeChat,
			ResourceID:     res.ChatID,
			UserID:         env.memberID,
			OrganizationID: env.orgID,
		}), "the created chat is audited as created by the automation owner")

		messages := env.userMessages(t, res.ChatID)
		require.Len(t, messages, 1)
		require.Equal(t, uuid.NullUUID{UUID: res.InputID, Valid: true}, messages[0].InputID)
	})
}
