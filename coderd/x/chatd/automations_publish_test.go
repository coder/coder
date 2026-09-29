package chatd_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// publishFixture is a chat owned by an agents-access member, with a
// server that publishes to it.
type publishFixture struct {
	db     database.Store
	sqlDB  *sql.DB
	server *chatd.Server
	owner  database.User
	org    database.Organization
	model  database.ChatModelConfig
	chat   database.Chat
}

func newPublishFixture(t *testing.T, status database.ChatStatus, overrides ...func(*chatd.Config)) publishFixture {
	t.Helper()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	server := newTestServer(t, db, ps, uuid.New(), overrides...)
	owner, org, model := seedChatDependencies(t, db)
	_, err := db.UpdateMemberRoles(testutil.Context(t, testutil.WaitShort), database.UpdateMemberRolesParams{
		GrantedRoles: []string{rbac.RoleAgentsAccess()},
		UserID:       owner.ID,
		OrgID:        org.ID,
	})
	require.NoError(t, err)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: model.ID,
		Title:             "automation target",
		Status:            status,
	})
	return publishFixture{db: db, sqlDB: sqlDB, server: server, owner: owner, org: org, model: model, chat: chat}
}

func (f publishFixture) webhook(ctx context.Context, t *testing.T, use codersdk.ChatAutomationWebhookUse, whenBusy codersdk.ChatAutomationWhenBusy) database.ChatAutomation {
	t.Helper()
	automation, _, err := f.server.CreateAutomation(ctx, chatd.CreateAutomationParams{
		OrganizationID: f.org.ID,
		OwnerID:        f.owner.ID,
		Request: codersdk.CreateChatAutomationRequest{
			Name:         "Deploy hook",
			Kind:         codersdk.ChatAutomationKindWebhook,
			TargetMode:   codersdk.ChatAutomationTargetModeExistingChat,
			TargetChatID: &f.chat.ID,
			Prompt:       "A deploy finished.",
			WebhookUse:   &use,
			WhenBusy:     &whenBusy,
		},
	})
	require.NoError(t, err)
	return automation
}

func (f publishFixture) publish(ctx context.Context, automation database.ChatAutomation) (chatd.PublishAutomationResult, error) {
	return f.server.PublishAutomationWebhook(ctx, chatd.PublishAutomationWebhookParams{
		AutomationID:  automation.ID,
		SecretVersion: automation.WebhookSecretVersion,
		Body:          []byte(`{"service":"api"}`),
	})
}

// requireNothingSaved checks that the chat has no messages and no queued
// messages.
func (f publishFixture) requireNothingSaved(ctx context.Context, t *testing.T) {
	t.Helper()
	require.Empty(t, chatMessages(ctx, t, f.db, f.chat.ID))
	queued, err := f.db.GetChatQueuedMessages(ctx, f.chat.ID)
	require.NoError(t, err)
	require.Empty(t, queued)
}

func TestPublishAutomationWebhook(t *testing.T) {
	t.Parallel()

	t.Run("IdleChat", func(t *testing.T) {
		t.Parallel()
		f := newPublishFixture(t, database.ChatStatusWaiting)
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti, codersdk.ChatAutomationWhenBusyQueue)

		result, err := f.publish(ctx, automation)
		require.NoError(t, err)
		require.Equal(t, f.chat.ID, result.ChatID)

		messages := chatMessages(ctx, t, f.db, f.chat.ID)
		require.Len(t, messages, 1)
		message := messages[0]
		require.Equal(t, uuid.NullUUID{UUID: automation.ID, Valid: true}, message.AutomationID)
		require.Equal(t, uuid.NullUUID{UUID: result.InputID, Valid: true}, message.InputID)
		require.Equal(t, uuid.NullUUID{UUID: f.owner.ID, Valid: true}, message.CreatedBy)
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		require.Len(t, parts, 2, "the prompt and the event data are separate parts")
		require.Equal(t, codersdk.ChatMessageText("A deploy finished."), parts[0])
		require.Contains(t, parts[1].Text, "<automation_event_data>\n{\"service\":\"api\"}\n</automation_event_data>")

		chat, err := f.db.GetChatByID(ctx, f.chat.ID)
		require.NoError(t, err)
		require.False(t, chat.AutomationID.Valid, "sending to an existing chat does not mark it as created by the automation")
	})

	t.Run("SingleUseConcurrent", func(t *testing.T) {
		t.Parallel()
		f := newPublishFixture(t, database.ChatStatusRunning)
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusyQueue)

		const publishers = 8
		errs := make([]error, publishers)
		var wg sync.WaitGroup
		for i := range publishers {
			wg.Go(func() {
				_, errs[i] = f.publish(ctx, automation)
			})
		}
		wg.Wait()
		accepted := 0
		for _, err := range errs {
			if err == nil {
				accepted++
				continue
			}
			require.ErrorIs(t, err, chatd.ErrAutomationWebhookConsumed)
		}
		require.Equal(t, 1, accepted)
		queued, err := f.db.GetChatQueuedMessages(ctx, f.chat.ID)
		require.NoError(t, err)
		require.Len(t, queued, 1)
		stored, err := f.db.GetChatAutomationByID(ctx, automation.ID)
		require.NoError(t, err)
		require.True(t, stored.WebhookConsumedAt.Valid)
	})

	t.Run("SecretRotatedBeforeCommit", func(t *testing.T) {
		t.Parallel()
		f := newPublishFixture(t, database.ChatStatusWaiting)
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusyQueue)
		_, _, err := f.server.RotateAutomationSecret(ctx, f.owner.ID, automation.ID)
		require.NoError(t, err)

		// automation still carries the version the request was verified
		// against.
		_, err = f.publish(ctx, automation)
		require.ErrorIs(t, err, chatd.ErrAutomationSecretChanged)
		f.requireNothingSaved(ctx, t)
		stored, err := f.db.GetChatAutomationByID(ctx, automation.ID)
		require.NoError(t, err)
		require.False(t, stored.WebhookConsumedAt.Valid)
	})

	t.Run("SkipBusyChat", func(t *testing.T) {
		t.Parallel()
		f := newPublishFixture(t, database.ChatStatusRunning)
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusySkip)

		_, err := f.publish(ctx, automation)
		require.ErrorIs(t, err, chatd.ErrAutomationChatBusy)
		f.requireNothingSaved(ctx, t)
		stored, err := f.db.GetChatAutomationByID(ctx, automation.ID)
		require.NoError(t, err)
		require.False(t, stored.WebhookConsumedAt.Valid, "a skipped delivery does not use up a single-use webhook")
	})

	t.Run("QueueBusyChatShare", func(t *testing.T) {
		t.Parallel()
		// A queue of 4 leaves automations a share of 2.
		f := newPublishFixture(t, database.ChatStatusRunning, func(cfg *chatd.Config) {
			cfg.Limits = chatd.Limits{MaxQueuedMessagesPerChat: 4}
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti, codersdk.ChatAutomationWhenBusyQueue)

		for range 2 {
			result, err := f.publish(ctx, automation)
			require.NoError(t, err)
			require.Equal(t, f.chat.ID, result.ChatID)
		}
		_, err := f.publish(ctx, automation)
		require.ErrorIs(t, err, chatd.ErrAutomationQueueShareFull)

		chat, err := f.db.GetChatByID(ctx, f.chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRunning, chat.Status, "queueing never interrupts the running turn")
		queued, err := f.db.GetChatQueuedMessages(ctx, f.chat.ID)
		require.NoError(t, err)
		require.Len(t, queued, 2)
		for _, row := range queued {
			require.Equal(t, uuid.NullUUID{UUID: automation.ID, Valid: true}, row.AutomationID)
		}

		// People keep the rest of the queue.
		sent, err := f.server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:    f.chat.ID,
			CreatedBy: f.owner.ID,
			Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("from a person")},
		})
		require.NoError(t, err)
		require.True(t, sent.Queued)
	})

	t.Run("RefusedOwner", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name   string
			mutate func(context.Context, *testing.T, publishFixture)
			want   error
		}{
			{"Suspended", func(ctx context.Context, t *testing.T, f publishFixture) {
				setOwnerStatus(ctx, t, f, database.UserStatusSuspended)
			}, chatd.ErrAutomationOwnerInactive},
			{"Dormant", func(ctx context.Context, t *testing.T, f publishFixture) {
				setOwnerStatus(ctx, t, f, database.UserStatusDormant)
			}, chatd.ErrAutomationOwnerInactive},
			{"Deleted", func(ctx context.Context, t *testing.T, f publishFixture) {
				require.NoError(t, f.db.UpdateUserDeletedByID(ctx, f.owner.ID))
			}, chatd.ErrAutomationOwnerInactive},
			{"NoAgentsAccess", func(ctx context.Context, t *testing.T, f publishFixture) {
				_, err := f.db.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
					GrantedRoles: []string{},
					UserID:       f.owner.ID,
					OrgID:        f.org.ID,
				})
				require.NoError(t, err)
				_, err = f.sqlDB.ExecContext(ctx, "UPDATE organizations SET default_org_member_roles = '{}' WHERE id = $1", f.org.ID)
				require.NoError(t, err)
			}, chatd.ErrAutomationForbidden},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newPublishFixture(t, database.ChatStatusWaiting)
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti, codersdk.ChatAutomationWhenBusyQueue)
				tc.mutate(ctx, t, f)

				_, err := f.publish(ctx, automation)
				require.ErrorIs(t, err, tc.want)
				f.requireNothingSaved(ctx, t)
			})
		}
	})

	t.Run("TargetNotRootChatOfOwner", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name   string
			mutate func(context.Context, *testing.T, publishFixture)
		}{
			{"Archived", func(ctx context.Context, t *testing.T, f publishFixture) {
				_, err := f.db.ArchiveChatByID(ctx, f.chat.ID)
				require.NoError(t, err)
			}},
			{"OtherOwner", func(ctx context.Context, t *testing.T, f publishFixture) {
				other := dbgen.User(t, f.db, database.User{})
				_, err := f.sqlDB.ExecContext(ctx, "UPDATE chats SET owner_id = $1 WHERE id = $2", other.ID, f.chat.ID)
				require.NoError(t, err)
			}},
			{"Subagent", func(ctx context.Context, t *testing.T, f publishFixture) {
				parent := dbgen.Chat(t, f.db, database.Chat{
					OrganizationID:    f.org.ID,
					OwnerID:           f.owner.ID,
					LastModelConfigID: f.model.ID,
				})
				_, err := f.sqlDB.ExecContext(ctx, "UPDATE chats SET parent_chat_id = $1, root_chat_id = $1 WHERE id = $2", parent.ID, f.chat.ID)
				require.NoError(t, err)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newPublishFixture(t, database.ChatStatusWaiting)
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti, codersdk.ChatAutomationWhenBusyQueue)
				tc.mutate(ctx, t, f)

				_, err := f.publish(ctx, automation)
				require.ErrorIs(t, err, chatd.ErrAutomationTargetUnavailable)
				f.requireNothingSaved(ctx, t)
			})
		}
	})

	t.Run("HookDenied", func(t *testing.T) {
		t.Parallel()
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"permission":{"decision":"deny"},"user_message":"blocked"}`))
		}))
		t.Cleanup(consumer.Close)
		f := newPublishFixture(t, database.ChatStatusWaiting, func(cfg *chatd.Config) {
			cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusyQueue)

		_, err := f.publish(ctx, automation)
		var denied *chathooks.UserPromptDeniedError
		require.ErrorAs(t, err, &denied)
		f.requireNothingSaved(ctx, t)
		stored, err := f.db.GetChatAutomationByID(ctx, automation.ID)
		require.NoError(t, err)
		require.False(t, stored.WebhookConsumedAt.Valid, "a denied delivery does not use up a single-use webhook")
	})
}

func setOwnerStatus(ctx context.Context, t *testing.T, f publishFixture, status database.UserStatus) {
	t.Helper()
	_, err := f.db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
		ID:        f.owner.ID,
		Status:    status,
		UpdatedAt: dbtestutil.NowInDefaultTimezone(),
	})
	require.NoError(t, err)
}
