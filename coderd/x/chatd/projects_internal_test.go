package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestDeleteChatProjectStopsRunningChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	starter := newBlockingTaskStarter(false)
	chat := f.createRunningChat(t)
	project := dbgen.ChatProject(t, f.db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.user.ID})
	_, err := f.sqlDB.ExecContext(ctx, "UPDATE chats SET project_id = $2 WHERE id = $1", chat.ID, project.ID)
	require.NoError(t, err)
	worker := startWorker(t, testOptions(t, f, starter))
	generation := starter.waitCall(t, taskKindGeneration, chat.ID)

	deleted, err := newUnstartedServer(t, f.pubsub, f.db).DeleteChatProject(ctx, project.ID)
	require.NoError(t, err)
	require.Len(t, deleted, 1)

	// The chat:update stops the runner without waiting for renewal.
	select {
	case <-generation.ctx.Done():
	case <-ctx.Done():
		t.Fatal("the runner of a deleted chat must stop generating")
	}

	// A worker holding a candidate list read before the delete must not
	// acquire the chat.
	worker.mu.Lock()
	manager := worker.manager
	worker.mu.Unlock()
	acquired, err := worker.acquireCandidate(ctx, uuid.New(), manager, chat.ID)
	require.NoError(t, err)
	require.False(t, acquired)
}

// TestDeleteChatProjectSerializesWithChatCreation verifies that the delete
// waits for a sub-chat insert holding its root and archives the child, and
// that a root chat creation blocked on the project lock fails once the
// delete commits.
func TestDeleteChatProjectSerializesWithChatCreation(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	project := dbgen.ChatProject(t, f.db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.user.ID})
	root := dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
	})

	childTx, err := f.sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = childTx.Rollback() }()
	_, err = childTx.ExecContext(ctx, "SELECT id FROM chats WHERE id = $1 FOR SHARE", root.ID)
	require.NoError(t, err)
	childID := uuid.New()
	_, err = childTx.ExecContext(ctx, `INSERT INTO chats (id, organization_id, owner_id, last_model_config_id, title, parent_chat_id, root_chat_id)
		VALUES ($1, $2, $3, $4, 'child', $5, $5)`, childID, f.org.ID, f.user.ID, f.model.ID, root.ID)
	require.NoError(t, err)

	type deleteResult struct {
		chats []database.Chat
		err   error
	}
	deleteDone := make(chan deleteResult, 1)
	go func() {
		chats, err := chatstate.DeleteChatProject(ctx, f.db, f.pubsub, project.ID)
		deleteDone <- deleteResult{chats: chats, err: err}
	}()
	waitForLockWait(ctx, t, f.sqlDB, "LockChatProjectRootChats", 1)

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")})
	require.NoError(t, err)
	createErr := make(chan error, 1)
	go func() {
		_, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
			OrganizationID:    f.org.ID,
			OwnerID:           f.user.ID,
			ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
			LastModelConfigID: f.model.ID,
			Title:             "root",
			ClientType:        database.ChatClientTypeApi,
			InitialStatus:     database.ChatStatusWaiting,
			InitialMessages:   []chatstate.Message{userMessage(content, f.model.ID, f.user.ID, nil)},
		})
		createErr <- err
	}()
	waitForLockWait(ctx, t, f.sqlDB, "GetChatProjectByIDForShare", 1)

	require.NoError(t, childTx.Commit())
	result := testutil.TryReceive(ctx, t, deleteDone)
	require.NoError(t, result.err)
	ids := make([]uuid.UUID, 0, len(result.chats))
	for _, chat := range result.chats {
		ids = append(ids, chat.ID)
	}
	require.ElementsMatch(t, []uuid.UUID{root.ID, childID}, ids)
	child, err := f.db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.True(t, child.Archived, "a child committed while the delete waited is archived")
	require.ErrorIs(t, testutil.TryReceive(ctx, t, createErr), chatstate.ErrChatProjectNotFound)
}

func TestDeleteChatProjectConcurrentDeletes(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	project := dbgen.ChatProject(t, f.db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.user.ID})

	holdTx, err := f.sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holdTx.Rollback() }()
	_, err = holdTx.ExecContext(ctx, "SELECT id FROM chat_projects WHERE id = $1 FOR UPDATE", project.ID)
	require.NoError(t, err)

	const deletes = 2
	errs := make(chan error, deletes)
	for range deletes {
		go func() {
			_, err := chatstate.DeleteChatProject(ctx, f.db, f.pubsub, project.ID)
			errs <- err
		}()
	}
	waitForLockWait(ctx, t, f.sqlDB, "GetChatProjectByIDForUpdate", deletes)
	require.NoError(t, holdTx.Rollback())

	var succeeded, notFound int
	for range deletes {
		err := testutil.TryReceive(ctx, t, errs)
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, sql.ErrNoRows):
			notFound++
		default:
			t.Fatalf("unexpected delete error: %v", err)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, deletes-1, notFound)
}

// waitForLockWait waits until n sessions are blocked on a row lock while
// running the named query.
func waitForLockWait(ctx context.Context, t *testing.T, sqlDB *sql.DB, queryName string, n int) {
	t.Helper()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var waiting int
		err := sqlDB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM pg_stat_activity
WHERE datname = current_database()
	AND wait_event_type = 'Lock'
	AND query LIKE '%-- name: '||$1||' %'
`, queryName).Scan(&waiting)
		return err == nil && waiting == n
	}, testutil.IntervalFast, "wait for "+queryName+" to block")
	require.NoError(t, ctx.Err())
}
