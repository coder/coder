package chatd

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/codersdk"
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

		db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(f func(database.Store) error, _ *database.TxOptions) error { return f(db) })
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot, nil)
		// The guarded agent-scoped stamp, not an unconditional SetChatContextSnapshot,
		// so a concurrent push that already hydrated the chat wins.
		db.EXPECT().HydrateAgentChatsContext(gomock.Any(), database.HydrateAgentChatsContextParams{
			AgentID:       agentID,
			AggregateHash: snapshot.AggregateHash,
			ContextError:  snapshot.SnapshotError,
		}).Return([]uuid.UUID{chat.ID}, nil)

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
		db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(f func(database.Store) error, _ *database.TxOptions) error { return f(db) })
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows)

		server.hydrateChatContextOnCreate(ctx, database.Chat{
			ID:      uuid.New(),
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
		})
	})
}

func TestAdoptWorkspaceChatsContextLogsSelectionError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		agents    []database.WorkspaceAgent
		wantError error
	}{
		{
			name:      "NoEligibleAgents",
			wantError: agentselect.ErrNoEligibleAgents,
		},
		{
			name: "MultipleChatAgents",
			agents: []database.WorkspaceAgent{
				{ID: uuid.New(), Name: "first-coderd-chat"},
				{ID: uuid.New(), Name: "second-coderd-chat"},
			},
			wantError: agentselect.ErrMultipleChatAgents,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			db := dbmock.NewMockStore(gomock.NewController(t))
			agentID := uuid.New()
			workspace := database.Workspace{ID: uuid.New()}
			db.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agentID).Return(workspace, nil)
			db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspace.ID).
				Return(tc.agents, nil)
			sink := testutil.NewFakeSink(t)
			server := &Server{logger: sink.Logger()}

			publish, err := server.AdoptWorkspaceChatsContext(ctx, db, agentID, []byte{0x01})
			require.NoError(t, err)
			publish()

			entries := sink.Entries()
			require.Len(t, entries, 1)
			require.Equal(t, slog.LevelDebug, entries[0].Level)
			require.Equal(t, workspace.ID.String(), sinkFieldValue(t, entries[0].Fields, "workspace_id"))
			require.Contains(t, sinkFieldValue(t, entries[0].Fields, "error"), tc.wantError.Error())
		})
	}
}

// TestUpdateAgentChatsContextPublishesContextChanges verifies one event per
// hydrated, dirtied, cleared, error-synced, or resource-synced chat.
func TestUpdateAgentChatsContextPublishesContextChanges(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ps := dbpubsub.NewInMemory()
	server := &Server{db: db, logger: slogtest.Make(t, nil), pubsub: ps}

	ownerID := uuid.New()
	agentID := uuid.New()
	hash := []byte{0x01}
	now := time.Now()

	hydratedChat := database.Chat{ID: uuid.New(), OwnerID: ownerID, ContextAggregateHash: hash}
	dirtiedChat := database.Chat{ID: uuid.New(), OwnerID: ownerID, ContextAggregateHash: []byte{0x99}}
	clearedChat := database.Chat{ID: uuid.New(), OwnerID: ownerID, ContextAggregateHash: hash}
	syncedChat := database.Chat{ID: uuid.New(), OwnerID: ownerID, ContextAggregateHash: hash}

	want := []uuid.UUID{hydratedChat.ID, dirtiedChat.ID, clearedChat.ID, syncedChat.ID}
	events := make(chan codersdk.ChatWatchEvent, len(want))
	cancelSub, err := ps.SubscribeWithErr(
		coderdpubsub.ChatWatchEventChannel(ownerID),
		coderdpubsub.HandleChatWatchEvent(func(_ context.Context, payload codersdk.ChatWatchEvent, err error) {
			require.NoError(t, err)
			events <- payload
		}),
	)
	require.NoError(t, err)
	defer cancelSub()

	db.EXPECT().HydrateAgentChatsContext(gomock.Any(), database.HydrateAgentChatsContextParams{
		AgentID:       agentID,
		AggregateHash: hash,
	}).Return([]uuid.UUID{hydratedChat.ID}, nil)
	db.EXPECT().MarkChatsContextDirtyByAgent(gomock.Any(), database.MarkChatsContextDirtyByAgentParams{
		AgentID:       agentID,
		AggregateHash: hash,
		DirtySince:    sql.NullTime{Time: now, Valid: true},
	}).Return([]database.MarkChatsContextDirtyByAgentRow{{ID: dirtiedChat.ID, OwnerID: ownerID}}, nil)
	db.EXPECT().ClearChatsContextDirtyByAgent(gomock.Any(), database.ClearChatsContextDirtyByAgentParams{
		AgentID:       agentID,
		AggregateHash: hash,
	}).Return([]uuid.UUID{clearedChat.ID}, nil)
	db.EXPECT().SyncAgentChatsContextError(gomock.Any(), database.SyncAgentChatsContextErrorParams{
		AgentID: agentID,
	}).Return([]uuid.UUID{syncedChat.ID, dirtiedChat.ID}, nil)
	// Live sync can overlap either dirty transition, with one event per chat.
	db.EXPECT().SyncAgentChatsContextLiveResources(gomock.Any(), agentID).
		Return([]uuid.UUID{syncedChat.ID, dirtiedChat.ID, clearedChat.ID}, nil)
	db.EXPECT().GetChatByID(gomock.Any(), hydratedChat.ID).Return(hydratedChat, nil)
	db.EXPECT().GetChatByID(gomock.Any(), dirtiedChat.ID).Return(dirtiedChat, nil)
	db.EXPECT().GetChatByID(gomock.Any(), clearedChat.ID).Return(clearedChat, nil)
	db.EXPECT().GetChatByID(gomock.Any(), syncedChat.ID).Return(syncedChat, nil)

	publish, err := server.UpdateAgentChatsContext(ctx, db, agentID, hash, "", now)
	require.NoError(t, err)
	publish()

	gotChatIDs := make([]uuid.UUID, 0, len(want))
	for range want {
		event := testutil.RequireReceive(ctx, t, events)
		require.Equal(t, codersdk.ChatWatchEventKindContextDirty, event.Kind)
		gotChatIDs = append(gotChatIDs, event.Chat.ID)
	}
	require.ElementsMatch(t, want, gotChatIDs)
}

// TestEnsureChatContextPinnedOnFirstTurn covers the lazy-bind pinning path. An
// API-created chat carries no agent at create, binds its agent on the first
// turn, and must pin the agent's already-pushed snapshot then. This is the
// mechanism that lets a workspace created mid-turn have its context pinned on
// the next turn: the agent pushes its snapshot before the chat is bound to it,
// so HydrateAgentChatsContext on that push cannot reach the chat, and the
// rebind-only binding does not pin a first-time agent.
func TestEnsureChatContextPinnedOnFirstTurn(t *testing.T) {
	t.Parallel()

	t.Run("PinsWhenUnpinnedAndSnapshotExists", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		ps := dbpubsub.NewInMemory()
		server := &Server{db: db, logger: slogtest.Make(t, nil), pubsub: ps}

		ownerID := uuid.New()
		agentID := uuid.New()
		chat := database.Chat{ID: uuid.New(), OwnerID: ownerID, AgentID: uuid.NullUUID{UUID: agentID, Valid: true}}
		// A second unpinned chat bound to the same agent is hydrated by the
		// same statement and must get its own watch event.
		siblingChat := database.Chat{ID: uuid.New(), OwnerID: ownerID, AgentID: chat.AgentID}
		snapshot := database.WorkspaceAgentContextSnapshot{
			WorkspaceAgentID: agentID,
			AggregateHash:    []byte{0x0a, 0x0b},
		}
		pinnedChat := chat
		pinnedChat.ContextAggregateHash = snapshot.AggregateHash
		pinnedSibling := siblingChat
		pinnedSibling.ContextAggregateHash = snapshot.AggregateHash

		events := make(chan codersdk.ChatWatchEvent, 2)
		cancelSub, err := ps.SubscribeWithErr(
			coderdpubsub.ChatWatchEventChannel(ownerID),
			coderdpubsub.HandleChatWatchEvent(func(_ context.Context, payload codersdk.ChatWatchEvent, err error) {
				require.NoError(t, err)
				events <- payload
			}),
		)
		require.NoError(t, err)
		defer cancelSub()

		db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(f func(database.Store) error, _ *database.TxOptions) error { return f(db) })
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(snapshot, nil)
		// The guarded agent-scoped stamp, not an unconditional SetChatContextSnapshot,
		// so a concurrent push that already hydrated the chat wins.
		db.EXPECT().HydrateAgentChatsContext(gomock.Any(), database.HydrateAgentChatsContextParams{
			AgentID:       agentID,
			AggregateHash: snapshot.AggregateHash,
			ContextError:  snapshot.SnapshotError,
		}).Return([]uuid.UUID{chat.ID, siblingChat.ID}, nil)
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(pinnedChat, nil)
		db.EXPECT().GetChatByID(gomock.Any(), siblingChat.ID).Return(pinnedSibling, nil)

		server.ensureChatContextPinnedOnFirstTurn(ctx, chat)

		// Watching clients cached both details without pinned resources, so
		// every hydrated chat must broadcast a context event.
		gotChatIDs := make([]uuid.UUID, 0, 2)
		for range 2 {
			event := testutil.RequireReceive(ctx, t, events)
			require.Equal(t, codersdk.ChatWatchEventKindContextDirty, event.Kind)
			gotChatIDs = append(gotChatIDs, event.Chat.ID)
		}
		require.ElementsMatch(t, []uuid.UUID{chat.ID, siblingChat.ID}, gotChatIDs)
	})

	t.Run("SkipsPublishWhenNoSnapshot", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}

		agentID := uuid.New()
		// ErrNoRows means the agent has not pushed yet: nothing is stamped
		// and no event is published (GetChatByID has no EXPECT, so a
		// post-hydration read would fail the test).
		db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(f func(database.Store) error, _ *database.TxOptions) error { return f(db) })
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).
			Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows)

		server.ensureChatContextPinnedOnFirstTurn(ctx, database.Chat{
			ID:      uuid.New(),
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
		})
	})

	t.Run("SkipsWhenAlreadyPinned", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		// A non-NULL pinned hash means the chat is already pinned (or dirty
		// awaiting refresh); the hook must touch the database zero times so it
		// never clobbers existing bodies or a dirty chat's stale hash.
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}

		server.ensureChatContextPinnedOnFirstTurn(ctx, database.Chat{
			ID:                   uuid.New(),
			AgentID:              uuid.NullUUID{UUID: uuid.New(), Valid: true},
			ContextAggregateHash: []byte{0x01},
		})
	})

	t.Run("SkipsWhenAgentless", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		// No agent bound yet: the hook must touch the database zero times.
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}

		server.ensureChatContextPinnedOnFirstTurn(ctx, database.Chat{ID: uuid.New()})
	})
}
