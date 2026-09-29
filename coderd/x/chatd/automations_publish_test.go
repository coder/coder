package chatd_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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

// newChatWebhook creates a webhook automation that starts a new chat for
// every event.
func (f publishFixture) newChatWebhook(ctx context.Context, t *testing.T, use codersdk.ChatAutomationWebhookUse) database.ChatAutomation {
	t.Helper()
	automation, _, err := f.server.CreateAutomation(ctx, chatd.CreateAutomationParams{
		OrganizationID: f.org.ID,
		OwnerID:        f.owner.ID,
		Request: codersdk.CreateChatAutomationRequest{
			Name:                 "Deploy hook",
			Kind:                 codersdk.ChatAutomationKindWebhook,
			TargetMode:           codersdk.ChatAutomationTargetModeNewChat,
			NewChatModelConfigID: &f.model.ID,
			ReasoningEffort:      ptr.Ref(string(database.ChatReasoningEffortHigh)),
			Prompt:               "A deploy finished.",
			WebhookUse:           &use,
		},
	})
	require.NoError(t, err)
	return automation
}

// singleUseWebhook creates a single-use webhook automation with the
// given target mode.
func (f publishFixture) singleUseWebhook(ctx context.Context, t *testing.T, targetMode codersdk.ChatAutomationTargetMode) database.ChatAutomation {
	t.Helper()
	if targetMode == codersdk.ChatAutomationTargetModeNewChat {
		return f.newChatWebhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle)
	}
	return f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusyQueue)
}

// createdChats returns the owner's chats other than the fixture chat.
func (f publishFixture) createdChats(ctx context.Context, t *testing.T) []uuid.UUID {
	t.Helper()
	rows, err := f.sqlDB.QueryContext(ctx, "SELECT id FROM chats WHERE owner_id = $1 AND id <> $2", f.owner.ID, f.chat.ID)
	require.NoError(t, err)
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

// removeAgentsAccess leaves the owner with no organization roles, so the
// owner may neither write the fixture chat nor create chats.
func (f publishFixture) removeAgentsAccess(ctx context.Context) error {
	_, err := f.db.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
		GrantedRoles: []string{},
		UserID:       f.owner.ID,
		OrgID:        f.org.ID,
	})
	if err != nil {
		return err
	}
	_, err = f.sqlDB.ExecContext(ctx, "UPDATE organizations SET default_org_member_roles = '{}' WHERE id = $1", f.org.ID)
	return err
}

func (f publishFixture) requireNotConsumed(ctx context.Context, t *testing.T, automation database.ChatAutomation) {
	t.Helper()
	stored, err := f.db.GetChatAutomationByID(ctx, automation.ID)
	require.NoError(t, err)
	require.False(t, stored.WebhookConsumedAt.Valid, "a refused delivery does not use up a single-use webhook")
}

// newHookConsumer returns a user_prompt_submit hook consumer that answers
// with body after calling onCall, and the number of hook calls.
func newHookConsumer(t *testing.T, body string, onCall func()) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if onCall != nil {
			onCall()
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(consumer.Close)
	return consumer, &calls
}

const (
	// hookNoDecision lets the prompt through unchanged.
	hookNoDecision = `{}`
	hookDeny       = `{"permission":{"decision":"deny"},"user_message":"blocked"}`
)

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

	t.Run("SecretRotatedBeforeSend", func(t *testing.T) {
		t.Parallel()
		for _, targetMode := range []codersdk.ChatAutomationTargetMode{
			codersdk.ChatAutomationTargetModeExistingChat,
			codersdk.ChatAutomationTargetModeNewChat,
		} {
			t.Run(string(targetMode), func(t *testing.T) {
				t.Parallel()
				consumer, hookCalls := newHookConsumer(t, hookDeny, nil)
				f := newPublishFixture(t, database.ChatStatusWaiting, func(cfg *chatd.Config) {
					cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
				})
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.singleUseWebhook(ctx, t, targetMode)
				_, _, err := f.server.RotateAutomationSecret(ctx, f.owner.ID, automation.ID)
				require.NoError(t, err)

				// automation still carries the version the request was
				// verified against. The payload of a stale secret never
				// reaches the hooks.
				_, err = f.publish(ctx, automation)
				require.ErrorIs(t, err, chatd.ErrAutomationSecretChanged)
				require.Zero(t, hookCalls.Load())
				f.requireNothingSaved(ctx, t)
				require.Empty(t, f.createdChats(ctx, t))
			})
		}
	})

	t.Run("SecretRotatedBeforeCommit", func(t *testing.T) {
		t.Parallel()
		// The hook runs after the pre-send checks and before the locks,
		// so rotating there reaches the check under lock.
		var rotate func()
		consumer, hookCalls := newHookConsumer(t, hookNoDecision, func() { rotate() })
		f := newPublishFixture(t, database.ChatStatusWaiting, func(cfg *chatd.Config) {
			cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusyQueue)
		rotate = func() {
			_, _, err := f.server.RotateAutomationSecret(ctx, f.owner.ID, automation.ID)
			assert.NoError(t, err)
		}

		_, err := f.publish(ctx, automation)
		require.ErrorIs(t, err, chatd.ErrAutomationSecretChanged)
		require.EqualValues(t, 1, hookCalls.Load())
		f.requireNothingSaved(ctx, t)
		f.requireNotConsumed(ctx, t, automation)
	})

	t.Run("SkipBusyChat", func(t *testing.T) {
		t.Parallel()
		consumer, hookCalls := newHookConsumer(t, hookNoDecision, nil)
		f := newPublishFixture(t, database.ChatStatusRunning, func(cfg *chatd.Config) {
			cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusySkip)

		_, err := f.publish(ctx, automation)
		require.ErrorIs(t, err, chatd.ErrAutomationChatBusy)
		require.Zero(t, hookCalls.Load(), "a refused input never reaches the hooks")
		f.requireNothingSaved(ctx, t)
		f.requireNotConsumed(ctx, t, automation)
	})

	t.Run("SkipChatBusyBeforeCommit", func(t *testing.T) {
		t.Parallel()
		// The hook runs after the pre-send checks and before the locks,
		// so a turn starting there reaches the busy check under lock.
		var startTurn func()
		consumer, hookCalls := newHookConsumer(t, hookNoDecision, func() { startTurn() })
		f := newPublishFixture(t, database.ChatStatusWaiting, func(cfg *chatd.Config) {
			cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusySkip)
		startTurn = func() {
			_, err := f.sqlDB.ExecContext(ctx, "UPDATE chats SET status = 'running' WHERE id = $1", f.chat.ID)
			assert.NoError(t, err)
		}

		_, err := f.publish(ctx, automation)
		require.ErrorIs(t, err, chatd.ErrAutomationChatBusy)
		require.EqualValues(t, 1, hookCalls.Load())
		f.requireNothingSaved(ctx, t)
	})

	t.Run("QueueBusyChatShare", func(t *testing.T) {
		t.Parallel()
		consumer, hookCalls := newHookConsumer(t, hookNoDecision, nil)
		// A queue of 4 leaves automations a share of 2.
		f := newPublishFixture(t, database.ChatStatusRunning, func(cfg *chatd.Config) {
			cfg.Limits = chatd.Limits{MaxQueuedMessagesPerChat: 4}
			cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
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
		require.EqualValues(t, 2, hookCalls.Load(), "a refused input never reaches the hooks")

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

	t.Run("QueueBusyChatShareIgnoresStaleRows", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name  string
			stale string
		}{
			{"Disabled", "UPDATE chat_automations SET enabled = false WHERE id = $1"},
			{"NewerGeneration", "UPDATE chat_automations SET queue_generation = queue_generation + 1 WHERE id = $1"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				// A queue of 4 leaves automations a share of 2.
				f := newPublishFixture(t, database.ChatStatusRunning, func(cfg *chatd.Config) {
					cfg.Limits = chatd.Limits{MaxQueuedMessagesPerChat: 4}
				})
				ctx := testutil.Context(t, testutil.WaitLong)
				filler := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti, codersdk.ChatAutomationWhenBusyQueue)
				for range 2 {
					_, err := f.publish(ctx, filler)
					require.NoError(t, err)
				}
				// Promotion drops the filler's rows now, so they leave
				// the share free. Changing the row directly keeps them
				// queued.
				_, err := f.sqlDB.ExecContext(ctx, tc.stale, filler.ID)
				require.NoError(t, err)

				automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti, codersdk.ChatAutomationWhenBusyQueue)
				_, err = f.publish(ctx, automation)
				require.NoError(t, err)
				queued, err := f.db.GetChatQueuedMessages(ctx, f.chat.ID)
				require.NoError(t, err)
				require.Len(t, queued, 3)
			})
		}
	})

	t.Run("SkipErroredChatWithOnlyStaleRows", func(t *testing.T) {
		t.Parallel()
		f := newPublishFixture(t, database.ChatStatusRunning)
		ctx := testutil.Context(t, testutil.WaitLong)
		filler := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti, codersdk.ChatAutomationWhenBusyQueue)
		_, err := f.publish(ctx, filler)
		require.NoError(t, err)
		// Promotion would drop the stale row, so the errored chat counts
		// as idle.
		_, err = f.sqlDB.ExecContext(ctx, "UPDATE chat_automations SET queue_generation = queue_generation + 1 WHERE id = $1", filler.ID)
		require.NoError(t, err)
		_, err = f.sqlDB.ExecContext(ctx, "UPDATE chats SET status = 'error' WHERE id = $1", f.chat.ID)
		require.NoError(t, err)

		automation := f.webhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle, codersdk.ChatAutomationWhenBusySkip)
		result, err := f.publish(ctx, automation)
		require.NoError(t, err)
		require.Equal(t, f.chat.ID, result.ChatID)
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
				require.NoError(t, f.removeAgentsAccess(ctx))
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
		for _, targetMode := range []codersdk.ChatAutomationTargetMode{
			codersdk.ChatAutomationTargetModeExistingChat,
			codersdk.ChatAutomationTargetModeNewChat,
		} {
			t.Run(string(targetMode), func(t *testing.T) {
				t.Parallel()
				consumer, hookCalls := newHookConsumer(t, hookDeny, nil)
				f := newPublishFixture(t, database.ChatStatusWaiting, func(cfg *chatd.Config) {
					cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
				})
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.singleUseWebhook(ctx, t, targetMode)

				_, err := f.publish(ctx, automation)
				var denied *chathooks.UserPromptDeniedError
				require.ErrorAs(t, err, &denied)
				require.EqualValues(t, 1, hookCalls.Load())
				f.requireNothingSaved(ctx, t)
				require.Empty(t, f.createdChats(ctx, t))
				f.requireNotConsumed(ctx, t, automation)
			})
		}
	})

	t.Run("NewChat", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		clock.Set(time.Date(2026, time.March, 4, 22, 7, 30, 0, time.FixedZone("CET", 3600)))
		f := newPublishFixture(t, database.ChatStatusWaiting, func(cfg *chatd.Config) {
			cfg.Clock = clock
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.newChatWebhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti)

		result, err := f.publish(ctx, automation)
		require.NoError(t, err)
		require.Equal(t, []uuid.UUID{result.ChatID}, f.createdChats(ctx, t))
		chat, err := f.db.GetChatByID(ctx, result.ChatID)
		require.NoError(t, err)
		require.Equal(t, uuid.NullUUID{UUID: automation.ID, Valid: true}, chat.AutomationID)
		require.Equal(t, "Deploy hook 2026-03-04 21:07 UTC", chat.Title)
		require.Equal(t, database.ChatClientTypeApi, chat.ClientType)
		require.Equal(t, f.model.ID, chat.LastModelConfigID)
		require.Empty(t, chat.MCPServerIDs)

		var user []database.ChatMessage
		for _, message := range chatMessages(ctx, t, f.db, chat.ID) {
			if message.Role == database.ChatMessageRoleUser {
				user = append(user, message)
			}
		}
		require.Len(t, user, 1)
		message := user[0]
		require.Equal(t, uuid.NullUUID{UUID: automation.ID, Valid: true}, message.AutomationID)
		require.Equal(t, uuid.NullUUID{UUID: result.InputID, Valid: true}, message.InputID)
		require.Equal(t, uuid.NullUUID{UUID: f.owner.ID, Valid: true}, message.CreatedBy)
		require.Equal(t, database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffortHigh, Valid: true}, message.ReasoningEffort)
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		require.Len(t, parts, 2)
		require.Equal(t, codersdk.ChatMessageText("A deploy finished."), parts[0])
		require.Contains(t, parts[1].Text, "<automation_event_data>\n{\"service\":\"api\"}\n</automation_event_data>")
	})

	t.Run("NewChatModelUnavailable", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name   string
			update string
		}{
			{"Disabled", "UPDATE chat_model_configs SET enabled = false WHERE id = $1"},
			{"Deleted", "UPDATE chat_model_configs SET deleted = true WHERE id = $1"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newPublishFixture(t, database.ChatStatusWaiting)
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.newChatWebhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle)
				_, err := f.sqlDB.ExecContext(ctx, tc.update, f.model.ID)
				require.NoError(t, err)

				_, err = f.publish(ctx, automation)
				require.ErrorIs(t, err, chatd.ErrAutomationModelUnavailable)
				require.Empty(t, f.createdChats(ctx, t))
				f.requireNotConsumed(ctx, t, automation)
			})
		}
	})

	t.Run("NewChatOwnerCannotCreateChats", func(t *testing.T) {
		t.Parallel()
		f := newPublishFixture(t, database.ChatStatusWaiting)
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.newChatWebhook(ctx, t, codersdk.ChatAutomationWebhookUseMulti)
		require.NoError(t, f.removeAgentsAccess(ctx))

		_, err := f.publish(ctx, automation)
		require.ErrorIs(t, err, chatd.ErrAutomationForbidden)
		require.Empty(t, f.createdChats(ctx, t))
	})

	t.Run("NewChatRefusedUnderLock", func(t *testing.T) {
		t.Parallel()
		// The user_prompt_submit hook runs after the checks made before
		// the create, so a change made while it runs is caught only by
		// the checks repeated under the automation lock, after the chat
		// row is inserted.
		for _, tc := range []struct {
			name   string
			change func(publishFixture, context.Context) error
			want   error
		}{
			{"ModelDisabled", func(f publishFixture, ctx context.Context) error {
				_, err := f.sqlDB.ExecContext(ctx, "UPDATE chat_model_configs SET enabled = false WHERE id = $1", f.model.ID)
				return err
			}, chatd.ErrAutomationModelUnavailable},
			{"NoAgentsAccess", publishFixture.removeAgentsAccess, chatd.ErrAutomationForbidden},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				var change func()
				consumer, hookCalls := newHookConsumer(t, hookNoDecision, func() { change() })
				f := newPublishFixture(t, database.ChatStatusWaiting, func(cfg *chatd.Config) {
					cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
				})
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.newChatWebhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle)
				change = func() { assert.NoError(t, tc.change(f, ctx)) }

				_, err := f.publish(ctx, automation)
				require.ErrorIs(t, err, tc.want)
				require.EqualValues(t, 1, hookCalls.Load())
				require.Empty(t, f.createdChats(ctx, t))
				f.requireNotConsumed(ctx, t, automation)
			})
		}
	})

	t.Run("NewChatSingleUseConcurrent", func(t *testing.T) {
		t.Parallel()
		f := newPublishFixture(t, database.ChatStatusWaiting)
		ctx := testutil.Context(t, testutil.WaitLong)
		automation := f.newChatWebhook(ctx, t, codersdk.ChatAutomationWebhookUseSingle)

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
		require.Len(t, f.createdChats(ctx, t), 1)
	})

	t.Run("SingleUseConsumedSkipsHooks", func(t *testing.T) {
		t.Parallel()
		for _, targetMode := range []codersdk.ChatAutomationTargetMode{
			codersdk.ChatAutomationTargetModeExistingChat,
			codersdk.ChatAutomationTargetModeNewChat,
		} {
			t.Run(string(targetMode), func(t *testing.T) {
				t.Parallel()
				consumer, hookCalls := newHookConsumer(t, hookDeny, nil)
				f := newPublishFixture(t, database.ChatStatusWaiting, func(cfg *chatd.Config) {
					cfg.HookDispatcher = newHookDispatcher(t, nil, consumer)
				})
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.singleUseWebhook(ctx, t, targetMode)
				count, err := f.db.ConsumeChatAutomationWebhookByID(ctx, database.ConsumeChatAutomationWebhookByIDParams{
					ID:  automation.ID,
					Now: dbtestutil.NowInDefaultTimezone(),
				})
				require.NoError(t, err)
				require.EqualValues(t, 1, count)

				// A repeated request to a used webhook must not expose its
				// payload to prompt hooks.
				_, err = f.publish(ctx, automation)
				require.ErrorIs(t, err, chatd.ErrAutomationWebhookConsumed)
				require.Zero(t, hookCalls.Load())
				f.requireNothingSaved(ctx, t)
				require.Empty(t, f.createdChats(ctx, t))
			})
		}
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
