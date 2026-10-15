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
	acquirer  LocalAcquirer
}

// LocalAcquirer lets the replica that makes a chat runnable acquire it in
// the same transaction, so no ownership hint makes every replica race for
// the chat row lock.
type LocalAcquirer interface {
	// PrepareAcquire runs inside the transaction with the chat locked and
	// unowned. Returning false leaves the chat to the ownership hint. An
	// error rolls back the transaction.
	PrepareAcquire(ctx context.Context, store database.Store, chat database.Chat) (AcquireInput, bool, error)
	// StartAcquired runs once after commit and publication for a chat
	// acquired with the input PrepareAcquire returned.
	StartAcquired(chatID uuid.UUID, input AcquireInput)
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

// WithLocalAcquirer returns a copy of m whose top-level updates acquire
// the chat through a when they leave it runnable and unowned. The
// machine's store must not be an outer transaction, because a is started
// after Update's own commit.
func (m *ChatMachine) WithLocalAcquirer(a LocalAcquirer) *ChatMachine {
	c := *m
	c.acquirer = a
	return &c
}

// Tx is the per-transaction handle passed to [ChatMachine.Update]
// callbacks. It carries the active context, the transactional store,
// and the chat ID. Tx does not cache mutable chat state across calls:
// every transition method reads the chat row and queue cardinality
// from the database on entry, so a bundle of transitions inside one
// Update callback always validates against the latest committed state.
type Tx struct {
	ctx    context.Context
	store  database.Store
	chatID uuid.UUID
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

// loadState reads the current chat row and queue cardinality from the
// active transaction, classifies the execution state, and returns the
// inputs every transition method needs. Returns ErrChatNotFound if
// the chat row was deleted in this transaction (or never existed).
func (tx *Tx) loadState() (database.Chat, ExecutionState, error) {
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
// Update opens a transaction on the captured store, atomically locks
// the chat row with FOR UPDATE and increments `snapshot_version`
// exactly once, then runs fn against a fresh [*Tx] and the active
// transaction store. It constructs a [PublishBuffer], enqueues
// `chat:update` (and a `chat:ownership` hint
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
// Callbacks that return an error roll back the transaction (rolling
// back the automatic snapshot bump) and publish nothing.
//
// With a [LocalAcquirer] attached, a top-level Update that leaves the
// chat runnable and unowned may acquire it in the same transaction. It
// then publishes the owned row and no ownership hint, and starts the
// acquirer once after commit.
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
	acquirer := m.acquirer
	if insideUpdate(ctx) {
		// The outer Update owns the commit, so a local acquire would
		// start the chat before it is committed.
		acquirer = nil
	} else {
		ctx = context.WithValue(ctx, updateInProgressKey{}, struct{}{})
	}

	buffer := NewPublishBuffer(m.publisher)
	defer buffer.Discard()

	var acquired *AcquireInput
	err := m.store.InTx(func(store database.Store) error {
		acquired = nil
		if _, err := store.LockChatAndBumpSnapshotVersion(ctx, m.chatID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			return xerrors.Errorf("lock chat and bump snapshot: %w", err)
		}
		tx := &Tx{
			ctx:    ctx,
			store:  store,
			chatID: m.chatID,
		}
		if err := fn(tx, store); err != nil {
			return err
		}
		chat, state, err := tx.loadState()
		if err != nil {
			return err
		}
		if acquirer != nil && state.IsRunnable() {
			chat, acquired, err = acquireLocally(tx, acquirer, chat)
			if err != nil {
				return err
			}
		}
		if err := buffer.Publish(
			coderdpubsub.ChatStateUpdateChannel(chat.ID),
			buildChatUpdateMessage(chat),
		); err != nil {
			return xerrors.Errorf("buffer chat update: %w", err)
		}
		// A local acquire just wrote a fresh lease.
		if state.IsRunnable() && acquired == nil {
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
	flushErr := buffer.Flush()
	if acquired != nil {
		// The chat is committed as ours even if a publication failed, so
		// start it rather than leave the lease to expire.
		acquirer.StartAcquired(m.chatID, *acquired)
	}
	return flushErr
}

// updateInProgressKey marks a context passed into a [ChatMachine.Update]
// transaction.
type updateInProgressKey struct{}

func insideUpdate(ctx context.Context) bool {
	return ctx.Value(updateInProgressKey{}) != nil
}

// acquireLocally acquires an unowned runnable chat for acquirer and
// returns the post-acquire row, or returns chat unchanged when the chat is
// owned or acquirer declines. Stale-owner takeovers stay on the worker
// path, which rechecks the lease under the admission lock.
func acquireLocally(tx *Tx, acquirer LocalAcquirer, chat database.Chat) (database.Chat, *AcquireInput, error) {
	if chat.WorkerID.Valid || chat.RunnerID.Valid {
		return chat, nil, nil
	}
	input, ok, err := acquirer.PrepareAcquire(tx.ctx, tx.store, chat)
	if err != nil {
		return database.Chat{}, nil, xerrors.Errorf("prepare local acquire: %w", err)
	}
	if !ok {
		return chat, nil, nil
	}
	result, err := tx.acquireFrom(chat, input)
	if err != nil {
		return database.Chat{}, nil, xerrors.Errorf("local acquire: %w", err)
	}
	return result.Chat, &input, nil
}

// Lock locks the chat row with FOR UPDATE and runs fn in a
// transaction without advancing snapshot_version. It uses the store
// captured by [NewChatMachine]. Use it when the caller needs a
// consistent chat snapshot plus related rows such as messages or
// queued messages but is NOT applying a transition.
//
// Callers must not pass a store here; it belongs on the machine.
//
// Lock publishes nothing. Callback errors roll back the transaction
// and propagate to the caller.
func (m *ChatMachine) Lock(
	ctx context.Context,
	fn func(database.Store) error,
) error {
	if m.store == nil {
		return xerrors.New("chatstate: ChatMachine has nil store")
	}
	return m.store.InTx(func(store database.Store) error {
		// GetChatByIDForUpdate locks the row WITHOUT bumping snapshot.
		_, err := store.GetChatByIDForUpdate(ctx, m.chatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			return xerrors.Errorf("lock chat: %w", err)
		}
		return fn(store)
	}, nil)
}

// ReadSnapshot runs fn in a read-only REPEATABLE READ transaction without
// advancing snapshot_version. It uses the store captured by
// [NewChatMachine]. Use it when the caller needs a consistent chat
// snapshot plus related rows such as messages or queued messages but is
// NOT applying a transition.
//
// Snapshot isolation gives fn a consistent multi-statement view without
// taking any row lock, so it never blocks (or is blocked by) the FOR
// UPDATE writers in [ChatMachine.Update] and [ChatMachine.Lock]. A
// transition committing mid-read is simply not visible to this snapshot;
// callers reconcile ordering via snapshot_version, so an older snapshot
// triggers a later refetch rather than an inconsistent read.
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
