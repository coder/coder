package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/experiments/experimentstest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// automationsExperimentStore turns the chat-automations experiment off for
// every user while off is set.
type automationsExperimentStore struct {
	t   testing.TB
	off atomic.Bool
	// offFor, when set, turns the experiment off for that user only.
	offFor atomic.Pointer[uuid.UUID]
}

func (s *automationsExperimentStore) Rules(context.Context) (map[codersdk.Experiment]experiments.StoredRule, error) {
	if s.off.Load() {
		return map[codersdk.Experiment]experiments.StoredRule{
			codersdk.ExperimentChatAutomations: experimentstest.StoredRule(s.t, experiments.Rule{Mode: experiments.ModeOff}),
		}, nil
	}
	if id := s.offFor.Load(); id != nil {
		return map[codersdk.Experiment]experiments.StoredRule{
			codersdk.ExperimentChatAutomations: experimentstest.StoredRule(s.t, experiments.Rule{
				Mode:      experiments.ModeCondition,
				Condition: fmt.Sprintf("user.id != %q", id.String()),
			}),
		}, nil
	}
	return map[codersdk.Experiment]experiments.StoredRule{}, nil
}

func (*automationsExperimentStore) UserAttributes(_ context.Context, userID uuid.UUID) (experiments.User, error) {
	return experiments.User{ID: userID.String()}, nil
}

// scheduleFixture is a chat owned by an agents-access member and a mock
// clock that every server of the fixture shares.
type scheduleFixture struct {
	db         database.Store
	ps         pubsub.Pubsub
	sqlDB      *sql.DB
	clock      *quartz.Mock
	experiment *automationsExperimentStore
	auditor    *audit.MockAuditor
	owner      database.User
	org        database.Organization
	model      database.ChatModelConfig
	chat       database.Chat
	// hookConsumer, when set, receives the lifecycle hooks of servers
	// created afterwards.
	hookConsumer *httptest.Server
}

func newScheduleFixture(t *testing.T, status database.ChatStatus, start time.Time) *scheduleFixture {
	t.Helper()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	clock := quartz.NewMock(t)
	clock.Set(start)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: owner.ID, OrganizationID: org.ID})
	_, err := db.UpdateMemberRoles(testutil.Context(t, testutil.WaitShort), database.UpdateMemberRolesParams{
		GrantedRoles: []string{rbac.RoleAgentsAccess()},
		UserID:       owner.ID,
		OrgID:        org.ID,
	})
	require.NoError(t, err)
	provider := dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "openai", BaseUrl: chattest.OpenAI(t)})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		IsDefault:      true,
		OrganizationID: org.ID,
	})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: model.ID,
		Title:             "automation target",
		Status:            status,
	})
	return &scheduleFixture{
		db: db, ps: ps, sqlDB: sqlDB, clock: clock,
		experiment: &automationsExperimentStore{t: t},
		auditor:    audit.NewMock(),
		owner:      owner, org: org, model: model, chat: chat,
	}
}

// newServer returns an unstarted server, one instance of coderd.
func (f *scheduleFixture) newServer(t *testing.T, limits Limits) *Server {
	t.Helper()
	return f.newServerWithStore(t, limits, nil)
}

// newServerWithStore is newServer with the server's authorized store
// passed through wrap, when set, so a test can observe or pace its reads.
func (f *scheduleFixture) newServerWithStore(t *testing.T, limits Limits, wrap func(database.Store) database.Store) *Server {
	t.Helper()
	// Most tests use per-minute schedules, which the default minimum
	// interval between runs rejects.
	if limits.MinAutomationScheduleInterval == 0 {
		limits.MinAutomationScheduleInterval = time.Minute
	}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	evaluator, err := experiments.New(logger, f.experiment, codersdk.ExperimentsKnown)
	require.NoError(t, err)
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	var auditor atomic.Pointer[audit.Auditor]
	var a audit.Auditor = f.auditor
	auditor.Store(&a)
	store := dbauthz.New(f.db, authorizer, logger, nil)
	if wrap != nil {
		store = wrap(store)
	}
	var hookDispatcher *dispatch.Dispatcher
	if f.hookConsumer != nil {
		hookDispatcher = dispatch.New(logger, f.hookConsumer.Client(), f.hookConsumer.URL, false,
			"test-hook-secret-32-bytes-minimum!!", testutil.WaitMedium, "test-deployment", "test-version", prometheus.NewRegistry())
	}
	server, err := New(f.ps, Config{
		Logger: logger,
		// The server authorizes like coderd does, so the scan's reads and
		// cursor writes run under the chatd subject's real permissions.
		Database:                   store,
		ReplicaID:                  uuid.New(),
		Clock:                      f.clock,
		PendingChatAcquireInterval: testutil.WaitLong,
		Experiments:                codersdk.ExperimentsKnown,
		ExperimentEvaluator:        evaluator,
		Authorizer:                 authorizer,
		Auditor:                    &auditor,
		Limits:                     limits,
		HookDispatcher:             hookDispatcher,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	return server
}

// existingChat creates a schedule automation that sends to the fixture
// chat.
func (f *scheduleFixture) existingChat(ctx context.Context, t *testing.T, server *Server, spec, timeZone string, whenBusy codersdk.ChatAutomationWhenBusy) database.ChatAutomation {
	t.Helper()
	return f.create(ctx, t, server, codersdk.CreateChatAutomationRequest{
		TargetMode:   codersdk.ChatAutomationTargetModeExistingChat,
		TargetChatID: &f.chat.ID,
		WhenBusy:     &whenBusy,
	}, spec, timeZone)
}

// newChat creates a schedule automation that starts a chat per run.
func (f *scheduleFixture) newChat(ctx context.Context, t *testing.T, server *Server, spec, timeZone string) database.ChatAutomation {
	t.Helper()
	return f.create(ctx, t, server, codersdk.CreateChatAutomationRequest{
		TargetMode:           codersdk.ChatAutomationTargetModeNewChat,
		NewChatModelConfigID: &f.model.ID,
	}, spec, timeZone)
}

func (f *scheduleFixture) create(ctx context.Context, t *testing.T, server *Server, req codersdk.CreateChatAutomationRequest, spec, timeZone string) database.ChatAutomation {
	t.Helper()
	return f.createFor(ctx, t, server, f.owner.ID, req, spec, timeZone)
}

// createFor creates a schedule automation owned by ownerID.
func (f *scheduleFixture) createFor(ctx context.Context, t *testing.T, server *Server, ownerID uuid.UUID, req codersdk.CreateChatAutomationRequest, spec, timeZone string) database.ChatAutomation {
	t.Helper()
	req.Name = "Standup"
	req.Kind = codersdk.ChatAutomationKindSchedule
	req.Prompt = "Post the standup."
	req.ScheduleCron = &spec
	req.ScheduleTimeZone = &timeZone
	subject, err := automationOwnerSubject(ctx, f.db, ownerID)
	require.NoError(t, err)
	automation, _, err := server.CreateAutomation(dbauthz.As(ctx, subject), CreateAutomationParams{OrganizationID: f.org.ID, OwnerID: ownerID, Request: req})
	require.NoError(t, err)
	return automation
}

// asOwner returns ctx authorized as the fixture's owner.
func (f *scheduleFixture) asOwner(ctx context.Context, t *testing.T) context.Context {
	t.Helper()
	owner, err := automationOwnerSubject(ctx, f.db, f.owner.ID)
	require.NoError(t, err)
	return dbauthz.As(ctx, owner)
}

// cursor returns the automation's schedule cursor in UTC.
func (f *scheduleFixture) cursor(ctx context.Context, t *testing.T, automationID uuid.UUID) time.Time {
	t.Helper()
	automation, err := f.db.GetChatAutomationByID(ctx, automationID)
	require.NoError(t, err)
	require.True(t, automation.ScheduleNextRunAt.Valid)
	return automation.ScheduleNextRunAt.Time.UTC()
}

// claimedUntil returns the automation's schedule claim.
func (f *scheduleFixture) claimedUntil(ctx context.Context, t *testing.T, automationID uuid.UUID) sql.NullTime {
	t.Helper()
	automation, err := f.db.GetChatAutomationByID(ctx, automationID)
	require.NoError(t, err)
	return automation.ScheduleClaimedUntil
}

// inputs counts the user messages and queued messages of the fixture chat.
func (f *scheduleFixture) inputs(ctx context.Context, t *testing.T) int {
	t.Helper()
	var messages int
	err := f.sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM chat_messages WHERE chat_id = $1 AND role = 'user'", f.chat.ID).Scan(&messages)
	require.NoError(t, err)
	queued, err := f.db.GetChatQueuedMessages(ctx, f.chat.ID)
	require.NoError(t, err)
	return messages + len(queued)
}

// createdChats returns the owner's chats other than the fixture chat.
func (f *scheduleFixture) createdChats(ctx context.Context, t *testing.T) []database.Chat {
	t.Helper()
	var chats []database.Chat
	rows, err := f.sqlDB.QueryContext(ctx, "SELECT id FROM chats WHERE owner_id = $1 AND id <> $2", f.owner.ID, f.chat.ID)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		chat, err := f.db.GetChatByID(ctx, id)
		require.NoError(t, err)
		chats = append(chats, chat)
	}
	require.NoError(t, rows.Err())
	return chats
}

// advanceTo moves the mock clock to target, firing the servers' timers on
// the way.
func (f *scheduleFixture) advanceTo(ctx context.Context, t *testing.T, target time.Time) {
	t.Helper()
	for {
		remaining := target.Sub(f.clock.Now())
		require.GreaterOrEqual(t, remaining, time.Duration(0), "the mock clock only moves forward")
		if remaining == 0 {
			return
		}
		next, ok := f.clock.Peek()
		if !ok || next >= remaining {
			f.clock.Advance(remaining).MustWait(ctx)
			return
		}
		_, waiter := f.clock.AdvanceNext()
		waiter.MustWait(ctx)
	}
}

const (
	lockChatRow       = "SELECT 1 FROM chats WHERE id = $1 FOR UPDATE"
	lockAutomationRow = "SELECT 1 FROM chat_automations WHERE id = $1 FOR UPDATE"
)

// lockRow runs a locking query on a separate connection and holds the
// lock until the returned transaction ends.
func (f *scheduleFixture) lockRow(ctx context.Context, t *testing.T, query string, id uuid.UUID) *sql.Tx {
	t.Helper()
	tx, err := f.sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, query, id)
	require.NoError(t, err)
	return tx
}

// waitForLockWaits waits until n other backends wait for a row lock.
func (f *scheduleFixture) waitForLockWaits(ctx context.Context, t *testing.T, n int) {
	t.Helper()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var waits int
		err := f.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_stat_activity
WHERE datname = current_database() AND pid <> pg_backend_pid() AND wait_event_type = 'Lock'`).Scan(&waits)
		return err == nil && waits >= n
	}, testutil.IntervalFast, "wait for blocked scans")
}

// scanAsync starts a scan on each server and returns a wait function.
func scanAsync(ctx context.Context, servers ...*Server) func() {
	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Go(func() { server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize) })
	}
	return wg.Wait
}

func nextRun(t *testing.T, spec, timeZone string, after time.Time) time.Time {
	t.Helper()
	sched, err := cron.Standard(spec, timeZone)
	require.NoError(t, err)
	return sched.Next(after).UTC()
}

// scheduleStart is 30 seconds before the first run of a per-minute
// schedule.
var scheduleStart = time.Date(2026, time.June, 1, 10, 0, 30, 0, time.UTC)

func TestAutomationScheduleScan(t *testing.T) {
	t.Parallel()

	t.Run("ConcurrentInstancesRunHooksOnce", func(t *testing.T) {
		t.Parallel()
		for _, targetMode := range []codersdk.ChatAutomationTargetMode{
			codersdk.ChatAutomationTargetModeExistingChat,
			codersdk.ChatAutomationTargetModeNewChat,
		} {
			t.Run(string(targetMode), func(t *testing.T) {
				t.Parallel()
				f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
				ctx := testutil.Context(t, testutil.WaitLong)
				// The prompt hook holds the instance that runs it until
				// released, so that instance commits nothing before the
				// other instance has read the due occurrence.
				var promptHooks atomic.Int64
				release := make(chan struct{})
				var releaseOnce sync.Once
				releaseHooks := func() { releaseOnce.Do(func() { close(release) }) }
				f.hookConsumer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request agenthooks.Request
					if json.NewDecoder(r.Body).Decode(&request) == nil && request.Type == agenthooks.EventUserPromptSubmit {
						promptHooks.Add(1)
						select {
						case <-release:
						case <-r.Context().Done():
						}
					}
					_, _ = w.Write([]byte(`{}`))
				}))
				t.Cleanup(f.hookConsumer.Close)
				t.Cleanup(releaseHooks)
				first, second := f.newServer(t, Limits{}), f.newServer(t, Limits{})
				var automation database.ChatAutomation
				if targetMode == codersdk.ChatAutomationTargetModeExistingChat {
					automation = f.existingChat(ctx, t, first, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
				} else {
					automation = f.newChat(ctx, t, first, "* * * * *", "Asia/Tokyo")
				}
				f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)

				var finished atomic.Int64
				var wg sync.WaitGroup
				for _, server := range []*Server{first, second} {
					wg.Go(func() {
						server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
						finished.Add(1)
					})
				}
				// One instance runs the hook while the other finishes its
				// scan without it. Without the claim, both would run it.
				testutil.Eventually(ctx, t, func(context.Context) bool {
					return promptHooks.Load() >= 2 || (promptHooks.Load() == 1 && finished.Load() == 1)
				}, testutil.IntervalFast, "one instance runs the prompt hook")
				releaseHooks()
				wg.Wait()

				require.EqualValues(t, 1, promptHooks.Load(), "the prompt hook runs once per occurrence")
				if targetMode == codersdk.ChatAutomationTargetModeExistingChat {
					require.Equal(t, 1, f.inputs(ctx, t))
				} else {
					chats := f.createdChats(ctx, t)
					require.Len(t, chats, 1)
					// 10:01 UTC is 19:01 in Tokyo.
					require.Equal(t, "Standup · 1 Jun 19:01 JST", chats[0].Title)
					// Like a chat created through the chat API, the chat is
					// audited once, as created by the owner.
					logs := f.auditor.AuditLogs()
					require.Len(t, logs, 1)
					require.Equal(t, database.AuditActionCreate, logs[0].Action)
					require.Equal(t, database.ResourceTypeChat, logs[0].ResourceType)
					require.Equal(t, chats[0].ID, logs[0].ResourceID)
					require.Equal(t, f.owner.ID, logs[0].UserID)
				}
				require.Equal(t, scheduleStart.Add(90*time.Second), f.cursor(ctx, t, automation.ID))
				require.False(t, f.claimedUntil(ctx, t, automation.ID).Valid, "accepting the occurrence drops the claim")
			})
		}
	})

	t.Run("ClaimHeldElsewhereSkipsUntilExpiry", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "*/5 * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		due := automation.ScheduleNextRunAt.Time.UTC()
		f.advanceTo(ctx, t, due)
		claimedUntil := due.Add(automationScheduleClaimLease)
		_, err := f.sqlDB.ExecContext(ctx, "UPDATE chat_automations SET schedule_claimed_until = $1 WHERE id = $2", claimedUntil, automation.ID)
		require.NoError(t, err)

		// Another instance holds the claim, so the scan leaves the
		// occurrence to it.
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Zero(t, f.inputs(ctx, t))
		require.Equal(t, due, f.cursor(ctx, t, automation.ID))

		// That instance stopped. Once its claim expires, the occurrence is
		// past the grace window, so the scan moves the cursor without
		// publishing and drops the claim.
		f.advanceTo(ctx, t, claimedUntil)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Zero(t, f.inputs(ctx, t))
		require.Equal(t, nextRun(t, "*/5 * * * *", "UTC", f.clock.Now()), f.cursor(ctx, t, automation.ID))
		require.False(t, f.claimedUntil(ctx, t, automation.ID).Valid)
	})

	t.Run("RefusalSkipsOccurrence", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name     string
			whenBusy codersdk.ChatAutomationWhenBusy
			limits   Limits
			// fill runs before the refused occurrence and returns the
			// inputs it saved.
			fill func(ctx context.Context, t *testing.T, f *scheduleFixture, server *Server, automation database.ChatAutomation) int
		}{
			{
				name:     "BusyChatSkip",
				whenBusy: codersdk.ChatAutomationWhenBusySkip,
			},
			{
				// A queue of 2 leaves automations a share of 1, which the
				// first occurrence fills.
				name:     "QueueShareFull",
				whenBusy: codersdk.ChatAutomationWhenBusyQueue,
				limits:   Limits{MaxQueuedMessagesPerChat: 2},
				fill: func(ctx context.Context, t *testing.T, f *scheduleFixture, server *Server, automation database.ChatAutomation) int {
					f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)
					server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
					return 1
				},
			},
			{
				name:     "QueueFull",
				whenBusy: codersdk.ChatAutomationWhenBusyQueue,
				limits:   Limits{MaxQueuedMessagesPerChat: 1},
				fill: func(ctx context.Context, t *testing.T, f *scheduleFixture, server *Server, _ database.ChatAutomation) int {
					sent, err := server.SendMessage(f.asOwner(ctx, t), SendMessageOptions{
						ChatID:    f.chat.ID,
						CreatedBy: f.owner.ID,
						Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("from a person")},
					})
					require.NoError(t, err)
					require.True(t, sent.Queued)
					return 1
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newScheduleFixture(t, database.ChatStatusRunning, scheduleStart)
				ctx := testutil.Context(t, testutil.WaitLong)
				server := f.newServer(t, tc.limits)
				automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", tc.whenBusy)
				saved := 0
				if tc.fill != nil {
					saved = tc.fill(ctx, t, f, server, automation)
				}
				refused := f.cursor(ctx, t, automation.ID)
				f.advanceTo(ctx, t, refused)

				server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
				require.Equal(t, saved, f.inputs(ctx, t))
				next := refused.Add(time.Minute)
				require.Equal(t, next, f.cursor(ctx, t, automation.ID))

				// The cursor is in the future, so a repeat scan does nothing.
				server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
				require.Equal(t, saved, f.inputs(ctx, t))
				require.Equal(t, next, f.cursor(ctx, t, automation.ID))
			})
		}
	})

	t.Run("EditRollsBackInFlightOccurrence", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)

		// The publish has claimed the occurrence and waits for the chat
		// lock, which it takes before the automation lock, while the edit
		// changes the schedule like UpdateAutomation does. Holding the
		// automation lock instead would stop the scan at its claim, before
		// the publish rechecks the occurrence.
		held := f.lockRow(ctx, t, lockChatRow, f.chat.ID)
		wait := scanAsync(ctx, server)
		f.waitForLockWaits(ctx, t, 1)
		edited := scheduleStart.Add(270 * time.Second)
		_, err := held.ExecContext(ctx, `UPDATE chat_automations
SET schedule_revision = schedule_revision + 1, schedule_cron = '*/5 * * * *', schedule_next_run_at = $2
WHERE id = $1`, automation.ID, edited)
		require.NoError(t, err)
		require.NoError(t, held.Commit())
		wait()

		require.Zero(t, f.inputs(ctx, t))
		require.Equal(t, edited, f.cursor(ctx, t, automation.ID))
	})

	t.Run("Restart", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			spec string
			// late is how long after the occurrence the new instance scans.
			late     time.Duration
			accepted int
		}{
			{name: "WithinGrace", spec: "*/5 * * * *", late: automationScheduleGrace, accepted: 1},
			// No cron time falls within the grace window before now.
			{name: "AfterMissedRuns", spec: "*/5 * * * *", late: 12 * time.Minute, accepted: 0},
			// The cursor is missed, but the cron time 0.7 s before now is
			// within the grace window and runs in the same scan.
			{name: "AfterMissedRunsWithTimelyRun", spec: "* * * * *", late: 2*time.Minute + 700*time.Millisecond, accepted: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
				ctx := testutil.Context(t, testutil.WaitLong)
				automation := f.existingChat(ctx, t, f.newServer(t, Limits{}), tc.spec, "UTC", codersdk.ChatAutomationWhenBusyQueue)
				f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time.Add(tc.late))

				restarted := f.newServer(t, Limits{})
				for range 2 {
					restarted.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
					require.Equal(t, tc.accepted, f.inputs(ctx, t))
					// Missed runs are not replayed: the cursor moves to the
					// first run after now.
					require.Equal(t, nextRun(t, tc.spec, "UTC", f.clock.Now()), f.cursor(ctx, t, automation.ID))
				}
			})
		}
	})

	t.Run("PreviewsMatchAcceptedRuns", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "*/10 * * * *", "Europe/Berlin")
		previews := AutomationNextRuns(automation, f.clock.Now(), 4)
		require.Len(t, previews, 4)

		for i, run := range previews[:3] {
			require.Equal(t, run, f.cursor(ctx, t, automation.ID))
			f.advanceTo(ctx, t, run)
			server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
			require.Len(t, f.createdChats(ctx, t), i+1)
			require.Equal(t, previews[i+1], f.cursor(ctx, t, automation.ID))
		}
	})

	t.Run("ExpiresWhileWaitingForLock", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)

		held := f.lockRow(ctx, t, lockChatRow, f.chat.ID)
		wait := scanAsync(ctx, server)
		f.waitForLockWaits(ctx, t, 1)
		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time.Add(automationScheduleGrace+time.Second))
		require.NoError(t, held.Commit())
		wait()

		require.Zero(t, f.inputs(ctx, t))
		// The cursor moves to the first run within the grace window, which
		// the next scan publishes.
		require.Equal(t, nextRun(t, "* * * * *", "UTC", f.clock.Now().Add(-automationScheduleGrace)), f.cursor(ctx, t, automation.ID))
	})

	t.Run("ModelUnavailableSkipsOccurrence", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "* * * * *", "UTC")
		_, err := f.sqlDB.ExecContext(ctx, "UPDATE chat_model_configs SET enabled = false WHERE id = $1", f.model.ID)
		require.NoError(t, err)
		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)

		// The refusal is definite, so the first attempt moves the cursor
		// instead of retrying the occurrence on every scan.
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Empty(t, f.createdChats(ctx, t))
		require.Equal(t, nextRun(t, "* * * * *", "UTC", f.clock.Now()), f.cursor(ctx, t, automation.ID))
	})

	t.Run("CursorOnlyMovesForward", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		cursor := automation.ScheduleNextRunAt.Time
		occurrence := automationOccurrence{revision: automation.ScheduleRevision, cursor: cursor}

		// A clock that stepped back would compute a next run at or before
		// the cursor, which could replay an occurrence.
		for _, next := range []time.Time{cursor, cursor.Add(-time.Minute)} {
			moved, err := advanceAutomationSchedule(ctx, server.db, automation.ID, occurrence, next, f.clock.Now())
			require.Error(t, err)
			require.False(t, moved)
			require.Equal(t, cursor.UTC(), f.cursor(ctx, t, automation.ID))
		}
	})

	t.Run("DaylightSavingTime", func(t *testing.T) {
		t.Parallel()
		newYork, err := time.LoadLocation("America/New_York")
		require.NoError(t, err)
		// Clocks in New York move forward on 2026-03-08.
		f := newScheduleFixture(t, database.ChatStatusWaiting, time.Date(2026, time.March, 7, 8, 0, 0, 0, newYork))
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "0 9 * * *", "America/New_York")

		var runs []time.Time
		for range 3 {
			run := f.cursor(ctx, t, automation.ID)
			runs = append(runs, run)
			f.advanceTo(ctx, t, run)
			server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
			require.Equal(t, nextRun(t, "0 9 * * *", "America/New_York", run), f.cursor(ctx, t, automation.ID))
		}
		for _, run := range runs {
			local := run.In(newYork)
			require.Equal(t, 9, local.Hour())
			require.Zero(t, local.Minute())
		}
		require.Equal(t, 23*time.Hour, runs[1].Sub(runs[0]), "the day clocks move forward is one hour short")
		require.Equal(t, 24*time.Hour, runs[2].Sub(runs[1]))
		require.Len(t, f.createdChats(ctx, t), 3)
	})

	t.Run("ExperimentOff", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		due := automation.ScheduleNextRunAt.Time.UTC()
		f.advanceTo(ctx, t, due)
		f.experiment.off.Store(true)

		// The scan drops the owner and leaves the cursor.
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Zero(t, f.inputs(ctx, t))
		require.Equal(t, due, f.cursor(ctx, t, automation.ID))

		// An experiment that turns off after the scan decided the owner is
		// refused by Publish. The evaluator also reports a failed read as
		// off, so the cursor stays for a retry.
		row, err := f.db.GetChatAutomationByID(ctx, automation.ID)
		require.NoError(t, err)
		server.runAutomationOccurrence(ctx, row, f.clock.Now())
		require.Zero(t, f.inputs(ctx, t))
		require.Equal(t, due, f.cursor(ctx, t, automation.ID))
		// The failed publish releases its claim, so the next scan can
		// claim the occurrence again.
		require.False(t, f.claimedUntil(ctx, t, automation.ID).Valid)

		// Within the grace window, the next scan accepts it once.
		f.experiment.off.Store(false)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Equal(t, 1, f.inputs(ctx, t))
		require.Equal(t, due.Add(time.Minute), f.cursor(ctx, t, automation.ID))
	})

	t.Run("SlowOccurrenceDoesNotBlockOthers", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		other := dbgen.Chat(t, f.db, database.Chat{
			OrganizationID:    f.org.ID,
			OwnerID:           f.owner.ID,
			LastModelConfigID: f.model.ID,
			Title:             "other target",
			Status:            database.ChatStatusWaiting,
		})
		blocked := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		free := f.createFor(ctx, t, server, f.owner.ID, codersdk.CreateChatAutomationRequest{
			TargetMode:   codersdk.ChatAutomationTargetModeExistingChat,
			TargetChatID: &other.ID,
		}, "* * * * *", "UTC")
		due := blocked.ScheduleNextRunAt.Time.UTC()
		// The blocked occurrence comes first in the scan's order.
		_, err := f.sqlDB.ExecContext(ctx, "UPDATE chat_automations SET schedule_next_run_at = $1 WHERE id = $2", due.Add(-time.Second), blocked.ID)
		require.NoError(t, err)
		f.advanceTo(ctx, t, due)
		held := f.lockRow(ctx, t, lockChatRow, f.chat.ID)

		wait := scanAsync(ctx, server)
		// The other chat gets its message while the first occurrence
		// still waits for the chat lock.
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			var n int
			err := f.sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM chat_messages WHERE chat_id = $1 AND role = 'user'", other.ID).Scan(&n)
			return err == nil && n == 1
		}, testutil.IntervalFast, "the free occurrence is accepted")
		require.Zero(t, f.inputs(ctx, t))
		require.NoError(t, held.Commit())
		wait()
		require.Equal(t, 1, f.inputs(ctx, t))
		require.Equal(t, due.Add(time.Minute), f.cursor(ctx, t, free.ID))
	})

	t.Run("NewChatTitleNamesOccurrence", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "* * * * *", "Asia/Tokyo")
		// Accepted at the end of the grace window, a minute after the
		// 10:01 UTC occurrence.
		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time.Add(automationScheduleGrace))
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		chats := f.createdChats(ctx, t)
		require.Len(t, chats, 1)
		require.Equal(t, "Standup · 1 Jun 19:01 JST", chats[0].Title)
	})

	t.Run("PagesPastRowsThatStayDue", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		// Another member whose experiment is off owns two due schedules
		// with older cursors. The scan drops them without a write, so they
		// stay first in every page.
		other := dbgen.User(t, f.db, database.User{})
		dbgen.OrganizationMember(t, f.db, database.OrganizationMember{UserID: other.ID, OrganizationID: f.org.ID})
		_, err := f.db.UpdateMemberRoles(ctx, database.UpdateMemberRolesParams{
			GrantedRoles: []string{rbac.RoleAgentsAccess()},
			UserID:       other.ID,
			OrgID:        f.org.ID,
		})
		require.NoError(t, err)
		f.experiment.offFor.Store(&other.ID)
		newChat := codersdk.CreateChatAutomationRequest{
			TargetMode:           codersdk.ChatAutomationTargetModeNewChat,
			NewChatModelConfigID: &f.model.ID,
		}
		var dropped []database.ChatAutomation
		for range 2 {
			dropped = append(dropped, f.createFor(ctx, t, server, other.ID, newChat, "* * * * *", "UTC"))
		}
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		due := automation.ScheduleNextRunAt.Time.UTC()
		for _, row := range dropped {
			_, err := f.sqlDB.ExecContext(ctx, "UPDATE chat_automations SET schedule_next_run_at = $1 WHERE id = $2", due.Add(-time.Second), row.ID)
			require.NoError(t, err)
		}
		f.advanceTo(ctx, t, due)

		// One row per page: the scan pages past the dropped rows.
		server.scanAutomationSchedules(ctx, 1)
		require.Equal(t, 1, f.inputs(ctx, t))
		require.Equal(t, due.Add(time.Minute), f.cursor(ctx, t, automation.ID))
		for _, row := range dropped {
			require.Equal(t, due.Add(-time.Second), f.cursor(ctx, t, row.ID))
		}
	})
}

func TestAutomationScheduleLoopScansAtStart(t *testing.T) {
	t.Parallel()
	f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
	ctx := testutil.Context(t, testutil.WaitLong)
	automation := f.existingChat(ctx, t, f.newServer(t, Limits{}), "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
	server := f.newServer(t, Limits{})
	f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time.Add(5*time.Minute))

	// The loop scans before it creates its ticker, so the missed
	// occurrence is skipped by the time the ticker exists.
	tickerTrap := f.clock.Trap().NewTicker("chatworker", "automation-schedules")
	defer tickerTrap.Close()
	server.Start()
	tickerTrap.MustWait(ctx).MustRelease(ctx)
	require.Equal(t, nextRun(t, "* * * * *", "UTC", f.clock.Now()), f.cursor(ctx, t, automation.ID))
}
