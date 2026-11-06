package chatd

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/codersdk"
)

// latestAgentSnapshot looks up an agent's pinned context snapshot; ok is false
// (with a nil error) when the agent has not pushed one yet.
func latestAgentSnapshot(ctx context.Context, db database.Store, agentID uuid.UUID) (aggregateHash []byte, snapshotError string, ok bool, err error) {
	snapshot, err := db.GetLatestWorkspaceAgentContextSnapshot(ctx, agentID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, "", false, nil
	case err != nil:
		return nil, "", false, xerrors.Errorf("get latest snapshot: %w", err)
	default:
		return snapshot.AggregateHash, snapshot.SnapshotError, true, nil
	}
}

// UpdateAgentChatsContext pins unpinned chats, marks or clears dirty markers by
// aggregateHash, and syncs snapshotError and MCP/ACP resources for unarchived
// chats bound to agentID. Its callback publishes one event per changed chat after
// commit.
func (p *Server) UpdateAgentChatsContext(ctx context.Context, tx database.Store, agentID uuid.UUID, aggregateHash []byte, snapshotError string, now time.Time) (func(), error) {
	//nolint:gocritic // An agent does not own the chats bound to it.
	ctx = dbauthz.AsChatd(ctx)

	// Chats created before the agent's first push land with a NULL pinned
	// hash. Stamp them now so they start clean.
	hydrated, err := tx.HydrateAgentChatsContext(ctx, database.HydrateAgentChatsContextParams{
		AgentID:       agentID,
		AggregateHash: aggregateHash,
		ContextError:  snapshotError,
	})
	if err != nil {
		return nil, xerrors.Errorf("hydrate agent chats context: %w", err)
	}

	dirtied, err := tx.MarkChatsContextDirtyByAgent(ctx, database.MarkChatsContextDirtyByAgentParams{
		AgentID:       agentID,
		AggregateHash: aggregateHash,
		DirtySince:    sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return nil, xerrors.Errorf("mark chats context dirty: %w", err)
	}
	cleared, err := tx.ClearChatsContextDirtyByAgent(ctx, database.ClearChatsContextDirtyByAgentParams{
		AgentID:       agentID,
		AggregateHash: aggregateHash,
	})
	if err != nil {
		return nil, xerrors.Errorf("clear chats context dirty: %w", err)
	}

	errorsSynced, err := tx.SyncAgentChatsContextError(ctx, database.SyncAgentChatsContextErrorParams{
		AgentID:      agentID,
		ContextError: snapshotError,
	})
	if err != nil {
		return nil, xerrors.Errorf("sync agent chats context error: %w", err)
	}

	// MCP and ACP resources bypass drift detection, so sync them on every push.
	synced, err := tx.SyncAgentChatsContextLiveResources(ctx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("sync agent chats live context resources: %w", err)
	}

	dirtiedIDs := make([]uuid.UUID, 0, len(dirtied))
	for _, chat := range dirtied {
		dirtiedIDs = append(dirtiedIDs, chat.ID)
	}
	// A chat can appear in several lists; publish it once.
	touched := slice.Unique(slices.Concat(hydrated, dirtiedIDs, cleared, errorsSynced, synced))
	return p.chatContextPublisher(ctx, tx, touched)
}

func (p *Server) chatContextPublisher(ctx context.Context, tx database.Store, touched []uuid.UUID) (func(), error) {
	if len(touched) == 0 {
		return func() {}, nil
	}

	// Read the touched chats inside the transaction and capture their rows so
	// the post-commit callback needs no database access: the published payload
	// reflects the just-committed state (no re-read a concurrent refresh
	// could race), and the callback does not depend on the request-scoped
	// context surviving past commit. Only the transitioned chats are read.
	touchedChats := make([]database.Chat, 0, len(touched))
	for _, id := range touched {
		chat, err := tx.GetChatByID(ctx, id)
		if err != nil {
			return nil, xerrors.Errorf("get touched chat %s: %w", id, err)
		}
		touchedChats = append(touchedChats, chat)
	}

	return func() {
		p.publishChatPubsubEvents(touchedChats, codersdk.ChatWatchEventKindContextDirty)
	}, nil
}

// AdoptWorkspaceChatsContext moves chats to the latest build's selected chat
// agent on its first stored snapshot. Matching pins keep their resources.
func (p *Server) AdoptWorkspaceChatsContext(ctx context.Context, tx database.Store, agentID uuid.UUID, aggregateHash []byte) (func(), error) {
	//nolint:gocritic // An agent does not own the workspace's chats.
	ctx = dbauthz.AsChatd(ctx)
	adopted, err := adoptWorkspaceChatsContext(ctx, tx, p.logger, agentID, aggregateHash)
	if err != nil {
		return nil, err
	}
	return p.chatContextPublisher(ctx, tx, adopted)
}

func adoptWorkspaceChatsContext(ctx context.Context, tx database.Store, logger slog.Logger, agentID uuid.UUID, aggregateHash []byte) ([]uuid.UUID, error) {
	workspace, err := tx.GetWorkspaceByAgentID(ctx, agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get workspace for context adoption: %w", err)
	}
	if workspace.Deleted {
		return nil, nil
	}
	agents, err := tx.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		return nil, xerrors.Errorf("get latest build agents for context adoption: %w", err)
	}
	selected, err := agentselect.FindChatAgent(agents)
	if err != nil {
		logger.Debug(ctx, "skip workspace context adoption: chat agent selection failed",
			slog.F("workspace_id", workspace.ID), slog.Error(err))
		return nil, nil
	}
	if selected.ID != agentID {
		return nil, nil
	}
	build, err := tx.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		return nil, xerrors.Errorf("get latest build for context adoption: %w", err)
	}
	chats, err := tx.RebindWorkspaceChatsToAgent(ctx, database.RebindWorkspaceChatsToAgentParams{
		WorkspaceID: workspace.ID,
		BuildID:     build.ID,
		AgentID:     agentID,
	})
	if err != nil {
		return nil, xerrors.Errorf("rebind workspace chats for context adoption: %w", err)
	}
	var adopted []uuid.UUID
	for _, chat := range chats {
		if chat.ContextAggregateHash != nil && bytes.Equal(chat.ContextAggregateHash, aggregateHash) {
			continue
		}
		if err := repinChatContext(ctx, tx, chat.ID, uuid.NullUUID{UUID: agentID, Valid: true}); err != nil {
			return nil, xerrors.Errorf("re-pin chat context for new build: %w", err)
		}
		adopted = append(adopted, chat.ID)
	}
	return adopted, nil
}

// hydrateAgentChatsFromSnapshot stamps every chat bound to agentID that still
// carries a NULL pinned hash with the agent's latest pushed snapshot and copies
// that snapshot's resources. It runs in one repeatable-read transaction so a
// concurrent push cannot commit between the hash read and the resource copy and
// leave a chat stamped with one snapshot's hash but another snapshot's
// resources. It is the shared core of first-time pinning: idempotent because
// HydrateAgentChatsContext only touches NULL-hash chats (a concurrent push that
// already hydrated the chat is not clobbered), and snapshot-gated so it does
// nothing when the agent has not pushed yet, never stamping empty state that
// would keep a later push from hydrating. It returns the IDs of the chats it
// pinned; empty when the agent has no snapshot or every chat was already
// pinned.
func (p *Server) hydrateAgentChatsFromSnapshot(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error) {
	var hydrated []uuid.UUID
	err := database.ReadModifyUpdate(p.db, func(tx database.Store) error {
		aggregateHash, snapshotError, ok, err := latestAgentSnapshot(ctx, tx, agentID)
		if err != nil {
			return err
		}
		if !ok {
			hydrated = nil
			return nil
		}
		hydrated, err = tx.HydrateAgentChatsContext(ctx, database.HydrateAgentChatsContextParams{
			AgentID:       agentID,
			AggregateHash: aggregateHash,
			ContextError:  snapshotError,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return hydrated, nil
}

// hydrateChatContextOnCreate pins a newly created chat to its agent's latest
// context snapshot when one already exists. Best-effort: a chat whose agent
// has not pushed yet is hydrated later by that agent's next push. Failures
// are logged and swallowed so they never block chat creation.
//
// A concurrent push that already hydrated the chat is not clobbered with a
// stale hash.
func (p *Server) hydrateChatContextOnCreate(ctx context.Context, chat database.Chat) {
	if !chat.AgentID.Valid {
		return
	}
	//nolint:gocritic // Chatd stamps chats it does not own as the daemon subject.
	ctx = dbauthz.AsChatd(ctx)
	if _, err := p.hydrateAgentChatsFromSnapshot(ctx, chat.AgentID.UUID); err != nil {
		p.logger.Warn(ctx, "hydrate chat context on create",
			slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

// ensureChatContextPinnedOnFirstTurn pins a chat to its freshly bound agent's
// latest pushed snapshot when the chat is still unpinned. API-created chats
// carry no agent at create, so hydrateChatContextOnCreate is a no-op for them;
// they bind their agent lazily on the first turn. Without this, such a chat
// reads empty pinned context on its first turn whenever the agent pushed before
// the chat existed, because that push could not hydrate a chat that did not yet
// exist. It reuses the create-path hydration, which is idempotent and
// snapshot-gated, so it never clobbers an already-pinned chat and never stamps
// empty state. The NULL-hash gate also leaves dirtied chats alone: their stale
// pinned hash is non-NULL until the refresh endpoint re-pins. Hydration
// pins every unpinned chat bound to the agent in one statement, so a
// context watch event is published for each pinned chat: watching clients
// cached those chats' details without pinned resources and need to
// refetch. Best-effort: failures are logged and swallowed so they never
// fail the turn.
func (p *Server) ensureChatContextPinnedOnFirstTurn(ctx context.Context, chat database.Chat) {
	if !chat.AgentID.Valid || chat.ContextAggregateHash != nil {
		return
	}
	//nolint:gocritic // Chatd stamps chats it does not own as the daemon subject.
	ctx = dbauthz.AsChatd(ctx)
	hydrated, err := p.hydrateAgentChatsFromSnapshot(ctx, chat.AgentID.UUID)
	if err != nil {
		p.logger.Warn(ctx, "ensure chat context pinned on first turn",
			slog.F("chat_id", chat.ID),
			slog.F("agent_id", chat.AgentID.UUID),
			slog.Error(err))
		return
	}
	pinnedChats := make([]database.Chat, 0, len(hydrated))
	for _, chatID := range hydrated {
		// Re-read each chat so the event payload carries the pinned
		// state, not the pre-hydration row.
		pinned, err := p.db.GetChatByID(ctx, chatID)
		if err != nil {
			p.logger.Warn(ctx, "read chat after first-turn context pin",
				slog.F("chat_id", chatID),
				slog.Error(err))
			continue
		}
		pinnedChats = append(pinnedChats, pinned)
	}
	p.publishChatPubsubEvents(pinnedChats, codersdk.ChatWatchEventKindContextDirty)
}

// repinChatContext re-pins a single chat to its agent's latest context
// snapshot: it sets the pinned hash and error and rewrites the chat's pinned
// resources (clear-then-copy) so the two always agree. A chat with no bound
// agent, or whose agent has no snapshot, has its pinned hash, dirty marker,
// and resources cleared. Callers run this inside a transaction.
func repinChatContext(ctx context.Context, db database.Store, chatID uuid.UUID, agentID uuid.NullUUID) error {
	var (
		aggregateHash []byte
		snapshotError string
		hasSnapshot   bool
	)
	if agentID.Valid {
		hash, snapErr, ok, err := latestAgentSnapshot(ctx, db, agentID.UUID)
		if err != nil {
			return err
		}
		if ok {
			aggregateHash = hash
			snapshotError = snapErr
			hasSnapshot = true
		}
	}

	if err := db.SetChatContextSnapshot(ctx, database.SetChatContextSnapshotParams{
		ID:            chatID,
		AggregateHash: aggregateHash,
		ContextError:  snapshotError,
	}); err != nil {
		return xerrors.Errorf("set chat context snapshot: %w", err)
	}

	// Clear-then-copy so the pinned resources always match the pinned hash.
	// A single delete+insert statement cannot see its own delete under
	// snapshot isolation, so overlapping sources would collide.
	if err := db.DeleteChatContextResourcesByChatID(ctx, chatID); err != nil {
		return xerrors.Errorf("clear chat context resources: %w", err)
	}
	if hasSnapshot {
		if err := db.InsertAgentContextResourcesIntoChat(ctx, database.InsertAgentContextResourcesIntoChatParams{
			ChatID:  chatID,
			AgentID: agentID.UUID,
		}); err != nil {
			return xerrors.Errorf("copy agent context resources: %w", err)
		}
	}
	return nil
}

// RefreshChatContext re-pins a chat to its agent's latest context snapshot
// (hash, error, and resource bodies) and clears the dirty marker. It backs
// PUT /chats/{chat}/context (no body). A chat with no bound agent, or whose
// agent has no snapshot, simply has its pinned hash, dirty marker, and
// resources cleared.
//
// The snapshot read and the re-pin run in one repeatable-read transaction so a
// concurrent push cannot land between them and leave the chat pinned to a
// stale hash with the dirty marker cleared.
func (p *Server) RefreshChatContext(ctx context.Context, chat database.Chat) (database.Chat, error) {
	//nolint:gocritic // Chatd re-pins the chat as the daemon subject.
	ctx = dbauthz.AsChatd(ctx)

	var updated database.Chat
	err := database.ReadModifyUpdate(p.db, func(tx database.Store) error {
		// Re-read the chat inside the transaction so a serialization-conflict
		// retry re-pins against the chat's current agent. Using the AgentID
		// captured before the transaction would re-pin to a stale agent if a
		// concurrent rebind landed between that read and the retry.
		current, err := tx.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get chat for refresh: %w", err)
		}
		if err := repinChatContext(ctx, tx, current.ID, current.AgentID); err != nil {
			return err
		}
		got, err := tx.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get chat after refresh: %w", err)
		}
		updated = got
		return nil
	})
	if err != nil {
		return database.Chat{}, err
	}
	p.publishChatPubsubEvents([]database.Chat{updated}, codersdk.ChatWatchEventKindContextDirty)
	return updated, nil
}
