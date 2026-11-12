package chatautomation_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/testutil"
)

// fixture is one organization with a member who owns automations and chats.
// Tests share a database and give each subtest its own fixture.
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

func (f fixture) chat(t *testing.T) database.Chat {
	t.Helper()
	return dbgen.Chat(t, f.db, database.Chat{OrganizationID: f.org.ID, OwnerID: f.owner.ID, LastModelConfigID: f.model.ID})
}

func (f fixture) automation(t *testing.T, target database.Chat) database.ChatAutomation {
	t.Helper()
	return dbgen.ChatAutomation(t, f.db, database.ChatAutomation{
		OrganizationID: f.org.ID,
		OwnerID:        f.owner.ID,
		TargetChatID:   uuid.NullUUID{UUID: target.ID, Valid: true},
	})
}

// queue inserts a queued message of automationID into the chat, or an
// ordinary queued message when automationID is uuid.Nil.
func (f fixture) queue(ctx context.Context, t *testing.T, chatID, automationID uuid.UUID) {
	t.Helper()
	var automation, input uuid.NullUUID
	if automationID != uuid.Nil {
		automation = uuid.NullUUID{UUID: automationID, Valid: true}
		input = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	}
	_, err := f.sqlDB.ExecContext(ctx, `
		INSERT INTO chat_queued_messages (chat_id, content, created_by, automation_id, input_id)
		SELECT id, '[]', owner_id, $2, $3 FROM chats WHERE id = $1`, chatID, automation, input)
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

func (f fixture) snapshot(ctx context.Context, t *testing.T, chatID uuid.UUID) int64 {
	t.Helper()
	chat, err := f.db.GetChatByID(ctx, chatID)
	require.NoError(t, err)
	return chat.SnapshotVersion
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

func subject(ctx context.Context, t *testing.T, db database.Store, userID uuid.UUID) rbac.Subject {
	t.Helper()
	actor, _, err := httpmw.UserRBACSubject(ctx, db, userID, rbac.ScopeAll)
	require.NoError(t, err)
	return actor
}

func authorized(t *testing.T, db database.Store) database.Store {
	t.Helper()
	return dbauthz.New(db, rbac.NewAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), coderdtest.AccessControlStorePointer())
}

// recordingPublisher records the channel of each published message.
type recordingPublisher struct {
	mu       sync.Mutex
	channels []string
}

func (p *recordingPublisher) Publish(channel string, _ []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.channels = append(p.channels, channel)
	return nil
}

func (p *recordingPublisher) published() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.channels...)
}

func TestDelete(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	for _, tc := range []struct {
		name string
		// actor returns the subject that deletes the owner's automation.
		actor func(ctx context.Context, t *testing.T, f fixture) rbac.Subject
		// noChatUpdate says that the actor cannot update the owner's chats,
		// so only the chatd context can lock them.
		noChatUpdate bool
	}{
		{name: "Owner", actor: func(ctx context.Context, t *testing.T, f fixture) rbac.Subject {
			return subject(ctx, t, f.db, f.owner.ID)
		}},
		{name: "OrganizationAdmin", actor: func(ctx context.Context, t *testing.T, f fixture) rbac.Subject {
			admin := dbgen.User(t, f.db, database.User{})
			dbgen.OrganizationMember(t, f.db, database.OrganizationMember{
				UserID: admin.ID, OrganizationID: f.org.ID, Roles: []string{rbac.RoleOrgAdmin()},
			})
			return subject(ctx, t, f.db, admin.ID)
		}},
		{name: "DeploymentOwner", actor: func(ctx context.Context, t *testing.T, f fixture) rbac.Subject {
			return subject(ctx, t, f.db, dbgen.User(t, f.db, database.User{RBACRoles: []string{rbac.RoleOwner().String()}}).ID)
		}},
		// The built-in administrator roles can also update the owner's chats,
		// so this role holds only the automation actions.
		{name: "AutomationAdminWithoutChatAccess", noChatUpdate: true, actor: func(_ context.Context, _ *testing.T, f fixture) rbac.Subject {
			return rbac.Subject{
				Type: rbac.SubjectTypeUser,
				ID:   uuid.NewString(),
				Roles: rbac.Roles{{
					Identifier: rbac.RoleIdentifier{Name: "automation-admin", OrganizationID: f.org.ID},
					ByOrgID: map[string]rbac.OrgPermissions{f.org.ID.String(): {
						Org: rbac.Permissions(map[string][]policy.Action{
							rbac.ResourceChatAutomation.Type: {policy.ActionRead, policy.ActionDelete},
						}),
					}},
				}},
				Scope: rbac.ScopeAll,
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newFixture(t, db, sqlDB)
			chatA, chatB, chatC := f.chat(t), f.chat(t), f.chat(t)
			automation := f.automation(t, chatA)
			other := f.automation(t, chatA)
			f.queue(ctx, t, chatA.ID, automation.ID)
			f.queue(ctx, t, chatA.ID, other.ID)
			f.queue(ctx, t, chatB.ID, automation.ID)
			f.queue(ctx, t, chatB.ID, uuid.Nil)
			f.queue(ctx, t, chatC.ID, other.ID)
			before := map[uuid.UUID]int64{}
			ps := pubsub.NewInMemory()
			updates := map[uuid.UUID]chan coderdpubsub.ChatStateUpdateMessage{}
			for _, chat := range []database.Chat{chatA, chatB, chatC} {
				before[chat.ID] = f.snapshot(ctx, t, chat.ID)
				ch := make(chan coderdpubsub.ChatStateUpdateMessage, 4)
				updates[chat.ID] = ch
				cancel, err := ps.Subscribe(coderdpubsub.ChatStateUpdateChannel(chat.ID), func(_ context.Context, msg []byte) {
					var update coderdpubsub.ChatStateUpdateMessage
					if json.Unmarshal(msg, &update) == nil {
						ch <- update
					}
				})
				require.NoError(t, err)
				t.Cleanup(cancel)
			}

			actor := tc.actor(ctx, t, f)
			if tc.noChatUpdate {
				err := rbac.NewAuthorizer(prometheus.NewRegistry()).Authorize(ctx, actor, policy.ActionUpdate, chatA.RBACObject())
				require.Error(t, err, "the actor must not be able to update the owner's chat")
			}
			deleted, err := chatautomation.Delete(ctx, testutil.Logger(t), authorized(t, db), ps, actor, f.org.ID, automation.ID)
			require.NoError(t, err)
			require.Equal(t, automation.ID, deleted.ID)
			require.Equal(t, automation.WebhookSecret, deleted.WebhookSecret)

			require.False(t, f.automationExists(ctx, t, automation.ID))
			require.True(t, f.automationExists(ctx, t, other.ID))
			require.Equal(t, []uuid.UUID{other.ID}, f.queued(ctx, t, chatA.ID))
			require.Equal(t, []uuid.UUID{uuid.Nil}, f.queued(ctx, t, chatB.ID))
			require.Equal(t, []uuid.UUID{other.ID}, f.queued(ctx, t, chatC.ID))

			for _, chat := range []database.Chat{chatA, chatB} {
				after := f.snapshot(ctx, t, chat.ID)
				require.Equal(t, before[chat.ID]+1, after, "snapshot_version of a chat with a queued message")
				update := testutil.RequireReceive(ctx, t, updates[chat.ID])
				require.Equal(t, after, update.SnapshotVersion)
			}
			require.Equal(t, before[chatC.ID], f.snapshot(ctx, t, chatC.ID))
			// MemoryPubsub delivers before Publish returns, so a message
			// for chat C would already be in the channel.
			require.Empty(t, updates[chatC.ID])
		})
	}
}

func TestDeleteNotFound(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	for _, tc := range []struct {
		name string
		// seed returns the actor, the automation to delete, and a chat that
		// holds a queued message of it.
		seed func(ctx context.Context, t *testing.T, f fixture) (actor, automationID, chatID uuid.UUID)
	}{
		{name: "Unknown", seed: func(ctx context.Context, t *testing.T, f fixture) (uuid.UUID, uuid.UUID, uuid.UUID) {
			chat, id := f.chat(t), uuid.New()
			f.queue(ctx, t, chat.ID, id)
			return f.owner.ID, id, chat.ID
		}},
		{name: "OtherOrganization", seed: func(ctx context.Context, t *testing.T, f fixture) (uuid.UUID, uuid.UUID, uuid.UUID) {
			otherOrg := newFixture(t, f.db, f.sqlDB)
			chat := otherOrg.chat(t)
			automation := otherOrg.automation(t, chat)
			otherOrg.queue(ctx, t, chat.ID, automation.ID)
			// The owner of the other organization's automation is also a
			// member of the organization in the request.
			dbgen.OrganizationMember(t, f.db, database.OrganizationMember{UserID: otherOrg.owner.ID, OrganizationID: f.org.ID})
			return otherOrg.owner.ID, automation.ID, chat.ID
		}},
		{name: "NotReadable", seed: func(ctx context.Context, t *testing.T, f fixture) (uuid.UUID, uuid.UUID, uuid.UUID) {
			chat := f.chat(t)
			automation := f.automation(t, chat)
			f.queue(ctx, t, chat.ID, automation.ID)
			member := dbgen.User(t, f.db, database.User{})
			dbgen.OrganizationMember(t, f.db, database.OrganizationMember{UserID: member.ID, OrganizationID: f.org.ID})
			return member.ID, automation.ID, chat.ID
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newFixture(t, db, sqlDB)
			actorID, id, chatID := tc.seed(ctx, t, f)
			existed := f.automationExists(ctx, t, id)
			before := f.snapshot(ctx, t, chatID)
			pub := &recordingPublisher{}

			_, err := chatautomation.Delete(ctx, testutil.Logger(t), authorized(t, db), pub,
				subject(ctx, t, db, actorID), f.org.ID, id)
			require.Error(t, err)
			require.True(t, httpapi.Is404Error(err), "want a not found error, got %v", err)
			require.Equal(t, existed, f.automationExists(ctx, t, id))
			require.Equal(t, []uuid.UUID{id}, f.queued(ctx, t, chatID))
			require.Equal(t, before, f.snapshot(ctx, t, chatID))
			require.Empty(t, pub.published())
		})
	}
}

func TestDeleteLockOrder(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	// lockChat opens a transaction on another connection that holds the
	// row lock of the chat. It returns the transaction and its backend.
	lockChat := func(ctx context.Context, t *testing.T, chatID uuid.UUID) (*sql.Tx, int) {
		t.Helper()
		tx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })
		var pid int
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
		var id uuid.UUID
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, chatID).Scan(&id))
		return tx, pid
	}
	// startDelete runs Delete on the raw store in a goroutine while holder
	// holds a chat lock. The cleanup releases holder and waits for Delete,
	// so a failed test leaves no goroutine behind.
	startDelete := func(ctx context.Context, t *testing.T, f fixture, pub *recordingPublisher, id uuid.UUID, holder *sql.Tx) <-chan error {
		t.Helper()
		actor := subject(ctx, t, db, f.owner.ID)
		logger := testutil.Logger(t)
		done := make(chan error, 1)
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			_, err := chatautomation.Delete(ctx, logger, db, pub, actor, f.org.ID, id)
			done <- err
		}()
		t.Cleanup(func() {
			_ = holder.Rollback()
			<-finished
		})
		return done
	}
	// waitBlocked waits until a backend waits for a lock that holder holds.
	waitBlocked := func(ctx context.Context, t *testing.T, holder int) {
		t.Helper()
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			var n int
			err := sqlDB.QueryRowContext(ctx, `
				SELECT count(*) FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock'
				  AND $1 = ANY(pg_blocking_pids(pid))`, holder).Scan(&n)
			return err == nil && n > 0
		}, testutil.IntervalFast, "Delete never waited for the chat lock")
	}
	requireRunning := func(t *testing.T, done <-chan error) {
		t.Helper()
		select {
		case err := <-done:
			t.Fatalf("Delete returned while a chat lock it needs was held: %v", err)
		default:
		}
	}

	t.Run("WaitsForTheChatLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		f := newFixture(t, db, sqlDB)
		chat := f.chat(t)
		automation := f.automation(t, chat)
		f.queue(ctx, t, chat.ID, automation.ID)
		pub := &recordingPublisher{}

		holder, pid := lockChat(ctx, t, chat.ID)
		done := startDelete(ctx, t, f, pub, automation.ID, holder)
		waitBlocked(ctx, t, pid)

		var acquired bool
		require.NoError(t, database.New(sqlDB).InTx(func(tx database.Store) error {
			var err error
			acquired, err = tx.TryAcquireLock(ctx, chatautomation.LockID(automation.ID))
			return err
		}, nil))
		require.False(t, acquired, "Delete must hold the automation lock while it waits for the chat")
		requireRunning(t, done)
		require.Empty(t, pub.published())

		// A chat transition that holds the lock may remove the queued
		// message itself.
		_, err := holder.ExecContext(ctx,
			`DELETE FROM chat_queued_messages WHERE chat_id = $1 AND automation_id = $2`, chat.ID, automation.ID)
		require.NoError(t, err)
		require.NoError(t, holder.Commit())

		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		require.False(t, f.automationExists(ctx, t, automation.ID))
		require.Empty(t, f.queued(ctx, t, chat.ID))
	})

	t.Run("LocksInChatIDOrder", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		f := newFixture(t, db, sqlDB)
		small, large := f.chat(t), f.chat(t)
		if bytes.Compare(small.ID[:], large.ID[:]) > 0 {
			small, large = large, small
		}
		automation := f.automation(t, small)
		f.queue(ctx, t, small.ID, automation.ID)
		f.queue(ctx, t, large.ID, automation.ID)
		beforeSmall, beforeLarge := f.snapshot(ctx, t, small.ID), f.snapshot(ctx, t, large.ID)
		pub := &recordingPublisher{}

		holder, pid := lockChat(ctx, t, large.ID)
		done := startDelete(ctx, t, f, pub, automation.ID, holder)
		waitBlocked(ctx, t, pid)

		probe, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		_, err = probe.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE NOWAIT`, small.ID)
		require.NoError(t, probe.Rollback())
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr, "Delete must lock the chat with the smaller ID first")
		require.Equal(t, "lock_not_available", pqErr.Code.Name())
		// The delete of the smaller chat's queued message is not committed.
		require.Equal(t, []uuid.UUID{automation.ID}, f.queued(ctx, t, small.ID))
		requireRunning(t, done)
		require.Empty(t, pub.published(), "Delete must publish only after the commit")

		require.NoError(t, holder.Rollback())
		require.NoError(t, testutil.RequireReceive(ctx, t, done))
		require.False(t, f.automationExists(ctx, t, automation.ID))
		require.Empty(t, f.queued(ctx, t, small.ID))
		require.Empty(t, f.queued(ctx, t, large.ID))
		require.Equal(t, beforeSmall+1, f.snapshot(ctx, t, small.ID))
		require.Equal(t, beforeLarge+1, f.snapshot(ctx, t, large.ID))
		require.Equal(t, []string{
			coderdpubsub.ChatStateUpdateChannel(small.ID),
			coderdpubsub.ChatStateUpdateChannel(large.ID),
		}, pub.published())
	})
}

var errInjected = xerrors.New("injected delete failure")

// failingStore fails the delete of the automation row, after the chat
// locks. InTx hands the wrapper to the callback so that the override
// reaches the transaction.
type failingStore struct {
	database.Store
	locks *atomic.Int32
}

func (s failingStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(failingStore{Store: tx, locks: s.locks})
	}, opts)
}

func (s failingStore) LockChatAndBumpSnapshotVersion(ctx context.Context, id uuid.UUID) (database.Chat, error) {
	s.locks.Add(1)
	return s.Store.LockChatAndBumpSnapshotVersion(ctx, id)
}

func (failingStore) DeleteChatAutomationByID(context.Context, uuid.UUID) error {
	return errInjected
}

func TestDeletePublishesNothingOnFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	chatA, chatB := f.chat(t), f.chat(t)
	automation := f.automation(t, chatA)
	f.queue(ctx, t, chatA.ID, automation.ID)
	f.queue(ctx, t, chatB.ID, automation.ID)
	beforeA, beforeB := f.snapshot(ctx, t, chatA.ID), f.snapshot(ctx, t, chatB.ID)
	pub := &recordingPublisher{}
	store := failingStore{Store: db, locks: &atomic.Int32{}}

	_, err := chatautomation.Delete(ctx, testutil.Logger(t), store, pub, subject(ctx, t, db, f.owner.ID), f.org.ID, automation.ID)
	require.ErrorIs(t, err, errInjected)
	require.EqualValues(t, 2, store.locks.Load(), "the failure must come after both chat locks")
	require.True(t, f.automationExists(ctx, t, automation.ID))
	require.Equal(t, []uuid.UUID{automation.ID}, f.queued(ctx, t, chatA.ID))
	require.Equal(t, []uuid.UUID{automation.ID}, f.queued(ctx, t, chatB.ID))
	require.Equal(t, beforeA, f.snapshot(ctx, t, chatA.ID))
	require.Equal(t, beforeB, f.snapshot(ctx, t, chatB.ID))
	require.Empty(t, pub.published())
}
