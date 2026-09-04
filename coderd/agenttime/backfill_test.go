package agenttime_test

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

type agentTimeFixture struct {
	user database.User
	org  database.Organization
	chat database.Chat
}

func TestLiveAgentTimeAccounting(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)

	day1 := time.Date(2025, 1, 1, 23, 30, 0, 0, time.UTC)
	day2 := time.Date(2025, 1, 2, 0, 30, 0, 0, time.UTC)
	ids := []int64{
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleSystem, database.ChatMessageVisibilityUser, day1, int64Ptr(100), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, day2, int64Ptr(200), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, day2, int64Ptr(300), true),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, day2, int64Ptr(400), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, day2, int64Ptr(0), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, day2, nil, false),
	}

	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1), 100)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 2), 900)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, 5)

	err := db.SoftDeleteChatMessageByID(ctx, ids[2])
	require.NoError(t, err)
	_, err = db.ArchiveChatByID(ctx, fixture.chat.ID)
	require.NoError(t, err)

	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 1), 100)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 2), 900)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, 5)
}

func TestAgentTimeMultiRowInsertTrigger(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	rows, err := sqlDB.QueryContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, runtime_ms, created_at, compressed)
		VALUES
			($1, 'assistant'::chat_message_role, '[]'::jsonb, 1, 'both'::chat_message_visibility, 100, $2, false),
			($1, 'tool'::chat_message_role, '[]'::jsonb, 1, 'model'::chat_message_visibility, 200, $3, true),
			($1, 'user'::chat_message_role, '[]'::jsonb, 1, 'user'::chat_message_visibility, NULL, $3, false)
		RETURNING id
	`, fixture.chat.ID, time.Date(2025, 1, 5, 23, 59, 0, 0, time.UTC), time.Date(2025, 1, 6, 0, 1, 0, 0, time.UTC))
	require.NoError(t, err)
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.Len(t, ids, 3)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 5), 100)
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 6), 200)
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, 2)
}

func TestAgentTimeAccountingRollsBackWithMessageInsert(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, runtime_ms, created_at)
		VALUES ($1, 'assistant'::chat_message_role, '[]'::jsonb, 1, 'both'::chat_message_visibility, 100, $2)
	`, fixture.chat.ID, time.Date(2025, 1, 7, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	require.EqualValues(t, 0, countChatMessages(ctx, t, sqlDB, fixture.chat.ID))
	requireNoDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 7))
}

func TestAgentTimeConcurrentAccountingClaimsOnce(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	ids := []int64{
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC), int64Ptr(100), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, time.Date(2025, 1, 3, 13, 0, 0, 0, time.UTC), int64Ptr(200), false),
		insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityModel, time.Date(2025, 1, 3, 14, 0, 0, 0, time.UTC), int64Ptr(300), false),
	}
	clearAgentTimeAccounting(ctx, t, sqlDB, ids)

	var claimedTotal atomic.Int64
	var eg errgroup.Group
	for range 8 {
		eg.Go(func() error {
			claimed, err := db.AccountAgentTimeMessages(ctx, ids)
			claimedTotal.Add(claimed)
			return err
		})
	}
	require.NoError(t, eg.Wait())
	require.EqualValues(t, len(ids), claimedTotal.Load())
	requireAgentTimeMarkerCount(ctx, t, sqlDB, ids, int64(len(ids)))
	requireDailyAgentTime(ctx, t, sqlDB, fixture.org.ID, fixture.user.ID, date(2025, 1, 3), 600)
}

func TestAgentTimeAggregateFailureRollsBackMessageInsert(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	_, err := sqlDB.ExecContext(ctx, `
		CREATE FUNCTION fail_agent_time_daily_write()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'agent time daily write failed';
		END;
		$$;
		CREATE TRIGGER trigger_fail_agent_time_daily_write
		BEFORE INSERT OR UPDATE ON agent_time_daily
		FOR EACH ROW
		EXECUTE FUNCTION fail_agent_time_daily_write();
	`)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, runtime_ms, created_at)
		VALUES ($1, 'assistant'::chat_message_role, '[]'::jsonb, 1, 'both'::chat_message_visibility, 100, $2)
	`, fixture.chat.ID, time.Date(2025, 1, 4, 12, 0, 0, 0, time.UTC))
	require.ErrorContains(t, err, "agent time daily write failed")
	require.EqualValues(t, 0, countChatMessages(ctx, t, sqlDB, fixture.chat.ID))
}

func TestAgentTimeAccountingDoesNotBumpChatState(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t, dbtestutil.WithDumpOnFailure())
	fixture := setupAgentTimeFixture(t, db)
	messageID := insertAgentTimeMessage(ctx, t, sqlDB, fixture.chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, time.Now().UTC(), int64Ptr(100), false)
	clearAgentTimeAccounting(ctx, t, sqlDB, []int64{messageID})
	var revisionBefore, revisionAfter int64
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT revision FROM chat_messages WHERE id=$1", messageID).Scan(&revisionBefore))

	_, err := sqlDB.ExecContext(ctx, `UPDATE chats SET generation_attempt = 7 WHERE id = $1`, fixture.chat.ID)
	require.NoError(t, err)
	before := readChatState(ctx, t, sqlDB, fixture.chat.ID)

	accounted, err := db.AccountAgentTimeMessages(ctx, []int64{messageID})
	require.NoError(t, err)
	require.EqualValues(t, 1, accounted)
	require.Equal(t, before, readChatState(ctx, t, sqlDB, fixture.chat.ID))
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT revision FROM chat_messages WHERE id=$1", messageID).Scan(&revisionAfter))
	require.Equal(t, revisionBefore, revisionAfter)
}

type chatState struct {
	SnapshotVersion   int64
	HistoryVersion    int64
	GenerationAttempt int64
}

func setupAgentTimeFixture(t testing.TB, db database.Store) agentTimeFixture {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
	})
	return agentTimeFixture{user: user, org: org, chat: chat}
}

func insertAgentTimeMessage(ctx context.Context, t testing.TB, db *sql.DB, chatID uuid.UUID, role database.ChatMessageRole, visibility database.ChatMessageVisibility, createdAt time.Time, runtimeMS *int64, compressed bool) int64 {
	t.Helper()

	var runtime any
	if runtimeMS != nil {
		runtime = *runtimeMS
	}
	var id int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO chat_messages (chat_id, role, content, content_version, visibility, runtime_ms, created_at, compressed)
		VALUES ($1, $2::chat_message_role, '[]'::jsonb, 1, $3::chat_message_visibility, $4, $5, $6)
		RETURNING id
	`, chatID, string(role), string(visibility), runtime, createdAt, compressed).Scan(&id)
	require.NoError(t, err)
	return id
}

func clearAgentTimeAccounting(ctx context.Context, t testing.TB, db *sql.DB, messageIDs []int64) {
	t.Helper()

	_, err := db.ExecContext(ctx, `DELETE FROM chat_message_agent_time_accounted WHERE message_id = ANY($1)`, pq.Array(messageIDs))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM agent_time_daily; DELETE FROM agent_time_organization_daily`)
	require.NoError(t, err)
}

func readChatState(ctx context.Context, t testing.TB, db *sql.DB, chatID uuid.UUID) chatState {
	t.Helper()

	var state chatState
	err := db.QueryRowContext(ctx, `SELECT snapshot_version, history_version, generation_attempt FROM chats WHERE id = $1`, chatID).Scan(&state.SnapshotVersion, &state.HistoryVersion, &state.GenerationAttempt)
	require.NoError(t, err)
	return state
}

func requireDailyAgentTime(ctx context.Context, t testing.TB, db *sql.DB, orgID uuid.UUID, userID uuid.UUID, day time.Time, expected int64) {
	t.Helper()

	var actual int64
	err := db.QueryRowContext(ctx, `
		SELECT agent_time_ms
		FROM agent_time_daily
		WHERE organization_id = $1 AND user_id = $2 AND day = $3
	`, orgID, userID, day).Scan(&actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	requireAgentTimeSummaryInvariant(ctx, t, db)
}

func requireNoDailyAgentTime(ctx context.Context, t testing.TB, db *sql.DB, orgID uuid.UUID, userID uuid.UUID, day time.Time) {
	t.Helper()

	var actual int64
	err := db.QueryRowContext(ctx, `
		SELECT agent_time_ms
		FROM agent_time_daily
		WHERE organization_id = $1 AND user_id = $2 AND day = $3
	`, orgID, userID, day).Scan(&actual)
	require.ErrorIs(t, err, sql.ErrNoRows)
	requireAgentTimeSummaryInvariant(ctx, t, db)
}

func requireAgentTimeMarkerCount(ctx context.Context, t testing.TB, db *sql.DB, messageIDs []int64, expected int64) {
	t.Helper()

	var actual int64
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_message_agent_time_accounted
		WHERE message_id = ANY($1)
	`, pq.Array(messageIDs)).Scan(&actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func countChatMessages(ctx context.Context, t testing.TB, db *sql.DB, chatID uuid.UUID) int64 {
	t.Helper()

	var count int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_messages WHERE chat_id = $1`, chatID).Scan(&count)
	require.NoError(t, err)
	return count
}

func int64Ptr(v int64) *int64 {
	return &v
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func requireAgentTimeSummaryInvariant(ctx context.Context, t testing.TB, db *sql.DB) {
	t.Helper()
	var mismatch bool
	err := db.QueryRowContext(ctx, `
 SELECT EXISTS (
  SELECT 1 FROM (
   SELECT organization_id,day,SUM(agent_time_ms) AS agent_time_ms
   FROM agent_time_daily GROUP BY 1,2
  ) canonical FULL JOIN agent_time_organization_daily overview USING (organization_id,day)
  WHERE canonical.agent_time_ms IS DISTINCT FROM overview.agent_time_ms
 )`).Scan(&mismatch)
	require.NoError(t, err)
	require.False(t, mismatch, "organization summaries must equal canonical user/day sums")
}
