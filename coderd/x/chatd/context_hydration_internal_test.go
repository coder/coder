package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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
		clock := quartz.NewMock(t)
		server := &Server{db: db, logger: slogtest.Make(t, nil), clock: clock}

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
		db.EXPECT().SettleChatsContextDrift(gomock.Any(), database.SettleChatsContextDriftParams{
			ChatIds:       []uuid.UUID{chat.ID},
			AgentID:       agentID,
			AggregateHash: snapshot.AggregateHash,
			ContextError:  snapshot.SnapshotError,
			DirtySince:    clock.Now(),
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

// TestSyncAgentChatsContextAddedResourcesStaysWithinTheChatCaps: snapshots
// that keep publishing new sources cannot grow a chat past its caps. New
// sources are admitted in source order; the rest wait for a refresh.
func TestSyncAgentChatsContextAddedResourcesStaysWithinTheChatCaps(t *testing.T) {
	t.Parallel()
	fix := newRebindFixture(t)
	chat := dbgen.Chat(t, fix.db, database.Chat{
		OwnerID:           fix.user.ID,
		OrganizationID:    fix.org.ID,
		LastModelConfigID: fix.model.ID,
		WorkspaceID:       uuid.NullUUID{UUID: fix.ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: fix.agentA, Valid: true},
		Status:            database.ChatStatusWaiting,
	})
	_, err := fix.db.HydrateAgentChatsContext(fix.ctx, database.HydrateAgentChatsContextParams{
		AgentID:       fix.agentA,
		AggregateHash: fix.hashA,
	})
	require.NoError(t, err)

	body := json.RawMessage(`{"instruction_file":{"content":"repo rules"}}`)
	publish := func(hash []byte, sources ...string) {
		for _, source := range sources {
			seedAgentContext(fix.ctx, t, fix.db, fix.agentA, source, hash, database.WorkspaceAgentContextBodyKindInstructionFile, body)
		}
	}
	add := func(hash []byte, maxResources, maxContentBytes int64) {
		_, err := fix.db.SyncAgentChatsContextAddedResources(fix.ctx, database.SyncAgentChatsContextAddedResourcesParams{
			AgentID:         fix.agentA,
			AggregateHash:   hash,
			MaxResources:    maxResources,
			MaxContentBytes: maxContentBytes,
		})
		require.NoError(t, err)
	}
	held := func() (sources []string, contentBytes int64) {
		rows, err := fix.db.ListChatContextResourcesByChatID(fix.ctx, chat.ID)
		require.NoError(t, err)
		for _, row := range rows {
			sources = append(sources, row.Source)
			contentBytes += row.SizeBytes
		}
		slices.Sort(sources)
		return sources, contentBytes
	}

	publish([]byte{0x02}, "/home/coder/r/c/AGENTS.md", "/home/coder/r/a/AGENTS.md", "/home/coder/r/b/AGENTS.md")
	add([]byte{0x02}, 3, 1<<20)
	sources, contentBytes := held()
	require.Equal(t, []string{"/home/coder/r/a/AGENTS.md", "/home/coder/r/b/AGENTS.md", fix.srcA}, sources, "the row cap admits the first new sources")

	publish([]byte{0x03}, "/home/coder/r/d/AGENTS.md")
	add([]byte{0x03}, 3, 1<<20)
	sources, _ = held()
	require.Len(t, sources, 3, "a later snapshot cannot grow a full chat")

	add([]byte{0x03}, 10, contentBytes+int64(len(body)))
	sources, _ = held()
	require.Equal(t, []string{"/home/coder/r/a/AGENTS.md", "/home/coder/r/b/AGENTS.md", "/home/coder/r/c/AGENTS.md", fix.srcA}, sources, "the content cap admits only what fits")
}

// TestHydrateAdoptsDiscoveredRows: a file chatd discovered before the
// agent's first push keeps the body the model read when the snapshot
// publishes the same source, and the chat is out of date if they differ.
func TestHydrateAdoptsDiscoveredRows(t *testing.T) {
	t.Parallel()
	fix := newRebindFixture(t)
	newChat := func() database.Chat {
		return dbgen.Chat(t, fix.db, database.Chat{
			OwnerID:           fix.user.ID,
			OrganizationID:    fix.org.ID,
			LastModelConfigID: fix.model.ID,
			WorkspaceID:       uuid.NullUUID{UUID: fix.ws.ID, Valid: true},
			AgentID:           uuid.NullUUID{UUID: fix.agentA, Valid: true},
			Status:            database.ChatStatusWaiting,
		})
	}
	stale, clean := newChat(), newChat()
	readBody := json.RawMessage(`{"instruction_file":{"content":"read before the push"}}`)
	require.NoError(t, fix.db.InsertChatContextDiscoveredResource(fix.ctx, database.InsertChatContextDiscoveredResourceParams{
		ChatID:      stale.ID,
		Source:      fix.srcA,
		BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:        readBody,
		ContentHash: []byte{0x01},
		SizeBytes:   int64(len(readBody)),
		Status:      database.WorkspaceAgentContextResourceStatusOk,
	}))
	server := &Server{db: fix.db, logger: slogtest.Make(t, nil), clock: quartz.NewMock(t)}

	hydrated, published, err := server.hydrateAgentChatsFromSnapshot(fix.ctx, fix.agentA)
	require.NoError(t, err)
	require.True(t, published)
	require.ElementsMatch(t, []uuid.UUID{stale.ID, clean.ID}, hydrated)

	rows, err := fix.db.ListChatContextResourcesByChatID(fix.ctx, stale.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.False(t, rows[0].Discovered, "the snapshot owns the adopted row")
	require.JSONEq(t, string(readBody), string(rows[0].Body), "adoption keeps the body the model read")
	got, err := fix.db.GetChatByID(fix.ctx, stale.ID)
	require.NoError(t, err)
	require.True(t, got.ContextDirtySince.Valid, "the adopted row differs from the snapshot")
	got, err = fix.db.GetChatByID(fix.ctx, clean.ID)
	require.NoError(t, err)
	require.False(t, got.ContextDirtySince.Valid)
	require.Equal(t, fix.hashA, got.ContextAggregateHash)
}

// TestHydrateAndMarkChatsDirtyPublishesForHydratedAndDirtied verifies one
// event per hydrated, added-to, dirtied, or MCP-synced chat.
func TestHydrateAndMarkChatsDirtyPublishesForHydratedAndDirtied(t *testing.T) {
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
	addedChat := database.Chat{ID: uuid.New(), OwnerID: ownerID, ContextAggregateHash: hash}
	dirtiedChat := database.Chat{ID: uuid.New(), OwnerID: ownerID, ContextAggregateHash: []byte{0x99}}
	syncedChat := database.Chat{ID: uuid.New(), OwnerID: ownerID, ContextAggregateHash: hash}

	events := make(chan codersdk.ChatWatchEvent, 4)
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
	// Return the dirtied chat from the additive sync too: a chat can gain
	// rows and still diverge on others.
	db.EXPECT().SyncAgentChatsContextAddedResources(gomock.Any(), database.SyncAgentChatsContextAddedResourcesParams{
		AgentID:         agentID,
		AggregateHash:   hash,
		MaxResources:    maxChatContextResources,
		MaxContentBytes: maxChatContextContentBytes,
	}).Return([]uuid.UUID{addedChat.ID, dirtiedChat.ID}, nil)
	db.EXPECT().SettleChatsContextDrift(gomock.Any(), database.SettleChatsContextDriftParams{
		ChatIds:       []uuid.UUID{hydratedChat.ID, addedChat.ID, dirtiedChat.ID},
		AgentID:       agentID,
		AggregateHash: hash,
		DirtySince:    now,
	}).Return(nil)
	db.EXPECT().MarkChatsContextDirtyByAgent(gomock.Any(), database.MarkChatsContextDirtyByAgentParams{
		AgentID:       agentID,
		AggregateHash: hash,
		DirtySince:    sql.NullTime{Time: now, Valid: true},
	}).Return([]database.MarkChatsContextDirtyByAgentRow{{ID: dirtiedChat.ID, OwnerID: ownerID}}, nil)
	// Return the dirtied chat from the sync to verify event deduplication.
	db.EXPECT().SyncAgentChatsContextMCPResources(gomock.Any(), agentID).
		Return([]uuid.UUID{syncedChat.ID, dirtiedChat.ID}, nil)
	db.EXPECT().GetChatByID(gomock.Any(), hydratedChat.ID).Return(hydratedChat, nil)
	db.EXPECT().GetChatByID(gomock.Any(), addedChat.ID).Return(addedChat, nil)
	db.EXPECT().GetChatByID(gomock.Any(), dirtiedChat.ID).Return(dirtiedChat, nil)
	db.EXPECT().GetChatByID(gomock.Any(), syncedChat.ID).Return(syncedChat, nil)

	publish, err := server.HydrateAndMarkChatsDirty(ctx, db, agentID, hash, "", now)
	require.NoError(t, err)
	publish()

	gotChatIDs := make([]uuid.UUID, 0, 4)
	for range 4 {
		event := testutil.RequireReceive(ctx, t, events)
		require.Equal(t, codersdk.ChatWatchEventKindContextDirty, event.Kind)
		gotChatIDs = append(gotChatIDs, event.Chat.ID)
	}
	require.ElementsMatch(t, []uuid.UUID{hydratedChat.ID, addedChat.ID, dirtiedChat.ID, syncedChat.ID}, gotChatIDs)
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
		clock := quartz.NewMock(t)
		server := &Server{db: db, logger: slogtest.Make(t, nil), pubsub: ps, clock: clock}

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
		db.EXPECT().SettleChatsContextDrift(gomock.Any(), database.SettleChatsContextDriftParams{
			ChatIds:       []uuid.UUID{chat.ID, siblingChat.ID},
			AgentID:       agentID,
			AggregateHash: snapshot.AggregateHash,
			DirtySince:    clock.Now(),
		}).Return(nil)
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(pinnedChat, nil)
		db.EXPECT().GetChatByID(gomock.Any(), siblingChat.ID).Return(pinnedSibling, nil)

		// The caller reads pinned context right after, so it must see the
		// pinned row rather than the pre-hydration one.
		require.Equal(t, pinnedChat, server.ensureChatContextPinnedOnFirstTurn(ctx, chat))

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

	t.Run("ReadsRowWhenPinnedConcurrently", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db, logger: slogtest.Make(t, nil), pubsub: dbpubsub.NewInMemory()}

		agentID := uuid.New()
		chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New(), AgentID: uuid.NullUUID{UUID: agentID, Valid: true}}
		snapshot := database.WorkspaceAgentContextSnapshot{WorkspaceAgentID: agentID, AggregateHash: []byte{0x0c}}
		pinnedChat := chat
		pinnedChat.ContextAggregateHash = snapshot.AggregateHash

		// A push or refresh pinned the chat after the caller read it, so
		// the guarded statement finds nothing to stamp; the row still has
		// to be read so the turn does not treat a published snapshot as
		// missing.
		db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
			func(f func(database.Store) error, _ *database.TxOptions) error { return f(db) })
		db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), agentID).Return(snapshot, nil)
		db.EXPECT().HydrateAgentChatsContext(gomock.Any(), gomock.Any()).Return(nil, nil)
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(pinnedChat, nil)

		require.Equal(t, pinnedChat, server.ensureChatContextPinnedOnFirstTurn(ctx, chat))
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
