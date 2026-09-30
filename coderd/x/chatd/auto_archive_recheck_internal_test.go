package chatd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// staleAutoArchiveCandidate returns the candidate row that archiveOnce
// would read for chatID, together with the cutoff it used. Tests change
// the chat after this read to model activity that lands before
// archiveCandidate runs.
func staleAutoArchiveCandidate(t *testing.T, f *workerTestFixture, now time.Time) (database.GetAutoArchiveInactiveChatCandidatesRow, time.Time) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	cutoff := dbtime.StartOfDay(now).Add(-90 * 24 * time.Hour)
	rows, err := f.db.GetAutoArchiveInactiveChatCandidates(ctx, database.GetAutoArchiveInactiveChatCandidatesParams{
		ArchiveCutoff: cutoff,
		LimitCount:    10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return rows[0], cutoff
}

// TestWorker_AutoArchiveRechecksCandidateActivity covers activity that
// lands between the unlocked candidate read and the archive. The chat
// must stay unarchived because its last message is newer than the cutoff.
func TestWorker_AutoArchiveRechecksCandidateActivity(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	insertArchiveMessage(t, f, chat.ID, now.Add(-100*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))
	worker := f.newArchiveWorker(t, nil, nil, nil)

	row, cutoff := staleAutoArchiveCandidate(t, f, now)
	// Fresh activity lands before archiveCandidate runs for this row.
	insertArchiveMessage(t, f, chat.ID, now)

	family, err := worker.archiveCandidate(ctx, row, cutoff)
	require.ErrorIs(t, err, errAutoArchiveCandidateIneligible)
	require.True(t, isExpectedAutoArchiveError(err))
	require.Empty(t, family)
	require.False(t, f.archived(t, chat.ID),
		"chat %s was auto-archived although its last message (%s) is newer than the cutoff (%s)",
		chat.ID, now, cutoff)
}

func TestWorker_AutoArchiveRechecksCandidatePin(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))
	worker := f.newArchiveWorker(t, nil, nil, nil)

	row, cutoff := staleAutoArchiveCandidate(t, f, now)
	f.setPinOrder(t, chat.ID, 1)

	_, err := worker.archiveCandidate(ctx, row, cutoff)
	require.ErrorIs(t, err, errAutoArchiveCandidateIneligible)
	require.False(t, f.archived(t, chat.ID), "chat pinned after the candidate read must not be auto-archived")
}

// TestWorker_AutoArchiveRechecksChildActivityUnderLock holds a child row
// lock while archiveCandidate runs, commits fresh child activity once the
// archive is blocked on that lock, and then releases it. The recheck runs
// after every family row is locked, so it must observe the new activity.
func TestWorker_AutoArchiveRechecksChildActivityUnderLock(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	root := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	child := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	f.linkChild(t, root.ID, child.ID)
	insertArchiveMessage(t, f, child.ID, now.Add(-100*24*time.Hour))
	// Stage a fresh child message as deleted so it does not count as
	// activity yet. Undeleting it under the child lock models a child
	// turn that commits a message while the archive waits.
	insertArchiveMessage(t, f, child.ID, now)
	_, err := f.sqlDB.ExecContext(ctx, "UPDATE chat_messages SET deleted = true WHERE chat_id = $1 AND created_at >= $2", child.ID, now)
	require.NoError(t, err)
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))
	worker := f.newArchiveWorker(t, nil, nil, nil)

	row, cutoff := staleAutoArchiveCandidate(t, f, now)

	conn, err := f.sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	lockTx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = lockTx.Rollback() }()
	var lockPID int
	require.NoError(t, lockTx.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&lockPID))
	_, err = lockTx.ExecContext(ctx, "SELECT id FROM chats WHERE id = $1 FOR UPDATE", child.ID)
	require.NoError(t, err)

	type result struct {
		family []autoArchivedChat
		err    error
	}
	done := make(chan result, 1)
	go func() {
		family, err := worker.archiveCandidate(ctx, row, cutoff)
		done <- result{family: family, err: err}
	}()

	// Wait until the archive transaction is blocked on the child lock.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var blocked bool
		err := f.sqlDB.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1::int = ANY(pg_blocking_pids(pid)))",
			lockPID,
		).Scan(&blocked)
		return err == nil && blocked
	}, testutil.IntervalFast, "archive transaction never blocked on the child row lock")

	_, err = lockTx.ExecContext(ctx, "UPDATE chat_messages SET deleted = false WHERE chat_id = $1", child.ID)
	require.NoError(t, err)
	require.NoError(t, lockTx.Commit())

	res := testutil.RequireReceive(ctx, t, done)
	require.ErrorIs(t, res.err, errAutoArchiveCandidateIneligible)
	require.Empty(t, res.family)
	require.False(t, f.archived(t, root.ID), "child activity committed under the child lock must keep the root alive")
	require.False(t, f.archived(t, child.ID))
}

// TestWorker_AutoArchiveRearchivesUnarchivedChatWithoutMessages pins the
// documented behavior that unarchiving alone is not activity: only
// messages count, so the next tick archives the chat again.
func TestWorker_AutoArchiveRearchivesUnarchivedChatWithoutMessages(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	chat := f.createArchiveCandidate(t, now.Add(-120*24*time.Hour))
	require.NoError(t, f.db.UpsertChatAutoArchiveDays(ctx, 90))
	worker := f.newArchiveWorker(t, nil, nil, nil)

	worker.archiveOnce(ctx, now)
	require.True(t, f.archived(t, chat.ID))

	_, err := chatstate.SetFamilyArchived(ctx, f.db, f.pubsub, chatstate.SetFamilyArchivedInput{
		RootID:   chat.ID,
		Archived: false,
	})
	require.NoError(t, err)
	require.False(t, f.archived(t, chat.ID))

	worker.archiveOnce(ctx, now)
	require.True(t, f.archived(t, chat.ID), "unarchive without a message must not count as activity")
}
