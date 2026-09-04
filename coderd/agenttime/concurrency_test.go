package agenttime_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/agenttime"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentTimeSummaryFailureIsAtomic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	fixture := setupAgentTimeFixture(t, db)
	id := insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, date(2025, 1, 1), new(int64(100)), false)
	clearAgentTimeAccounting(ctx, t, sqlDB, []int64{id})
	isolateBackfillOrg(ctx, t, sqlDB, fixture.org.ID, 0)
	_, err := sqlDB.ExecContext(ctx, "DELETE FROM agent_time_backfill_status WHERE organization_id=$1", fixture.org.ID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `
 CREATE FUNCTION fail_agent_time_summary() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'summary unavailable'; END; $$;
 CREATE TRIGGER fail_agent_time_summary BEFORE INSERT OR UPDATE ON agent_time_organization_daily
 FOR EACH ROW EXECUTE FUNCTION fail_agent_time_summary();`)
	require.NoError(t, err)
	_, err = agenttime.RunOnce(ctx, db, 100)
	require.ErrorContains(t, err, "summary unavailable")
	status := readBackfillStatus(ctx, t, sqlDB, fixture.org.ID)
	require.Contains(t, status.LastError, "summary unavailable", "first-batch failures must survive status-discovery rollback")
	require.Zero(t, status.ProcessedMessages)
	requireNoDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1))
	requireAgentTimeMarkerCount(ctx, t, sqlDB, []int64{id}, 0)
	_, err = sqlDB.ExecContext(ctx, "DELETE FROM chats WHERE id=$1", fixture.chat.ID)
	require.ErrorContains(t, err, "summary unavailable")
	require.EqualValues(t, 1, countChatMessages(ctx, t, sqlDB, fixture.chat.ID))
	requireNoDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1))
	_, err = sqlDB.ExecContext(ctx, "DROP TRIGGER fail_agent_time_summary ON agent_time_organization_daily")
	require.NoError(t, err)
	_, err = agenttime.RunOnce(ctx, db, 100)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, "DELETE FROM chats WHERE id=$1", fixture.chat.ID)
	require.NoError(t, err)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1), 100)
}

func TestAgentTimeSubchatConcurrentSummaries(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	fixture := setupAgentTimeFixture(t, db)
	secondUser := dbgen.User(t, db, database.User{})
	subchat := dbgen.Chat(t, db, database.Chat{
		OrganizationID: fixture.org.ID, OwnerID: secondUser.ID,
		LastModelConfigID: fixture.chat.LastModelConfigID, ParentChatID: uuid.NullUUID{UUID: fixture.chat.ID, Valid: true}, RootChatID: uuid.NullUUID{UUID: fixture.chat.ID, Valid: true},
	})
	var group errgroup.Group
	for i := range 20 {
		chatID := fixture.chat.ID
		if i%2 == 1 {
			chatID = subchat.ID
		}
		first, second := date(2025, 1, 1), date(2025, 1, 2)
		if i%2 == 1 {
			first, second = second, first
		}
		group.Go(func() error {
			_, err := sqlDB.ExecContext(ctx, `
   INSERT INTO chat_messages (chat_id,role,content,content_version,visibility,runtime_ms,created_at)
   VALUES ($1,'assistant','[]',1,'model',5,$2),($1,'tool','[]',1,'model',5,$3)`, chatID, first, second)
			return err
		})
	}
	require.NoError(t, group.Wait())
	for _, userID := range []uuid.UUID{fixture.user.ID, secondUser.ID} {
		requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, userID, date(2025, 1, 1), 50)
		requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, userID, date(2025, 1, 2), 50)
	}
}

func TestAgentTimeOutOfOrderCommits(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	first, second := setupAgentTimeFixture(t, db), setupAgentTimeFixture(t, db)
	early, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer early.Rollback()
	late, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer late.Rollback()
	for _, row := range []struct {
		tx   *sql.Tx
		chat uuid.UUID
		day  time.Time
	}{{early, first.chat.ID, date(2025, 1, 1)}, {late, second.chat.ID, date(2025, 1, 2)}} {
		_, err = row.tx.ExecContext(ctx, `INSERT INTO chat_messages (chat_id,role,content,content_version,visibility,runtime_ms,created_at)
  VALUES ($1,'assistant','[]',1,'both',100,$2)`, row.chat, row.day)
		require.NoError(t, err)
	}
	require.NoError(t, late.Commit())
	requireDailyAgentTime(ctx, t, sqlDB, second.org.ID, second.user.ID, date(2025, 1, 2), 100)
	requireNoDailyAgentTime(ctx, t, sqlDB, first.org.ID, first.user.ID, date(2025, 1, 1))
	_, err = agenttime.RunOnce(ctx, db, 100)
	require.NoError(t, err)
	require.NoError(t, early.Commit())
	requireDailyAgentTime(ctx, t, sqlDB, first.org.ID, first.user.ID, date(2025, 1, 1), 100)
}

func TestAgentTimeConcurrentInsertBackfillAndLegacyPurge(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	fixture := setupAgentTimeFixture(t, db)
	id := insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, date(2025, 1, 1), new(int64(100)), false)
	clearAgentTimeAccounting(ctx, t, sqlDB, []int64{id})
	isolateBackfillOrg(ctx, t, sqlDB, fixture.org.ID, 0)
	_, err := sqlDB.ExecContext(ctx, "UPDATE chats SET archived=true,updated_at='2025-01-01' WHERE id=$1", fixture.chat.ID)
	require.NoError(t, err)
	live, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer live.Rollback()
	_, err = live.ExecContext(ctx, "SELECT id FROM chats WHERE id=$1 FOR UPDATE", fixture.chat.ID)
	require.NoError(t, err)
	_, err = live.ExecContext(ctx, `INSERT INTO chat_messages (chat_id,role,content,content_version,visibility,runtime_ms,created_at)
 VALUES ($1,'assistant','[]',1,'both',200,'2025-01-01')`, fixture.chat.ID)
	require.NoError(t, err)
	legacy, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer legacy.Close()
	var pid int
	require.NoError(t, legacy.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	deleted := make(chan error, 1)
	go func() {
		_, err := legacy.ExecContext(ctx, "DELETE FROM chats WHERE id=$1", fixture.chat.ID)
		deleted <- err
	}()
	require.Eventually(t, func() bool {
		var blocked bool
		err := sqlDB.QueryRowContext(ctx, "SELECT cardinality(pg_blocking_pids($1)) > 0", pid).Scan(&blocked)
		return err == nil && blocked
	}, testutil.WaitLong, testutil.IntervalFast, "legacy deletion must be waiting on the live chat lock")
	event, err := agenttime.RunOnce(ctx, db, 100)
	require.NoError(t, err)
	require.True(t, event.ResetCursor)
	count, err := db.DeleteOldChats(ctx, database.DeleteOldChatsParams{BeforeTime: date(2026, 1, 1), LimitCount: 100})
	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, live.Commit())
	select {
	case err := <-deleted:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.Zero(t, countChatMessages(ctx, t, sqlDB, fixture.chat.ID))
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1), 300)
}
