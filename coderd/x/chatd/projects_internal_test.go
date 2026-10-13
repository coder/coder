package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
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

	_, err = f.db.UpdateChatProjectDeletedByID(ctx, project.ID)
	require.NoError(t, err)
	running, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, running.RunnerID.Valid, "the tombstone alone leaves the chat running until the cleanup")
	newUnstartedServer(t, f.pubsub, f.db).ScheduleDeletedChatProjectCleanup(ctx, project.ID)

	// The cleanup's chat:update stops the runner without waiting for
	// renewal.
	select {
	case <-generation.ctx.Done():
	case <-ctx.Done():
		t.Fatal("the runner of a deleted chat must stop generating")
	}

	// A worker holding a candidate list read before the cleanup must not
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

func TestDeletedChatProjectCleanupSucceedsWhenPublishFails(t *testing.T) {
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
	_, err := f.db.UpdateChatProjectDeletedByID(ctx, project.ID)
	require.NoError(t, err)
	events := make(chan codersdk.ChatWatchEvent, 1)
	cancel, err := f.pubsub.SubscribeWithErr(coderdpubsub.ChatWatchEventChannel(f.user.ID), coderdpubsub.HandleChatWatchEvent(
		func(_ context.Context, event codersdk.ChatWatchEvent, err error) {
			if err == nil {
				events <- event
			}
		}))
	require.NoError(t, err)
	defer cancel()

	server := newUnstartedServer(t, chatUpdateFailingPubsub{Pubsub: f.pubsub}, f.db)
	require.NoError(t, server.cleanupDeletedChatProject(ctx, project.ID), "a committed cleanup succeeds even if chat:update fails")
	archived, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, archived.Archived)
	event := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, codersdk.ChatWatchEventKindProjectDeleted, event.Kind)
	require.Equal(t, chat.ID, event.Chat.ID)
}

func TestSweepDeletedChatProjects(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	newProjectChat := func() (database.ChatProject, database.Chat) {
		project := dbgen.ChatProject(t, f.db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.user.ID})
		chat := dbgen.Chat(t, f.db, database.Chat{
			OrganizationID:    f.org.ID,
			OwnerID:           f.user.ID,
			LastModelConfigID: f.model.ID,
			ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
		})
		return project, chat
	}
	deleted, deletedChat := newProjectChat()
	_, liveChat := newProjectChat()
	_, err := f.db.UpdateChatProjectDeletedByID(ctx, deleted.ID)
	require.NoError(t, err)

	worker := startWorker(t, testOptions(t, f, nil))
	worker.sweepDeletedChatProjects(ctx)

	chat, err := f.db.GetChatByID(ctx, deletedChat.ID)
	require.NoError(t, err)
	require.True(t, chat.Archived, "the sweep archives a deleted project's chats")
	chat, err = f.db.GetChatByID(ctx, liveChat.ID)
	require.NoError(t, err)
	require.False(t, chat.Archived, "the sweep leaves live projects alone")
	pending, err := f.db.GetDeletedChatProjectIDsWithUnarchivedChats(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, pending)
}
