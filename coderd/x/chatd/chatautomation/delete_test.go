package chatautomation_test

import (
	"bytes"
	"context"
	"database/sql"
	"sync"
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
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
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

// TestDelete deletes as a custom role that holds only read and delete on
// automations, so the chat locks must not need update on the owner's chats.
func TestDelete(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
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
	for _, chat := range []database.Chat{chatA, chatB, chatC} {
		before[chat.ID] = f.snapshot(ctx, t, chat.ID)
	}
	actor := rbac.Subject{
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
	require.Error(t, rbac.NewAuthorizer(prometheus.NewRegistry()).Authorize(ctx, actor, policy.ActionUpdate, chatA.RBACObject()))
	pub := &recordingPublisher{}

	deleted, err := chatautomation.Delete(ctx, testutil.Logger(t), authorized(t, db), pub, actor, f.org.ID, automation.ID)
	require.NoError(t, err)
	require.Equal(t, automation.ID, deleted.ID)
	require.False(t, f.automationExists(ctx, t, automation.ID))
	require.True(t, f.automationExists(ctx, t, other.ID))
	require.Equal(t, []uuid.UUID{other.ID}, f.queued(ctx, t, chatA.ID))
	require.Equal(t, []uuid.UUID{uuid.Nil}, f.queued(ctx, t, chatB.ID))
	require.Equal(t, []uuid.UUID{other.ID}, f.queued(ctx, t, chatC.ID))
	// Only the chats that held a queued message of the automation change.
	require.Equal(t, before[chatA.ID]+1, f.snapshot(ctx, t, chatA.ID))
	require.Equal(t, before[chatB.ID]+1, f.snapshot(ctx, t, chatB.ID))
	require.Equal(t, before[chatC.ID], f.snapshot(ctx, t, chatC.ID))
	require.ElementsMatch(t, []string{
		coderdpubsub.ChatStateUpdateChannel(chatA.ID),
		coderdpubsub.ChatStateUpdateChannel(chatB.ID),
	}, pub.published())
}

// TestDeleteOtherOrganization deletes, through another organization, an
// automation that the actor can read: it is not found and nothing changes.
func TestDeleteOtherOrganization(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	chat := f.chat(t)
	automation := f.automation(t, chat)
	f.queue(ctx, t, chat.ID, automation.ID)
	before := f.snapshot(ctx, t, chat.ID)
	actor, _, err := httpmw.UserRBACSubject(ctx, db, f.owner.ID, rbac.ScopeAll)
	require.NoError(t, err)
	pub := &recordingPublisher{}

	_, err = chatautomation.Delete(ctx, testutil.Logger(t), authorized(t, db), pub, actor,
		newFixture(t, db, sqlDB).org.ID, automation.ID)
	require.True(t, httpapi.Is404Error(err), "want a not found error, got %v", err)
	require.True(t, f.automationExists(ctx, t, automation.ID))
	require.Equal(t, []uuid.UUID{automation.ID}, f.queued(ctx, t, chat.ID))
	require.Equal(t, before, f.snapshot(ctx, t, chat.ID))
	require.Empty(t, pub.published())
}

// TestDeleteLockOrder holds the lock of the chat with the larger ID while
// Delete runs, and checks the locks that Delete holds while it waits.
func TestDeleteLockOrder(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	f := newFixture(t, db, sqlDB)
	small, large := f.chat(t), f.chat(t)
	if bytes.Compare(small.ID[:], large.ID[:]) > 0 {
		small, large = large, small
	}
	automation := f.automation(t, small)
	f.queue(ctx, t, small.ID, automation.ID)
	f.queue(ctx, t, large.ID, automation.ID)
	beforeSmall, beforeLarge := f.snapshot(ctx, t, small.ID), f.snapshot(ctx, t, large.ID)
	actor, _, err := httpmw.UserRBACSubject(ctx, db, f.owner.ID, rbac.ScopeAll)
	require.NoError(t, err)
	logger := testutil.Logger(t)
	pub := &recordingPublisher{}

	holder, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	var pid int
	require.NoError(t, holder.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	_, err = holder.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, large.ID)
	require.NoError(t, err)
	done, finished := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(finished)
		_, err := chatautomation.Delete(ctx, logger, db, pub, actor, f.org.ID, automation.ID)
		done <- err
	}()
	// A failed assertion releases the chat lock, so Delete returns.
	t.Cleanup(func() {
		_ = holder.Rollback()
		<-finished
	})
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var n int
		err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND $1 = ANY(pg_blocking_pids(pid))`, pid).Scan(&n)
		return err == nil && n > 0
	}, testutil.IntervalFast, "Delete never waited for the chat lock")

	var acquired bool
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`,
		chatautomation.LockID(automation.ID)).Scan(&acquired))
	require.False(t, acquired, "Delete must hold the automation lock while it waits for a chat")
	_, err = sqlDB.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE NOWAIT`, small.ID)
	var pqErr *pq.Error
	require.ErrorAs(t, err, &pqErr, "Delete must lock the chat with the smaller ID first")
	require.Equal(t, "lock_not_available", pqErr.Code.Name())
	require.Empty(t, pub.published(), "Delete must publish only after the commit")

	// A chat transition that holds the lock may remove the queued message
	// itself. Delete holds no lock on it yet.
	_, err = holder.ExecContext(ctx,
		`DELETE FROM chat_queued_messages WHERE chat_id = $1 AND automation_id = $2`, large.ID, automation.ID)
	require.NoError(t, err)
	require.NoError(t, holder.Commit())
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
}
