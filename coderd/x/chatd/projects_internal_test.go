package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
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
	require.Len(t, deleted.Before, 1)

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

// chatUpdateFailingPubsub fails chat:update publishes and passes the rest
// through.
type chatUpdateFailingPubsub struct {
	dbpubsub.Pubsub
}

func (p chatUpdateFailingPubsub) Publish(channel string, payload []byte) error {
	if strings.HasPrefix(channel, "chat:update:") {
		return xerrors.New("publish disabled")
	}
	return p.Pubsub.Publish(channel, payload)
}

func TestDeleteChatProjectCommitsWhenPublishFails(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	project := dbgen.ChatProject(t, f.db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.user.ID})
	chat := dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
	})

	deleted, err := newUnstartedServer(t, chatUpdateFailingPubsub{Pubsub: f.pubsub}, f.db).DeleteChatProject(ctx, project.ID)
	require.NoError(t, err, "a committed delete succeeds even if chat:update fails")
	require.Len(t, deleted.After, 1)
	require.True(t, deleted.After[0].Archived)
	archived, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, archived.Archived)
}
