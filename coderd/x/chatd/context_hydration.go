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

// latestAgentSnapshot returns the agent's most recent pinned context snapshot.
// ok is false (with a nil error) when the agent has not pushed a snapshot yet.
// It takes a Store so callers can run it against either the daemon database or
// an open transaction.
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

	dirtiedIDs := make([]uuid.UUID, len(dirtied))
	for i, d := range dirtied {
		dirtiedIDs[i] = d.ID
	}

	// The publish runs after commit, so resolve the dirtied chats then,
	// reading only the transitioned IDs rather than scanning every active
	// chat for the agent.
	return func() {
		dirtyChats := make([]database.Chat, 0, len(dirtiedIDs))
		for _, id := range dirtiedIDs {
			chat, err := p.db.GetChatByID(ctx, id)
			if err != nil {
				p.logger.Warn(ctx, "publish context dirty: get chat",
					slog.F("chat_id", id), slog.Error(err))
				continue
			}
			dirtyChats = append(dirtyChats, chat)
		}
		p.publishChatPubsubEvents(dirtyChats, codersdk.ChatWatchEventKindContextDirty)
	}, nil
}

// hydrateChatContextOnCreate pins a newly created chat to its agent's latest
// context snapshot when one already exists. Best-effort: a chat whose agent
// has not pushed yet is hydrated later by that agent's next push. Failures
// are logged and swallowed so they never block chat creation.
//
// It stamps via the NULL-guarded HydrateAgentChatsContext so a concurrent
// push that already hydrated the chat is not clobbered with a stale hash.
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

// RefreshChatContext re-pins a chat to its agent's latest context snapshot and
// clears the dirty marker. It backs PUT /chats/{chat}/context (no body). A
// chat with no bound agent, or whose agent has no snapshot, simply has its
// pinned hash and dirty marker cleared.
//
// The snapshot read and the re-pin run in one repeatable-read transaction so a
// concurrent push cannot land between them and leave the chat pinned to a
// stale hash with the dirty marker cleared.
func (p *Server) RefreshChatContext(ctx context.Context, chat database.Chat) (database.Chat, error) {
	//nolint:gocritic // Chatd re-pins the chat as the daemon subject.
	ctx = dbauthz.AsChatd(ctx)

	var updated database.Chat
	err := database.ReadModifyUpdate(p.db, func(tx database.Store) error {
		var (
			aggregateHash []byte
			snapshotError string
		)
		if chat.AgentID.Valid {
			hash, snapErr, ok, err := latestAgentSnapshot(ctx, tx, chat.AgentID.UUID)
			if err != nil {
				return err
			}
			if ok {
				aggregateHash = hash
				snapshotError = snapErr
			}
		}

		if err := tx.SetChatContextSnapshot(ctx, database.SetChatContextSnapshotParams{
			ID:            chat.ID,
			AggregateHash: aggregateHash,
			ContextError:  snapshotError,
		}); err != nil {
			return xerrors.Errorf("set chat context snapshot: %w", err)
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
