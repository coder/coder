package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
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

	deleted, err := DeleteChatProjectWithoutEvents(ctx, f.db, project.ID)
	require.NoError(t, err)
	require.Len(t, deleted, 1)

	worker.mu.Lock()
	manager := worker.manager
	worker.mu.Unlock()
	require.NoError(t, manager.heartbeatOnce(ctx))
	select {
	case <-generation.ctx.Done():
	case <-ctx.Done():
		t.Fatal("the runner of a deleted chat must stop generating")
	}

	// A worker holding a candidate list read before the delete must not
	// acquire the chat.
	acquired, err := worker.acquireCandidate(ctx, uuid.New(), manager, chat.ID)
	require.NoError(t, err)
	require.False(t, acquired)
}
