package chatstate

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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
// commit writes so Update knows whether it still has to bump. committed
// keeps the last commit write's result so Update can publish and return
// the committed row without reading it again; it is never used to
// validate a transition.
type Tx struct {
	ctx    context.Context
	store  database.Store
	chatID uuid.UUID

	locked    *lockedRow
	committed *commitResult

	historyChanged bool
	queueChanged   bool
	commits        int
}

// lockedRow carries the queue state read right after the lock so the
// first execution-state classification needs no further reads.
type lockedRow struct {
	chat      database.Chat
	hasQueued bool
}

// commitResult is the chat row a commit write returned together with the
// queue and ownership lease flags evaluated in the same statement.
type commitResult struct {
	chat           database.Chat
	hasQueued      bool
	ownershipStale bool
}

// recordCommit keeps the result of the commit write that just ran.
func (tx *Tx) recordCommit(chat database.Chat, hasQueued, ownershipStale bool) {
	tx.committed = &commitResult{chat: chat, hasQueued: hasQueued, ownershipStale: ownershipStale}
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
// versions. The commit write returns the committed row together with
// the queue and ownership lease flags, and Update publishes from that
// result: it constructs a [PublishBuffer], enqueues `chat:update` (and a
// `chat:ownership` hint when the post-transition state is
// worker-runnable and ownership is missing or stale) inside the
// transaction, and flushes the buffer only after the transaction
// function succeeds. If the transaction rolls back, the deferred Discard
// suppresses every buffered publication so subscribers never see
// uncommitted state.
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
//
// Lock order is the chat row first, then automations in ascending id
// order (see [LockAutomations]). When an attempt that took automation
// locks is aborted by a PostgreSQL deadlock, Update reruns the whole
// transaction, including fn, up to [maxUpdateAttempts] times. fn must
// therefore be safe to rerun: it may only do database work through the
// transaction and assign results. Attempts that took no automation locks
// are never retried, and neither is an Update nested inside another
// Update, because the outer transaction is already aborted.
func (m *ChatMachine) Update(
	ctx context.Context,
	fn func(*Tx, database.Store) error,
) error {
	_, err := m.UpdateReturning(ctx, fn)
	return err
}

// UpdateReturning is [ChatMachine.Update] that also returns the chat row
// the transition's commit write produced, so callers do not read the row
// again while holding the lock. The row is returned whenever the
// transaction committed, including when publishing the state update
// failed afterwards; it is zero when the transaction rolled back.
func (m *ChatMachine) UpdateReturning(
	ctx context.Context,
	fn func(*Tx, database.Store) error,
) (database.Chat, error) {
	if m.store == nil {
		return database.Chat{}, xerrors.New("chatstate: ChatMachine has nil store")
	}
	if m.publisher == nil {
		return database.Chat{}, xerrors.New("chatstate: ChatMachine has nil publisher")
	}
	if _, nested := updateAttemptFromContext(ctx); nested {
		// The outer Update owns the transaction, its lock record, and
		// any retry.
		return m.updateOnce(ctx, fn)
	}
	for attempt := 1; ; attempt++ {
		state := &updateAttempt{}
		chat, err := m.updateOnce(context.WithValue(ctx, updateAttemptKey{}, state), fn)
		if err == nil || attempt >= maxUpdateAttempts ||
			!state.automationLocks || !database.IsDeadlockError(err) {
			return chat, err
		}
	}
}

// maxUpdateAttempts bounds the deadlock retries of [ChatMachine.Update].
const maxUpdateAttempts = 3

// updateOnce runs one Update attempt with its own publish buffer, so an
// aborted attempt publishes nothing.
func (m *ChatMachine) updateOnce(
	ctx context.Context,
	fn func(*Tx, database.Store) error,
) (database.Chat, error) {
	buffer := NewPublishBuffer(m.publisher)
	defer buffer.Discard()

	var committed database.Chat
	err := m.store.InTx(func(store database.Store) error {
		locked, err := store.LockChatForTransition(ctx, m.chatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			return xerrors.Errorf("lock chat: %w", err)
		}
		// The lock returned the authoritative row. Cache its RBAC object
		// on the transaction context so dbauthz authorizes the callback's
		// writes without re-reading the chat under the lock.
		txCtx, err := dbauthz.WithChatRBAC(ctx, dbauthz.CacheableChatRBAC(locked))
		if err != nil {
			return xerrors.Errorf("cache chat rbac: %w", err)
		}
		queued, err := store.CountChatQueuedMessages(txCtx, m.chatID)
		if err != nil {
			return xerrors.Errorf("count queued messages: %w", err)
		}
		tx := &Tx{
			ctx:    txCtx,
			store:  store,
			chatID: m.chatID,
			locked: &lockedRow{chat: locked, hasQueued: queued > 0},
		}
		if err := fn(tx, store); err != nil {
			return err
		}
		if tx.commits == 0 {
			historyChanged, queueChanged := tx.takeVersionFlags()
			bumped, err := store.BumpChatSnapshotVersion(txCtx, database.BumpChatSnapshotVersionParams{
				ID:             m.chatID,
				HistoryChanged: historyChanged,
				QueueChanged:   queueChanged,
				StaleSeconds:   HeartbeatStaleSeconds,
			})
			if err != nil {
				return xerrors.Errorf("bump chat snapshot: %w", err)
			}
			tx.recordCommit(bumped.Chat, bumped.HasQueued, bumped.OwnershipStale)
		} else if tx.historyChanged || tx.queueChanged {
			return ErrStagedAfterCommitWrite
		}
		result := tx.committed
		if result == nil {
			return xerrors.New("chatstate: commit write recorded no result")
		}
		committed = result.chat
		if err := buffer.Publish(
			coderdpubsub.ChatStateUpdateChannel(result.chat.ID),
			buildChatUpdateMessage(result.chat),
		); err != nil {
			return xerrors.Errorf("buffer chat update: %w", err)
		}
		if ClassifyExecutionState(result.chat, result.hasQueued, true).IsRunnable() && result.ownershipStale {
			if err := buffer.Publish(
				coderdpubsub.ChatStateOwnershipChannel,
				buildChatOwnershipMessage(result.chat),
			); err != nil {
				return xerrors.Errorf("buffer ownership hint: %w", err)
			}
		}
		return nil
	}, nil)
	if err != nil {
		return database.Chat{}, err
	}
	return committed, buffer.Flush()
}

// ReadSnapshot runs fn in a read-only REPEATABLE READ transaction without
// advancing snapshot_version. It uses the store captured by
// [NewChatMachine]. Use it when the caller needs a consistent chat
// snapshot plus related rows such as messages or queued messages but is
// NOT applying a transition.
//
// Snapshot isolation gives fn a consistent multi-statement view without
// taking any row lock, so it never blocks (or is blocked by) the FOR NO
// KEY UPDATE lock in [ChatMachine.Update]. A transition committing
// mid-read is simply not visible to this snapshot; callers reconcile
// ordering via snapshot_version, so an older snapshot triggers a later
// refetch rather than an inconsistent read.
//
// ReadSnapshot does not check that the chat exists; fn's own reads report
// a missing chat as sql.ErrNoRows.
//
// Callers must not pass a store here; it belongs on the machine.
//
// ReadSnapshot publishes nothing. Callback errors roll back the
// transaction and propagate to the caller.
func (m *ChatMachine) ReadSnapshot(fn func(database.Store) error) error {
	if m.store == nil {
		return xerrors.New("chatstate: ChatMachine has nil store")
	}
	return m.store.InTx(fn, &database.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
}
