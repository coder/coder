package chatautomation_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// fixture is one organization with a member who owns automations and chats.
type fixture struct {
	db    database.Store
	sqlDB *sql.DB
	org   database.Organization
	owner database.User
	model database.ChatModelConfig
}

func newFixture(t *testing.T, db database.Store, sqlDB *sql.DB) fixture {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: owner.ID, OrganizationID: org.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})
	return fixture{db: db, sqlDB: sqlDB, org: org, owner: owner, model: model}
}

// chat inserts a running chat, a state that is valid with and without a
// queue.
func (f fixture) chat(t *testing.T) database.Chat {
	t.Helper()
	return dbgen.Chat(t, f.db, database.Chat{
		OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: f.model.ID,
		Status: database.ChatStatusRunning,
	})
}

func (f fixture) automation(t *testing.T, target database.Chat) database.ChatAutomation {
	t.Helper()
	return dbgen.ChatAutomation(t, f.db, database.ChatAutomation{
		OrganizationID: f.org.ID,
		OwnerID:        f.owner.ID,
		TargetChatID:   uuid.NullUUID{UUID: target.ID, Valid: true},
	})
}

// execer is a *sql.DB or a *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// queue inserts a queued message of automationID into the chat, or an
// ordinary queued message when automationID is uuid.Nil.
func queue(ctx context.Context, t *testing.T, e execer, chatID, automationID uuid.UUID) {
	t.Helper()
	queueRow(ctx, t, e, chatID, automationID, false)
}

// queueRow is queue for a message that can be under edit.
//
//nolint:revive // editing is the state of the row, not a mode of the helper.
func queueRow(ctx context.Context, t *testing.T, e execer, chatID, automationID uuid.UUID, editing bool) {
	t.Helper()
	var automation, input uuid.NullUUID
	if automationID != uuid.Nil {
		automation = uuid.NullUUID{UUID: automationID, Valid: true}
		input = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	}
	_, err := e.ExecContext(ctx, `
		INSERT INTO chat_queued_messages (chat_id, content, created_by, model_config_id, automation_id, input_id, editing_since)
		SELECT id, '[]', owner_id, last_model_config_id, $2, $3, CASE WHEN $4 THEN now() END FROM chats WHERE id = $1`,
		chatID, automation, input, editing)
	require.NoError(t, err)
}

// queued returns the automation of each queued message of the chat, in
// insertion order. uuid.Nil stands for an ordinary queued message.
func (f fixture) queued(ctx context.Context, t *testing.T, chatID uuid.UUID) []uuid.UUID {
	t.Helper()
	rows, err := f.sqlDB.QueryContext(ctx,
		`SELECT automation_id FROM chat_queued_messages WHERE chat_id = $1 ORDER BY id`, chatID)
	require.NoError(t, err)
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.NullUUID
		require.NoError(t, rows.Scan(&id))
		out = append(out, id.UUID)
	}
	require.NoError(t, rows.Err())
	return out
}

func (f fixture) automationExists(ctx context.Context, t *testing.T, id uuid.UUID) bool {
	t.Helper()
	_, err := f.db.GetChatAutomationByID(ctx, id)
	if xerrors.Is(err, sql.ErrNoRows) {
		return false
	}
	require.NoError(t, err)
	return true
}

func authorized(t *testing.T, db database.Store) database.Store {
	t.Helper()
	return dbauthz.New(db, rbac.NewAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), coderdtest.AccessControlStorePointer())
}

// roleActor holds one custom organization role with the given actions on chat
// automations and nothing else.
func roleActor(orgID uuid.UUID, actions ...policy.Action) rbac.Subject {
	return rbac.Subject{
		Type: rbac.SubjectTypeUser,
		ID:   uuid.NewString(),
		Roles: rbac.Roles{{
			Identifier: rbac.RoleIdentifier{Name: "automation-role", OrganizationID: orgID},
			ByOrgID: map[string]rbac.OrgPermissions{orgID.String(): {
				Org: rbac.Permissions(map[string][]policy.Action{rbac.ResourceChatAutomation.Type: actions}),
			}},
		}},
		Scope: rbac.ScopeAll,
	}
}

// published keeps what a Delete published.
type published struct {
	mu       sync.Mutex
	channels []string
	payloads [][]byte
}

func (p *published) Publish(channel string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.channels = append(p.channels, channel)
	p.payloads = append(p.payloads, payload)
	return nil
}

// on returns the payloads that were published on the channel.
func (p *published) on(channel string) [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out [][]byte
	for i, c := range p.channels {
		if c == channel {
			out = append(out, p.payloads[i])
		}
	}
	return out
}

// deleteAs deletes as a role that holds only read and delete on automations:
// the queue cleanup needs no permission on a chat.
func deleteAs(ctx context.Context, t *testing.T, db database.Store, ps chatstate.Publisher, orgID, id uuid.UUID) (database.ChatAutomation, error) {
	t.Helper()
	return chatautomation.Delete(ctx, testutil.Logger(t), authorized(t, db), ps,
		roleActor(orgID, policy.ActionRead, policy.ActionDelete), orgID, id)
}

// TestDelete deletes an automation and checks each chat. A queue is spelled
// head first: 'a' is a message of the deleted automation, 'o' of another
// automation and 'h' of a person. An upper-case letter puts the row under
// edit.
func TestDelete(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	type chatCase struct {
		status     database.ChatStatus
		archived   bool
		queue      string
		wantQueue  string
		wantStatus database.ChatStatus
	}
	for _, tc := range []struct {
		name  string
		chats []chatCase
	}{
		{name: "NoChat"},
		{name: "OtherQueuedMessages", chats: []chatCase{
			{status: database.ChatStatusRunning, queue: "ho", wantQueue: "ho", wantStatus: database.ChatStatusRunning},
		}},
		{name: "TwoChats", chats: []chatCase{
			{status: database.ChatStatusRunning, queue: "aoha", wantQueue: "oh", wantStatus: database.ChatStatusRunning},
			{status: database.ChatStatusError, queue: "ha", wantQueue: "h", wantStatus: database.ChatStatusError},
			{status: database.ChatStatusRunning, queue: "h", wantQueue: "h", wantStatus: database.ChatStatusRunning},
		}},
		{name: "ArchivedChat", chats: []chatCase{
			{status: database.ChatStatusError, archived: true, queue: "a", wantStatus: database.ChatStatusError},
		}},
		// The blocked head goes, so the chat leaves paused: the next
		// message is promoted and the chat runs.
		{name: "PausedChat", chats: []chatCase{
			{status: database.ChatStatusPaused, queue: "Ah", wantStatus: database.ChatStatusRunning},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newFixture(t, db, sqlDB)
			automation, other := f.automation(t, f.chat(t)), f.automation(t, f.chat(t))
			sources := map[rune]uuid.UUID{'a': automation.ID, 'o': other.ID, 'h': uuid.Nil}
			kinds := map[uuid.UUID]rune{automation.ID: 'a', other.ID: 'o', uuid.Nil: 'h'}

			chats := make([]database.Chat, len(tc.chats))
			for i, c := range tc.chats {
				chats[i] = dbgen.Chat(t, db, database.Chat{
					OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: f.model.ID, Status: c.status,
				})
				_, err := sqlDB.ExecContext(ctx, `UPDATE chats SET archived = $2 WHERE id = $1`, chats[i].ID, c.archived)
				require.NoError(t, err)
				for _, kind := range c.queue {
					queueRow(ctx, t, sqlDB, chats[i].ID, sources[unicode.ToLower(kind)], unicode.IsUpper(kind))
				}
			}
			ps := &published{}

			deleted, err := deleteAs(ctx, t, db, ps, f.org.ID, automation.ID)

			require.NoError(t, err)
			require.Equal(t, automation.ID, deleted.ID)
			require.False(t, f.automationExists(ctx, t, automation.ID))
			require.True(t, f.automationExists(ctx, t, other.ID))
			var resumed []uuid.UUID
			for i, c := range tc.chats {
				var left strings.Builder
				for _, id := range f.queued(ctx, t, chats[i].ID) {
					left.WriteRune(kinds[id])
				}
				require.Equal(t, c.wantQueue, left.String(), "queue of chat %d", i)
				chat, err := db.GetChatByID(ctx, chats[i].ID)
				require.NoError(t, err)
				require.Equal(t, c.wantStatus, chat.Status, "status of chat %d", i)
				require.Equal(t, c.archived, chat.Archived, "archived flag of chat %d", i)

				// Only a chat that held a message of the automation changes.
				updates := 0
				if strings.ContainsAny(c.queue, "aA") {
					updates = 1
				}
				require.Len(t, ps.on(coderdpubsub.ChatStateUpdateChannel(chat.ID)), updates, "updates of chat %d", i)
				if c.wantStatus != c.status {
					resumed = append(resumed, chat.ID)
				}
			}
			// The chat list hears of each chat that left paused.
			var watched []uuid.UUID
			for _, payload := range ps.on(coderdpubsub.ChatWatchEventChannel(f.owner.ID)) {
				var event codersdk.ChatWatchEvent
				require.NoError(t, json.Unmarshal(payload, &event))
				require.Equal(t, codersdk.ChatWatchEventKindStatusChange, event.Kind)
				require.Equal(t, codersdk.ChatStatusRunning, event.Chat.Status)
				watched = append(watched, event.Chat.ID)
			}
			require.Equal(t, resumed, watched)
		})
	}
}

// TestDeleteInvalidChat queues a message of the automation in a waiting chat,
// which is an invalid state. The cleanup refuses that chat, so the delete
// fails and changes nothing.
func TestDeleteInvalidChat(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	valid := f.chat(t)
	invalid := dbgen.Chat(t, db, database.Chat{OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: f.model.ID})
	automation := f.automation(t, valid)
	queue(ctx, t, sqlDB, valid.ID, automation.ID)
	queue(ctx, t, sqlDB, invalid.ID, automation.ID)
	ps := &published{}

	_, err := deleteAs(ctx, t, db, ps, f.org.ID, automation.ID)

	require.ErrorIs(t, err, chatstate.ErrInvalidState)
	require.ErrorContains(t, err, invalid.ID.String())
	require.False(t, httpapi.Is404Error(err))
	require.True(t, f.automationExists(ctx, t, automation.ID))
	require.Equal(t, []uuid.UUID{automation.ID}, f.queued(ctx, t, valid.ID))
	require.Equal(t, []uuid.UUID{automation.ID}, f.queued(ctx, t, invalid.ID))
	require.Empty(t, ps.channels)
}

// TestDeleteQueuedMessagesAfterOwnerDelete runs the steps of a user delete:
// the automation locks, the soft delete, whose trigger removes the
// automations, and then the queue cleanup by the IDs that were read first.
func TestDeleteQueuedMessagesAfterOwnerDelete(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	running := f.chat(t)
	paused := dbgen.Chat(t, db, database.Chat{
		OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: f.model.ID, Status: database.ChatStatusPaused,
	})
	first, second := f.automation(t, running), f.automation(t, running)
	queue(ctx, t, sqlDB, running.ID, first.ID)
	queue(ctx, t, sqlDB, running.ID, uuid.Nil)
	queue(ctx, t, sqlDB, running.ID, second.ID)
	queueRow(ctx, t, sqlDB, paused.ID, second.ID, true)
	ps := &published{}

	var publish func()
	err := db.InTx(func(tx database.Store) error {
		ids, err := chatautomation.LockByOwner(ctx, tx, f.owner.ID)
		if err != nil {
			return err
		}
		want := []uuid.UUID{first.ID, second.ID}
		slices.SortFunc(want, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		require.Equal(t, want, ids, "the locks are taken in automation ID order")
		if err := tx.UpdateUserDeletedByID(ctx, f.owner.ID); err != nil {
			return err
		}
		left, err := tx.GetChatAutomationIDsByOwnerID(ctx, f.owner.ID)
		require.NoError(t, err)
		require.Empty(t, left, "the trigger removed the automations")
		still, err := tx.GetChatQueuedMessages(ctx, running.ID)
		require.NoError(t, err)
		require.Len(t, still, 3, "their queued messages are still there")
		publish, err = chatautomation.DeleteQueuedMessages(ctx, testutil.Logger(t), tx, ps, ids)
		return err
	}, nil)
	require.NoError(t, err)
	publish()

	require.Equal(t, []uuid.UUID{uuid.Nil}, f.queued(ctx, t, running.ID))
	require.Empty(t, f.queued(ctx, t, paused.ID))
	chat, err := db.GetChatByID(ctx, paused.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, chat.Status, "the chat left paused")
	require.Len(t, ps.on(coderdpubsub.ChatStateUpdateChannel(running.ID)), 1)
	require.Len(t, ps.on(coderdpubsub.ChatStateUpdateChannel(paused.ID)), 1)
	require.Len(t, ps.on(coderdpubsub.ChatWatchEventChannel(f.owner.ID)), 1)
}

// goneStore removes the automation behind the back of the transaction right
// before its delete statement, as a soft delete of the owner by direct SQL
// can.
type goneStore struct {
	database.Store
	sqlDB *sql.DB
}

func (s goneStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error { return fn(goneStore{Store: tx, sqlDB: s.sqlDB}) }, opts)
}

func (s goneStore) DeleteChatAutomationByID(ctx context.Context, id uuid.UUID) (int64, error) {
	if _, err := s.sqlDB.ExecContext(ctx, `DELETE FROM chat_automations WHERE id = $1`, id); err != nil {
		return 0, err
	}
	return s.Store.DeleteChatAutomationByID(ctx, id)
}

// TestDeleteRowAlreadyGone deletes an automation whose row goes away between
// the read and the delete statement: it is not found, and its queue stays.
func TestDeleteRowAlreadyGone(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	chat := f.chat(t)
	automation := f.automation(t, chat)
	queue(ctx, t, sqlDB, chat.ID, automation.ID)
	ps := &published{}
	actor, _, err := httpmw.UserRBACSubject(ctx, db, f.owner.ID, rbac.ScopeAll)
	require.NoError(t, err)

	// The store is not authorized here: dbauthz reads the row again before
	// the delete statement and would report the missing row itself.
	_, err = chatautomation.Delete(ctx, testutil.Logger(t), goneStore{Store: db, sqlDB: sqlDB}, ps, actor, f.org.ID, automation.ID)

	require.True(t, httpapi.Is404Error(err), "want a not found error, got %v", err)
	require.Equal(t, []uuid.UUID{automation.ID}, f.queued(ctx, t, chat.ID))
	require.Empty(t, ps.channels)
}

// TestDeleteForbidden deletes as a role that can read the automation and not
// delete it.
func TestDeleteForbidden(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	chat := f.chat(t)
	automation := f.automation(t, chat)
	queue(ctx, t, sqlDB, chat.ID, automation.ID)

	_, err := chatautomation.Delete(ctx, testutil.Logger(t), authorized(t, db), &published{},
		roleActor(f.org.ID, policy.ActionRead), f.org.ID, automation.ID)

	require.ErrorIs(t, err, chatautomation.ErrDeleteForbidden)
	require.True(t, f.automationExists(ctx, t, automation.ID))
	require.Equal(t, []uuid.UUID{automation.ID}, f.queued(ctx, t, chat.ID))
}

// TestDeleteOtherOrganization deletes, through another organization, an
// automation that the actor can read: it is not found.
func TestDeleteOtherOrganization(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	automation := f.automation(t, f.chat(t))
	actor, _, err := httpmw.UserRBACSubject(ctx, db, f.owner.ID, rbac.ScopeAll)
	require.NoError(t, err)

	_, err = chatautomation.Delete(ctx, testutil.Logger(t), authorized(t, db), &published{}, actor, newFixture(t, db, sqlDB).org.ID, automation.ID)

	require.True(t, httpapi.Is404Error(err), "want a not found error, got %v", err)
	require.True(t, f.automationExists(ctx, t, automation.ID))
}

// TestDeleteLock holds the automation lock, as a run does, while it queues a
// message of the automation. Delete waits for the lock and reads the queue
// after it gets it, so it removes that message too.
func TestDeleteLock(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	chat := f.chat(t)
	automation := f.automation(t, chat)
	actor, _, err := httpmw.UserRBACSubject(ctx, db, f.owner.ID, rbac.ScopeAll)
	require.NoError(t, err)

	holder, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	var pid int
	require.NoError(t, holder.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, chatautomation.LockID(automation.ID))
	require.NoError(t, err)
	done, finished := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(finished)
		_, err := chatautomation.Delete(ctx, testutil.Logger(t), authorized(t, db), &published{}, actor, f.org.ID, automation.ID)
		done <- err
	}()
	// A failed assertion releases the lock, so Delete returns.
	t.Cleanup(func() {
		_ = holder.Rollback()
		<-finished
	})
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var n int
		err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND $1 = ANY(pg_blocking_pids(pid))`, pid).Scan(&n)
		return err == nil && n > 0
	}, testutil.IntervalFast, "Delete never waited for the automation lock")

	queue(ctx, t, holder, chat.ID, automation.ID)
	require.NoError(t, holder.Commit())

	require.NoError(t, testutil.RequireReceive(ctx, t, done))
	require.False(t, f.automationExists(ctx, t, automation.ID))
	require.Empty(t, f.queued(ctx, t, chat.ID))
}
