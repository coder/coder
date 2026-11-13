package chatautomation_test

import (
	"context"
	"database/sql"
	"testing"

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

// execer is a *sql.DB or a *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// queue inserts a queued message of automationID into the chat, or an
// ordinary queued message when automationID is uuid.Nil.
func queue(ctx context.Context, t *testing.T, e execer, chatID, automationID uuid.UUID) {
	t.Helper()
	var automation, input uuid.NullUUID
	if automationID != uuid.Nil {
		automation = uuid.NullUUID{UUID: automationID, Valid: true}
		input = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	}
	_, err := e.ExecContext(ctx, `
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

// TestDelete deletes as a role that holds only read and delete on automations.
// A queued message of the automation makes the delete refuse. An ordinary
// queued message and a message of another automation do not.
func TestDelete(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	for _, tc := range []struct {
		name    string
		seed    func(ctx context.Context, t *testing.T, chat database.Chat, automation, other uuid.UUID)
		refused bool
	}{
		{name: "NoQueuedMessage"},
		{
			name: "OtherQueuedMessages",
			seed: func(ctx context.Context, t *testing.T, chat database.Chat, _, other uuid.UUID) {
				queue(ctx, t, sqlDB, chat.ID, uuid.Nil)
				queue(ctx, t, sqlDB, chat.ID, other)
			},
		},
		{
			name: "QueuedMessageOfTheAutomation",
			seed: func(ctx context.Context, t *testing.T, chat database.Chat, automation, other uuid.UUID) {
				queue(ctx, t, sqlDB, chat.ID, other)
				queue(ctx, t, sqlDB, chat.ID, automation)
			},
			refused: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newFixture(t, db, sqlDB)
			chat := f.chat(t)
			automation, other := f.automation(t, chat), f.automation(t, chat)
			if tc.seed != nil {
				tc.seed(ctx, t, chat, automation.ID, other.ID)
			}
			before := f.queued(ctx, t, chat.ID)

			deleted, err := chatautomation.Delete(ctx, authorized(t, db),
				roleActor(f.org.ID, policy.ActionRead, policy.ActionDelete), f.org.ID, automation.ID)

			require.True(t, f.automationExists(ctx, t, other.ID))
			require.Equal(t, before, f.queued(ctx, t, chat.ID))
			if tc.refused {
				require.ErrorIs(t, err, chatautomation.ErrQueuedMessages)
				require.False(t, httpapi.Is404Error(err))
				require.True(t, f.automationExists(ctx, t, automation.ID))
				return
			}
			require.NoError(t, err)
			require.Equal(t, automation.ID, deleted.ID)
			require.False(t, f.automationExists(ctx, t, automation.ID))
		})
	}
}

// TestDeleteLock holds the automation lock, as a run does, while it queues a
// message of the automation. Delete waits for the lock and reads the queue
// after it gets it, so it refuses.
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
		_, err := chatautomation.Delete(ctx, authorized(t, db), actor, f.org.ID, automation.ID)
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

	require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), chatautomation.ErrQueuedMessages)
	require.True(t, f.automationExists(ctx, t, automation.ID))
}
