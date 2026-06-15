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

// HydrateAndMarkChatsDirty implements agentapi.ContextDirtyMarker. It runs
// inside the PushContextState transaction: it stamps the pushed snapshot hash
// on chats for the agent that have not been hydrated yet (no dirty event),
// then flips already-pinned chats whose hash differs to dirty. It returns a
// callback that publishes the dirty watch events; the caller invokes it only
// after the transaction commits.
//
// The pinned hash on dirtied chats is intentionally left unchanged; the
// refresh endpoint re-pins it. The writes run as the chatd system subject
// because an agent does not own the chats bound to it.
func (p *Server) HydrateAndMarkChatsDirty(ctx context.Context, tx database.Store, agentID uuid.UUID, aggregateHash []byte, snapshotError string, now time.Time) (func(), error) {
	//nolint:gocritic // An agent does not own the chats bound to it, so the
	// daemon subject stamps and dirties them.
	ctx = dbauthz.AsChatd(ctx)

	// Chats created before the agent finished its initial push land with a
	// NULL pinned hash. Stamp them now so they start clean. This is their
	// first hydration, so no dirty event is emitted.
	if err := tx.HydrateAgentChatsContext(ctx, database.HydrateAgentChatsContextParams{
		AgentID:       agentID,
		AggregateHash: aggregateHash,
		ContextError:  snapshotError,
	}); err != nil {
		return nil, xerrors.Errorf("hydrate agent chats context: %w", err)
	}

	flipped, err := tx.MarkChatsContextDirtyByAgent(ctx, database.MarkChatsContextDirtyByAgentParams{
		AgentID:       agentID,
		AggregateHash: aggregateHash,
		DirtySince:    sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return nil, xerrors.Errorf("mark chats context dirty: %w", err)
	}
	if len(flipped) == 0 {
		return func() {}, nil
	}

	// Resolve the expanded chat rows for the flipped IDs so the watch
	// events carry a complete chat payload. Read within the same tx for a
	// consistent view of the chats just flipped.
	active, err := tx.GetActiveChatsByAgentID(ctx, agentID)
	if err != nil {
		return nil, xerrors.Errorf("get active chats by agent: %w", err)
	}
	flippedIDs := make(map[uuid.UUID]struct{}, len(flipped))
	for _, f := range flipped {
		flippedIDs[f.ID] = struct{}{}
	}
	dirtyChats := make([]database.Chat, 0, len(flipped))
	for _, chat := range active {
		if _, ok := flippedIDs[chat.ID]; ok {
			dirtyChats = append(dirtyChats, chat)
		}
	}

	return func() {
		for _, chat := range dirtyChats {
			p.publishChatPubsubEvent(chat, codersdk.ChatWatchEventKindContextDirty, nil)
		}
	}, nil
}

// hydrateChatContextOnCreate pins a newly created chat to its agent's latest
// context snapshot when one already exists. Best-effort: a chat whose agent
// has not pushed yet is hydrated later by that agent's next push. Failures
// are logged and swallowed so they never block chat creation; the columns
// are dark and do not affect chat behavior.
func (p *Server) hydrateChatContextOnCreate(ctx context.Context, chat database.Chat) {
	if !chat.AgentID.Valid {
		return
	}
	//nolint:gocritic // Chatd is internal, not a user; it reads the agent
	// snapshot and stamps the chat as the daemon subject.
	ctx = dbauthz.AsChatd(ctx)
	snapshot, err := p.db.GetLatestWorkspaceAgentContextSnapshot(ctx, chat.AgentID.UUID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return
	case err != nil:
		p.logger.Warn(ctx, "hydrate chat context on create: get latest snapshot",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	if err := p.db.SetChatContextSnapshot(ctx, database.SetChatContextSnapshotParams{
		ID:            chat.ID,
		AggregateHash: snapshot.AggregateHash,
		ContextError:  snapshot.SnapshotError,
	}); err != nil {
		p.logger.Warn(ctx, "hydrate chat context on create: set snapshot",
			slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

// RefreshChatContext re-pins a chat to its agent's latest context snapshot and
// clears the dirty marker. It backs PUT /chats/{chat}/context (no body). A
// chat with no bound agent, or whose agent has no snapshot, simply has its
// pinned hash and dirty marker cleared.
func (p *Server) RefreshChatContext(ctx context.Context, chat database.Chat) (database.Chat, error) {
	//nolint:gocritic // Chatd is internal, not a user; it re-pins the chat to
	// the agent snapshot as the daemon subject.
	ctx = dbauthz.AsChatd(ctx)

	var (
		aggregateHash []byte
		snapshotError string
	)
	if chat.AgentID.Valid {
		snapshot, err := p.db.GetLatestWorkspaceAgentContextSnapshot(ctx, chat.AgentID.UUID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// No snapshot yet; clear the pinned hash and dirty marker.
		case err != nil:
			return database.Chat{}, xerrors.Errorf("get latest snapshot: %w", err)
		default:
			aggregateHash = snapshot.AggregateHash
			snapshotError = snapshot.SnapshotError
		}
	}

	if err := p.db.SetChatContextSnapshot(ctx, database.SetChatContextSnapshotParams{
		ID:            chat.ID,
		AggregateHash: aggregateHash,
		ContextError:  snapshotError,
	}); err != nil {
		return database.Chat{}, xerrors.Errorf("set chat context snapshot: %w", err)
	}

	updated, err := p.db.GetChatByID(ctx, chat.ID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("get chat after refresh: %w", err)
	}
	return updated, nil
}
