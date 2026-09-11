package dbtestutil

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// chatWriteRejection records one guarded write that ran without a snapshot
// allocation for its chat.
type chatWriteRejection struct {
	method string
	chatID uuid.UUID
}

// chatWriteRecorder collects every rejection made by the guards that share
// it. NewDB owns one recorder per test database and fails the test at
// cleanup for each recorded rejection, so a rejection fails the test even
// when the caller drops the returned error.
type chatWriteRecorder struct {
	mu         sync.Mutex
	rejections []chatWriteRejection
}

// add records a rejection and returns the error the guarded method hands
// back to its caller.
func (r *chatWriteRecorder) add(method string, chatID uuid.UUID) error {
	r.mu.Lock()
	r.rejections = append(r.rejections, chatWriteRejection{method: method, chatID: chatID})
	r.mu.Unlock()
	return xerrors.Errorf("%s for chat %s outside a chat state transition (no snapshot allocated in this transaction); "+
		"write history through chatstate.ChatMachine.Update or CreateChat, or dbgen in tests", method, chatID)
}

// list returns a copy of the recorded rejections.
func (r *chatWriteRecorder) list() []chatWriteRejection {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]chatWriteRejection, len(r.rejections))
	copy(out, r.rejections)
	return out
}

// reset discards the recorded rejections.
func (r *chatWriteRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rejections = nil
}

// chatWriteGuard is a database.Store that rejects writes to chat_messages
// and chat_queued_messages unless the same transaction allocated a snapshot
// for the chat first, through InsertChat or LockChatAndBumpSnapshotVersion.
// Production satisfies this by construction: chatstate.CreateChat inserts
// the chat row and chatstate.ChatMachine.Update bumps the snapshot version
// before any history write. Test fixtures satisfy it through dbgen, which
// allocates inside its own transaction.
//
// The root handle never allocates, so every guarded write on it is
// rejected. Each transaction started through InTx gets its own allocation
// set; a nested InTx that reuses the outer transaction shares the outer set.
// The guard wraps the store returned by database.New directly, so the
// nested check compares the transaction store by pointer identity.
type chatWriteGuard struct {
	database.Store
	rec *chatWriteRecorder
	// allocated is nil on the root handle and holds the chats whose snapshot
	// this transaction allocated otherwise. mu guards it because a Store,
	// including a transaction handle, may be used from several goroutines.
	mu        sync.Mutex
	allocated map[uuid.UUID]struct{}
}

func newChatWriteGuard(store database.Store, rec *chatWriteRecorder) *chatWriteGuard {
	return &chatWriteGuard{Store: store, rec: rec}
}

func (g *chatWriteGuard) Wrappers() []string {
	return append(g.Store.Wrappers(), "dbtestutil.chatWriteGuard")
}

func (g *chatWriteGuard) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return g.Store.InTx(func(tx database.Store) error {
		if tx == g.Store {
			return fn(g)
		}
		return fn(&chatWriteGuard{Store: tx, rec: g.rec, allocated: map[uuid.UUID]struct{}{}})
	}, opts)
}

func (g *chatWriteGuard) mark(chatID uuid.UUID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.allocated != nil {
		g.allocated[chatID] = struct{}{}
	}
}

// require returns the rejection error unless this transaction allocated a
// snapshot for chatID.
func (g *chatWriteGuard) require(method string, chatID uuid.UUID) error {
	g.mu.Lock()
	_, ok := g.allocated[chatID]
	g.mu.Unlock()
	if ok {
		return nil
	}
	return g.rec.add(method, chatID)
}

func (g *chatWriteGuard) InsertChat(ctx context.Context, arg database.InsertChatParams) (database.Chat, error) {
	chat, err := g.Store.InsertChat(ctx, arg)
	if err == nil {
		g.mark(chat.ID)
	}
	return chat, err
}

func (g *chatWriteGuard) LockChatAndBumpSnapshotVersion(ctx context.Context, id uuid.UUID) (database.Chat, error) {
	chat, err := g.Store.LockChatAndBumpSnapshotVersion(ctx, id)
	if err == nil {
		g.mark(id)
	}
	return chat, err
}

func (g *chatWriteGuard) InsertChatMessages(ctx context.Context, arg database.InsertChatMessagesParams) ([]database.InsertChatMessagesRow, error) {
	if err := g.require("InsertChatMessages", arg.ChatID); err != nil {
		return nil, err
	}
	return g.Store.InsertChatMessages(ctx, arg)
}

// SoftDeleteChatMessageByID resolves the chat from the message because the
// parameters carry no chat id. A missing message passes through unchecked
// because the underlying update affects no row.
func (g *chatWriteGuard) SoftDeleteChatMessageByID(ctx context.Context, id int64) error {
	msg, err := g.GetChatMessageByID(ctx, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("resolve chat for message %d: %w", id, err)
	}
	if err == nil {
		if err := g.require("SoftDeleteChatMessageByID", msg.ChatID); err != nil {
			return err
		}
	}
	return g.Store.SoftDeleteChatMessageByID(ctx, id)
}

func (g *chatWriteGuard) SoftDeleteChatMessagesAfterID(ctx context.Context, arg database.SoftDeleteChatMessagesAfterIDParams) error {
	if err := g.require("SoftDeleteChatMessagesAfterID", arg.ChatID); err != nil {
		return err
	}
	return g.Store.SoftDeleteChatMessagesAfterID(ctx, arg)
}

func (g *chatWriteGuard) SoftDeleteContextFileMessages(ctx context.Context, chatID uuid.UUID) error {
	if err := g.require("SoftDeleteContextFileMessages", chatID); err != nil {
		return err
	}
	return g.Store.SoftDeleteContextFileMessages(ctx, chatID)
}

func (g *chatWriteGuard) InsertChatQueuedMessage(ctx context.Context, arg database.InsertChatQueuedMessageParams) (database.ChatQueuedMessage, error) {
	if err := g.require("InsertChatQueuedMessage", arg.ChatID); err != nil {
		return database.ChatQueuedMessage{}, err
	}
	return g.Store.InsertChatQueuedMessage(ctx, arg)
}

func (g *chatWriteGuard) InsertChatQueuedMessageWithCreator(ctx context.Context, arg database.InsertChatQueuedMessageWithCreatorParams) (database.ChatQueuedMessage, error) {
	if err := g.require("InsertChatQueuedMessageWithCreator", arg.ChatID); err != nil {
		return database.ChatQueuedMessage{}, err
	}
	return g.Store.InsertChatQueuedMessageWithCreator(ctx, arg)
}

func (g *chatWriteGuard) DeleteChatQueuedMessage(ctx context.Context, arg database.DeleteChatQueuedMessageParams) error {
	if err := g.require("DeleteChatQueuedMessage", arg.ChatID); err != nil {
		return err
	}
	return g.Store.DeleteChatQueuedMessage(ctx, arg)
}

func (g *chatWriteGuard) DeleteChatQueuedMessageReturningCount(ctx context.Context, arg database.DeleteChatQueuedMessageReturningCountParams) (int64, error) {
	if err := g.require("DeleteChatQueuedMessageReturningCount", arg.ChatID); err != nil {
		return 0, err
	}
	return g.Store.DeleteChatQueuedMessageReturningCount(ctx, arg)
}

func (g *chatWriteGuard) DeleteAllChatQueuedMessages(ctx context.Context, chatID uuid.UUID) error {
	if err := g.require("DeleteAllChatQueuedMessages", chatID); err != nil {
		return err
	}
	return g.Store.DeleteAllChatQueuedMessages(ctx, chatID)
}

func (g *chatWriteGuard) DeleteAllChatQueuedMessagesReturningCount(ctx context.Context, chatID uuid.UUID) (int64, error) {
	if err := g.require("DeleteAllChatQueuedMessagesReturningCount", chatID); err != nil {
		return 0, err
	}
	return g.Store.DeleteAllChatQueuedMessagesReturningCount(ctx, chatID)
}

func (g *chatWriteGuard) PopNextQueuedMessage(ctx context.Context, chatID uuid.UUID) (database.ChatQueuedMessage, error) {
	if err := g.require("PopNextQueuedMessage", chatID); err != nil {
		return database.ChatQueuedMessage{}, err
	}
	return g.Store.PopNextQueuedMessage(ctx, chatID)
}

func (g *chatWriteGuard) ReorderChatQueuedMessageToFront(ctx context.Context, arg database.ReorderChatQueuedMessageToFrontParams) (int64, error) {
	if err := g.require("ReorderChatQueuedMessageToFront", arg.ChatID); err != nil {
		return 0, err
	}
	return g.Store.ReorderChatQueuedMessageToFront(ctx, arg)
}

func (g *chatWriteGuard) ReorderChatQueuedMessageToHead(ctx context.Context, arg database.ReorderChatQueuedMessageToHeadParams) (int64, error) {
	if err := g.require("ReorderChatQueuedMessageToHead", arg.ChatID); err != nil {
		return 0, err
	}
	return g.Store.ReorderChatQueuedMessageToHead(ctx, arg)
}
