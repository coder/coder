package chatd

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

// TestHydrateChatContextOnCreate covers the create-time pinning path, which the
// end-to-end test cannot reach: chats there are inserted directly, bypassing
// CreateChat. It pins to the agent's latest snapshot via the NULL-guarded
// HydrateAgentChatsContext so a concurrent push is never clobbered, and is a
// best-effort no-op when there is no agent or no snapshot.
func TestHydrateChatContextOnCreate(t *testing.T) {
	t.Parallel()

	t.Run("PinsWhenSnapshotExists", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}

		agentID := uuid.New()
		chat := database.Chat{ID: uuid.New(), AgentID: uuid.NullUUID{UUID: agentID, Valid: true}}
		snapshot := database.WorkspaceAgentContextSnapshot{
			WorkspaceAgentID: agentID,
			AggregateHash:    []byte{0x0a, 0x0b},
			SnapshotError:    "one source failed",
		}

		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot, nil)
		// The guarded agent-scoped stamp, not an unconditional SetChatContextSnapshot,
		// so a concurrent push that already hydrated the chat wins.
		db.EXPECT().HydrateAgentChatsContext(gomock.Any(), database.HydrateAgentChatsContextParams{
			AgentID:       agentID,
			AggregateHash: snapshot.AggregateHash,
			ContextError:  snapshot.SnapshotError,
		}).Return(nil)

		server.hydrateChatContextOnCreate(ctx, chat)
	})

	t.Run("SkipsWhenAgentless", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		// No EXPECT calls: a chat with no agent must touch the database zero times.
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}

		server.hydrateChatContextOnCreate(ctx, database.Chat{ID: uuid.New()})
	})

	t.Run("SkipsWhenNoSnapshot", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}

		agentID := uuid.New()
		// ErrNoRows means the agent has not pushed yet; no stamp is written
		// (HydrateAgentChatsContext has no EXPECT, so a call would fail the test).
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows)

		server.hydrateChatContextOnCreate(ctx, database.Chat{
			ID:      uuid.New(),
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
		})
	})
}
