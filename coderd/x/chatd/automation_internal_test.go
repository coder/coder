package chatd

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAutomationSchedule(t *testing.T) {
	t.Parallel()

	type fixture struct {
		worker *workerTestFixture
		model  database.ChatModelConfig
		server *Server
	}
	// The servers are not started, so a chat that a run starts stays busy.
	newServer := func(t *testing.T, f fixture, hooks *dispatch.Dispatcher) *Server {
		server, err := New(f.worker.pubsub, Config{
			Logger:             slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			Database:           f.worker.db,
			ReplicaID:          uuid.New(),
			Experiments:        codersdk.Experiments{codersdk.ExperimentAgentLifecycleHooks},
			HookDispatcher:     hooks,
			AutomationsEnabled: func(context.Context, uuid.UUID) bool { return true },
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, server.Close()) })
		return server
	}
	setup := func(t *testing.T, hooks *dispatch.Dispatcher) fixture {
		db, ps := dbtestutil.NewDB(t)
		user, org, model := seedInternalChatDeps(t, db)
		f := fixture{worker: &workerTestFixture{db: db, pubsub: newRecordingPubsub(ps), user: user, org: org}, model: model}
		f.server = newServer(t, f, hooks)
		return f
	}
	// schedule seeds an enabled */5 schedule that came due a second before
	// now.
	schedule := func(t *testing.T, f fixture, seed database.ChatAutomation, now time.Time) database.ChatAutomation {
		seed.OrganizationID = f.worker.org.ID
		seed.OwnerID = f.worker.user.ID
		seed.Enabled = true
		seed.ScheduleCron = sql.NullString{String: "*/5 * * * *", Valid: true}
		seed.ScheduleNextRunAt = sql.NullTime{Time: now.Add(-time.Second), Valid: true}
		return dbgen.ChatAutomation(t, f.worker.db, seed)
	}
	idleChat := func(t *testing.T, f fixture) database.Chat {
		return dbgen.Chat(t, f.worker.db, database.Chat{OrganizationID: f.worker.org.ID, OwnerID: f.worker.user.ID, LastModelConfigID: f.model.ID})
	}
	existingChat := func(chat database.Chat) database.ChatAutomation {
		return database.ChatAutomation{TargetChatID: uuid.NullUUID{UUID: chat.ID, Valid: true}}
	}
	sentBy := func(ctx context.Context, t *testing.T, f fixture, chatID, automationID uuid.UUID) int {
		messages, err := f.worker.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
		require.NoError(t, err)
		sent := 0
		for _, m := range messages {
			if m.AutomationID == (uuid.NullUUID{UUID: automationID, Valid: true}) {
				sent++
			}
		}
		return sent
	}
	read := func(ctx context.Context, t *testing.T, f fixture, id uuid.UUID) database.ChatAutomation {
		automation, err := f.worker.db.GetChatAutomationByID(ctx, id)
		require.NoError(t, err)
		return automation
	}
	requireLastRun := func(t *testing.T, a database.ChatAutomation, outcome database.ChatAutomationRunOutcome, reason string, unaccepted int32) {
		t.Helper()
		require.Equal(t, database.NullChatAutomationRunOutcome{ChatAutomationRunOutcome: outcome, Valid: true}, a.LastRunOutcome)
		require.Equal(t, reason, a.LastRunReason.String)
		require.Equal(t, unaccepted, a.LastRunUnacceptedCount)
	}

	t.Run("IdleChatThenBusyChat", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		f := setup(t, nil)
		chat := idleChat(t, f)
		now := dbtime.Now()
		automation := schedule(t, f, existingChat(chat), now)

		f.server.tickSchedules(ctx, now)
		automation = read(ctx, t, f, automation.ID)
		requireLastRun(t, automation, database.ChatAutomationRunOutcomeAccepted, "", 0)
		require.Equal(t, 1, sentBy(ctx, t, f, chat.ID, automation.ID))
		require.True(t, automation.ScheduleNextRunAt.Time.After(now))

		// The turn that the first run started is still running.
		now = automation.ScheduleNextRunAt.Time.Add(time.Second)
		f.server.tickSchedules(ctx, now)
		automation = read(ctx, t, f, automation.ID)
		requireLastRun(t, automation, database.ChatAutomationRunOutcomeSkipped, string(runReasonChatBusy), 1)
		require.Equal(t, 1, sentBy(ctx, t, f, chat.ID, automation.ID))

		forceExecutionState(t, f.worker, chat.ID, database.ChatStatusWaiting, false)
		f.server.tickSchedules(ctx, automation.ScheduleNextRunAt.Time.Add(time.Second))
		requireLastRun(t, read(ctx, t, f, automation.ID), database.ChatAutomationRunOutcomeAccepted, "", 0)
		require.Equal(t, 2, sentBy(ctx, t, f, chat.ID, automation.ID))
	})

	t.Run("ReplicaSkipsClaimedOccurrence", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		f := setup(t, nil)
		chat := idleChat(t, f)
		now := dbtime.Now()
		automation := schedule(t, f, existingChat(chat), now)
		other := newServer(t, f, nil)

		require.NoError(t, f.worker.db.InTx(func(tx database.Store) error {
			claimed, err := tx.ClaimDueChatAutomationSchedules(ctx, database.ClaimDueChatAutomationSchedulesParams{Now: now, LimitCount: 10})
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			// Another replica scans while this claim is open: it skips the
			// locked row instead of waiting for it or running it.
			other.tickSchedules(ctx, now)
			return nil
		}, nil))
		require.Zero(t, sentBy(ctx, t, f, chat.ID, automation.ID))
		require.False(t, read(ctx, t, f, automation.ID).LastRunOutcome.Valid)

		f.server.tickSchedules(ctx, now)
		other.tickSchedules(ctx, now)
		require.Equal(t, 1, sentBy(ctx, t, f, chat.ID, automation.ID))
	})

	t.Run("DisableAfterClaim", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		f := setup(t, nil)
		// The idle chat stops the run under the lock; the busy chat stops it
		// before, so the disable must also keep chat_busy out of last_run.
		idle, busy := idleChat(t, f), idleChat(t, f)
		forceExecutionState(t, f.worker, busy.ID, database.ChatStatusRunning, false)
		now := dbtime.Now()
		automations := []database.ChatAutomation{schedule(t, f, existingChat(idle), now), schedule(t, f, existingChat(busy), now)}

		claimed, err := f.server.claimDueSchedules(ctx, now)
		require.NoError(t, err)
		require.Len(t, claimed, 2)
		for _, a := range automations {
			_, _, err = chatautomation.Update(ctx, f.worker.db, f.worker.org.ID, a.ID, codersdk.UpdateChatAutomationRequest{Enabled: new(false)}, now)
			require.NoError(t, err)
		}
		for _, a := range claimed {
			f.server.runScheduleOccurrence(ctx, a, now)
			require.Zero(t, sentBy(ctx, t, f, a.TargetChatID.UUID, a.ID))
			require.False(t, read(ctx, t, f, a.ID).LastRunOutcome.Valid)
		}
	})

	t.Run("PromptHook", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name    string
			status  int
			body    string
			outcome database.ChatAutomationRunOutcome
			reason  runReason
		}{
			{"Denies", http.StatusOK, `{"permission":{"decision":"deny"},"user_message":"blocked"}`, database.ChatAutomationRunOutcomeRefused, runReasonPromptHookDenied},
			// A failing hook endpoint is not a decision, and a later run can
			// succeed.
			{"Fails", http.StatusInternalServerError, "", database.ChatAutomationRunOutcomeSkipped, runReasonPromptHookFailed},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}))
				t.Cleanup(consumer.Close)
				f := setup(t, dispatch.New(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), consumer.Client(), consumer.URL, false,
					"test-hook-secret-32-bytes-minimum!!", time.Second, "test-deployment", "test-version", prometheus.NewRegistry()))
				chat := idleChat(t, f)
				now := dbtime.Now()
				automation := schedule(t, f, existingChat(chat), now)

				f.server.tickSchedules(ctx, now)
				requireLastRun(t, read(ctx, t, f, automation.ID), tc.outcome, string(tc.reason), 1)
				require.Zero(t, sentBy(ctx, t, f, chat.ID, automation.ID))
				// The outcome reports the hook; the chat stays as it was.
				chat, err := f.worker.db.GetChatByID(ctx, chat.ID)
				require.NoError(t, err)
				require.Equal(t, database.ChatStatusWaiting, chat.Status)
			})
		}
	})

	t.Run("NewChatSkipsWhileEarlierChatRuns", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		f := setup(t, nil)
		now := dbtime.Now()
		automation := schedule(t, f, database.ChatAutomation{
			TargetMode:           database.ChatAutomationTargetModeNewChat,
			NewChatModelConfigID: uuid.NullUUID{UUID: f.model.ID, Valid: true},
		}, now)

		f.server.tickSchedules(ctx, now)
		automation = read(ctx, t, f, automation.ID)
		requireLastRun(t, automation, database.ChatAutomationRunOutcomeAccepted, "", 0)
		created, err := f.worker.db.GetChatByID(ctx, automation.LastRunChatID.UUID)
		require.NoError(t, err)
		require.Equal(t, uuid.NullUUID{UUID: automation.ID, Valid: true}, created.AutomationID)
		require.Equal(t, 1, sentBy(ctx, t, f, created.ID, automation.ID))

		f.server.tickSchedules(ctx, automation.ScheduleNextRunAt.Time.Add(time.Second))
		requireLastRun(t, read(ctx, t, f, automation.ID), database.ChatAutomationRunOutcomeSkipped, string(runReasonOverlap), 1)
	})
}
