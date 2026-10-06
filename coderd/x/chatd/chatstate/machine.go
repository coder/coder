package chatstate

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

// HeartbeatStaleSeconds is the threshold chatstate uses when deciding
// whether to publish a `chat:ownership` hint for a runnable chat. A
// heartbeat older than this many seconds (by database time) counts
// as stale and triggers a hint so an idle worker can attempt a
// takeover.
const HeartbeatStaleSeconds = 30

// ChatMachine is a chat-scoped handle for state-machine operations on
// a single chat row. It captures the database store, the pubsub
// publisher, and the chat ID at construction time so callers do not
// have to thread them through Update, Lock, or any transition method.
//
// ChatMachine values are cheap. Create one per chat for the lifetime
// of a request or worker turn; do not cache mutable chat state across
// calls.
type ChatMachine struct {
	store     database.Store
	publisher Publisher
	chatID    uuid.UUID
}

// NewChatMachine constructs a chat-scoped state machine handle. The
// store may be the root database handle or an existing transaction
// handle; publisher is the pubsub used for `chat:update` and
// `chat:ownership` emissions. Both are required and captured for the
// lifetime of the returned machine.
func NewChatMachine(
	store database.Store,
	publisher Publisher,
	chatID uuid.UUID,
) *ChatMachine {
	return &ChatMachine{
		store:     store,
		publisher: publisher,
		chatID:    chatID,
	}
}

// Tx is the per-transaction handle passed to [ChatMachine.Update]
// callbacks. It carries the active context, the transactional store,
// the chat ID, the row returned by the transition lock (locked), and
// the write intent the transaction's commit write must record
// (historyChanged, queueChanged, commits).
//
// locked lets the first read in the transaction skip a round trip while
// the row lock is held. It is safe to reuse because the lock blocks
// every other writer of the chats row until commit, and every queue
// write goes through a transition under the same lock, so only this
// transaction can make it stale. A transition's commit write does make
// it stale, so the first loadState call consumes it and later calls
// read the database. That relies on every transition validating before
// it writes, which is a convention of the transition methods, not
// something Tx checks. Writes a callback makes through the raw store
// are not visible to locked.
//
// Triggers no longer record history or queue changes, so the message
// and queue helpers set historyChanged and queueChanged, and the next
// commit write consumes them (see takeVersionFlags). commits counts
// commit writes so Update knows whether it still has to bump.
type Tx struct {
	ctx    context.Context
	store  database.Store
	chatID uuid.UUID

	locked *lockedRow

	historyChanged bool
	queueChanged   bool
	commits        int
}

// lockedRow carries has_queued alongside the chat so the first
// execution-state classification needs no queue count.
type lockedRow struct {
	chat      database.Chat
	hasQueued bool
}

// Ctx returns the context the surrounding [ChatMachine.Update] call
// is using.
func (tx *Tx) Ctx() context.Context { return tx.ctx }

// ChatID returns the chat ID this transaction is scoped to.
func (tx *Tx) ChatID() uuid.UUID { return tx.chatID }

// Store exposes the active transaction store so callers can perform
// validation reads (for example loading the messages affected by an
// EditMessage transition) and metadata writes (for example updating
// title or labels) that must be atomic with the transition.
//
// Callers MUST NOT use Store to mutate execution-state tables
// (chats.status, chat_messages, chat_queued_messages, chat_heartbeats,
// or the version fields on chats). Those mutations belong to the
// transition methods and are validated against the state machine
// matrix.
func (tx *Tx) Store() database.Store { return tx.store }

// loadState returns the chat row and execution state for a transition
// to validate against. The locked row is returned at most once because
// the transition that reads it is about to write the chat.
func (tx *Tx) loadState() (database.Chat, ExecutionState, error) {
	if l := tx.locked; l != nil {
		tx.locked = nil
		return l.chat, ClassifyExecutionState(l.chat, l.hasQueued, true), nil
	}
	return tx.readState()
}

// readState reads the chat row and queue cardinality from the database,
// for callers that need the row after a commit write. Returns
// ErrChatNotFound if the chat row was deleted in this transaction (or
// never existed).
func (tx *Tx) readState() (database.Chat, ExecutionState, error) {
	chat, err := tx.store.GetChatByID(tx.ctx, tx.chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return database.Chat{}, StateN, ErrChatNotFound
		}
		return database.Chat{}, "", xerrors.Errorf("load chat: %w", err)
	}
	count, err := tx.store.CountChatQueuedMessages(tx.ctx, tx.chatID)
	if err != nil {
		return database.Chat{}, "", xerrors.Errorf("count queued messages: %w", err)
	}
	return chat, ClassifyExecutionState(chat, count > 0, true), nil
}

// Current returns the chat row and execution state without another round
// trip or lock when no transition has run yet, by returning the row the
// [ChatMachine.Update] lock already returned. It does not consume that
// row, so a callback can inspect state and then call a transition that
// validates against the same row. Once a transition has run, it reads
// the database.
func (tx *Tx) Current() (database.Chat, ExecutionState, error) {
	if l := tx.locked; l != nil {
		return l.chat, ClassifyExecutionState(l.chat, l.hasQueued, true), nil
	}
	return tx.loadState()
}

// takeVersionFlags returns and clears the pending history and queue
// change flags and counts the commit write that consumes them. Every
// statement that advances snapshot_version must go through this so the
// version fields it writes reflect everything the transaction changed.
func (tx *Tx) takeVersionFlags() (historyChanged, queueChanged bool) {
	historyChanged, queueChanged = tx.historyChanged, tx.queueChanged
	tx.historyChanged, tx.queueChanged = false, false
	tx.commits++
	return historyChanged, queueChanged
}

// requireNoVersionFlags is takeVersionFlags for commit writes that
// cannot record history or queue changes. Pending flags would be lost,
// so they are a programming error in the transition bundle.
func (tx *Tx) requireNoVersionFlags(t Transition) error {
	if tx.historyChanged || tx.queueChanged {
		return xerrors.Errorf("chatstate: %s cannot commit pending history or queue changes", t)
	}
	tx.commits++
	return nil
}

// requireFromAllowed loads the current state and validates t against
// the transition matrix. Returns the loaded chat and execution state
// on success, [ErrInvalidState] when the chat is in an invalid state
// and t is not [TransitionReconcileInvalidState], and a typed
// *TransitionError otherwise.
func (tx *Tx) requireFromAllowed(t Transition) (database.Chat, ExecutionState, error) {
	chat, from, err := tx.loadState()
	if err != nil {
		return chat, from, err
	}
	if from == StateInvalid && t != TransitionReconcileInvalidState {
		return chat, from, ErrInvalidState
	}
	if err := requireExecutionTransition(t, from); err != nil {
		return chat, from, err
	}
	return chat, from, nil
}

// Update applies one or more transitions to the machine's chat.
//
// Update opens a transaction on the captured store, locks the chat row
// with FOR NO KEY UPDATE without writing it, then runs fn against a
// fresh [*Tx] and the active transaction store. Each transition ends
// with a single commit write that advances `snapshot_version`; the lock
// itself writes nothing so that commit write is the only UPDATE of the
// row in the transaction and Postgres does not re-run the chat's foreign
// key checks against parent rows shared by many chats. FOR NO KEY UPDATE
// still serializes transitions but does not block the FOR KEY SHARE locks
// that foreign-key child writes such as heartbeats take on the chat row
// (see LockChatForTransition). If fn performs no commit write, Update
// bumps `snapshot_version` once after it so every Update publishes a new
// version. Rows read inside fn before that bump carry the pre-commit
// versions. Update constructs a
// [PublishBuffer], enqueues `chat:update` (and a `chat:ownership` hint
// when the post-transition state is worker-runnable and ownership is
// missing or stale) inside the transaction, and flushes the buffer only after
// the transaction function succeeds. If the transaction rolls back,
// the deferred Discard suppresses every buffered publication so
// subscribers never see uncommitted state.
//
// If Update is called with a store that is already in a transaction,
// [database.Store.InTx] reuses the active transaction. In that case,
// callers that need outer-transaction publication semantics can pass a
// [PublishBuffer] as the machine publisher. The inner buffer flushes
// into the outer buffer, and the outer owner remains responsible for
// publishing only after the outer transaction commits.
//
// If the chat row does not exist, Update returns [ErrChatNotFound]
// without mutating anything.
//
// Callbacks that return an error roll back the transaction and publish
// nothing.
func (m *ChatMachine) Update(
	ctx context.Context,
	fn func(*Tx, database.Store) error,
) error {
	if m.store == nil {
		return xerrors.New("chatstate: ChatMachine has nil store")
	}
	if m.publisher == nil {
		return xerrors.New("chatstate: ChatMachine has nil publisher")
	}

	buffer := NewPublishBuffer(m.publisher)
	defer buffer.Discard()

	err := m.store.InTx(func(store database.Store) error {
		locked, err := store.LockChatForTransition(ctx, m.chatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			return xerrors.Errorf("lock chat: %w", err)
		}
		tx := &Tx{
			ctx:    ctx,
			store:  store,
			chatID: m.chatID,
			locked: &lockedRow{chat: locked.Chat, hasQueued: locked.HasQueued},
		}
		if err := fn(tx, store); err != nil {
			return err
		}
		if tx.commits == 0 {
			historyChanged, queueChanged := tx.takeVersionFlags()
			if _, err := store.BumpChatSnapshotVersion(ctx, database.BumpChatSnapshotVersionParams{
				ID:             m.chatID,
				HistoryChanged: historyChanged,
				QueueChanged:   queueChanged,
			}); err != nil {
				return xerrors.Errorf("bump chat snapshot: %w", err)
			}
		}
		// The commit write changed the row, so publication must not use
		// the locked row.
		chat, state, err := tx.readState()
		if err != nil {
			return err
		}
		if err := buffer.Publish(
			coderdpubsub.ChatStateUpdateChannel(chat.ID),
			buildChatUpdateMessage(chat),
		); err != nil {
			return xerrors.Errorf("buffer chat update: %w", err)
		}
		if state.IsRunnable() {
			stale, err := ownershipStaleOrMissing(ctx, store, chat, HeartbeatStaleSeconds)
			if err != nil {
				return xerrors.Errorf("evaluate ownership: %w", err)
			}
			if stale {
				if err := buffer.Publish(
					coderdpubsub.ChatStateOwnershipChannel,
					buildChatOwnershipMessage(chat),
				); err != nil {
					return xerrors.Errorf("buffer ownership hint: %w", err)
				}
			}
		}
		return nil
	}, nil)
	if err != nil {
		return err
	}
	return buffer.Flush()
}

// ReadLock takes a shared lock on the chat row with FOR SHARE and runs
// fn in a transaction without advancing snapshot_version. It uses the
// store captured by [NewChatMachine]. Use it when the caller needs a
// consistent chat snapshot plus related rows such as messages or queued
// messages but is NOT applying a transition and does NOT need to block
// concurrent readers.
//
// The FOR SHARE lock permits other shared lockers to proceed
// concurrently while still blocking writers that take FOR NO KEY UPDATE
// (such as [ChatMachine.Update]) until the transaction commits.
//
// Callers must not pass a store here; it belongs on the machine.
//
// ReadLock publishes nothing. Callback errors roll back the transaction
// and propagate to the caller.
func (m *ChatMachine) ReadLock(
	ctx context.Context,
	fn func(database.Store) error,
) error {
	if m.store == nil {
		return xerrors.New("chatstate: ChatMachine has nil store")
	}
	return m.store.InTx(func(store database.Store) error {
		// GetChatByIDForShare takes a shared lock on the row WITHOUT
		// bumping snapshot.
		_, err := store.GetChatByIDForShare(ctx, m.chatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			return xerrors.Errorf("read lock chat: %w", err)
		}
		return fn(store)
	}, nil)
}

// ownershipStaleOrMissing reports whether the chat's current
// (chat_id, runner_id) lease is missing or stale. The staleSeconds
// threshold is forwarded to [database.IsChatHeartbeatStale] so the
// comparison runs against database time inside a single SQL query.
func ownershipStaleOrMissing(ctx context.Context, store database.Store, chat database.Chat, staleSeconds int32) (bool, error) {
	if !chat.WorkerID.Valid || !chat.RunnerID.Valid {
		return true, nil
	}
	return store.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
		ChatID:       chat.ID,
		RunnerID:     chat.RunnerID.UUID,
		StaleSeconds: staleSeconds,
	})
}
