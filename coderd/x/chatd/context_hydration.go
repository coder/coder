package chatd

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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

// HydrateAndMarkChatsDirty implements agentapi.ContextDirtyMarker. It runs
// inside the PushContextState transaction: it stamps the pushed snapshot hash
// on chats for the agent that have not been hydrated yet (no dirty event),
// then flips already-pinned chats whose hash differs to dirty. It returns a
// callback that publishes the dirty watch events; the caller invokes it only
// after the transaction commits, and the callback is a no-op when nothing
// transitioned to dirty.
//
// The pinned hash on dirtied chats is intentionally left unchanged; the
// refresh endpoint re-pins it.
func (p *Server) HydrateAndMarkChatsDirty(ctx context.Context, tx database.Store, agentID uuid.UUID, aggregateHash []byte, snapshotError string, now time.Time) (func(), error) {
	//nolint:gocritic // An agent does not own the chats bound to it.
	ctx = dbauthz.AsChatd(ctx)

	// Chats created before the agent's first push land with a NULL pinned
	// hash. Stamp them now so they start clean; this is their first
	// hydration, so no dirty event is emitted.
	if err := tx.HydrateAgentChatsContext(ctx, database.HydrateAgentChatsContextParams{
		AgentID:       agentID,
		AggregateHash: aggregateHash,
		ContextError:  snapshotError,
	}); err != nil {
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
	if len(dirtied) == 0 {
		return func() {}, nil
	}

	// Read the dirtied chats inside the transaction and capture their rows so
	// the post-commit callback needs no database access: the published payload
	// reflects the just-committed dirty state (no re-read a concurrent refresh
	// could race), and the callback does not depend on the request-scoped
	// context surviving past commit. Only the transitioned chats are read.
	dirtyChats := make([]database.Chat, 0, len(dirtied))
	for _, d := range dirtied {
		chat, err := tx.GetChatByID(ctx, d.ID)
		if err != nil {
			return nil, xerrors.Errorf("get dirtied chat %s: %w", d.ID, err)
		}
		dirtyChats = append(dirtyChats, chat)
	}

	return func() {
		p.publishChatPubsubEvents(dirtyChats, codersdk.ChatWatchEventKindContextDirty)
	}, nil
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

	aggregateHash, snapshotError, ok, err := latestAgentSnapshot(ctx, p.db, chat.AgentID.UUID)
	if err != nil {
		p.logger.Warn(ctx, "hydrate chat context on create: get latest snapshot",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	if !ok {
		return
	}
	if err := p.db.HydrateAgentChatsContext(ctx, database.HydrateAgentChatsContextParams{
		AgentID:       chat.AgentID.UUID,
		AggregateHash: aggregateHash,
		ContextError:  snapshotError,
	}); err != nil {
		p.logger.Warn(ctx, "hydrate chat context on create: stamp chats",
			slog.F("chat_id", chat.ID), slog.Error(err))
	}
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
	return updated, nil
}
