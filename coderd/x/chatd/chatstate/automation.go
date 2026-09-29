package chatstate

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// AutomationProvenance identifies the automation input that admitted a
// message. It is copied onto the queued row (all three fields) and onto
// the history row (AutomationID and InputID) that carries the message.
type AutomationProvenance struct {
	AutomationID uuid.UUID
	InputID      uuid.UUID
	// QueueGeneration is the automation's queue_generation at admission.
	// A queued row whose generation no longer matches the automation is
	// stale and is dropped instead of promoted.
	QueueGeneration int64
}

func (p AutomationProvenance) validate() error {
	if p.AutomationID == uuid.Nil {
		return xerrors.New("automation provenance: automation ID is required")
	}
	if p.InputID == uuid.Nil {
		return xerrors.New("automation provenance: input ID is required")
	}
	if p.QueueGeneration < 1 {
		return xerrors.Errorf("automation provenance: queue generation must be positive, got %d", p.QueueGeneration)
	}
	return nil
}

// AdmitFunc admits an automation message inside the transaction that
// writes it. It runs after the chat row is locked (or inserted, for chat
// creation) and before the message is written. A returned error rolls
// back the whole transaction, so nothing the caller wrote survives.
//
// The callback must use only the supplied transaction store, must not
// make network calls, and must lock automations only through
// [LockAutomations] so every automation lock follows one order: the chat
// row first, then automations in ascending id order.
type AdmitFunc func(ctx context.Context, store database.Store, chatID uuid.UUID) (AutomationProvenance, error)

// admit runs fn and validates the provenance it returns.
func admit(ctx context.Context, fn AdmitFunc, store database.Store, chatID uuid.UUID) (AutomationProvenance, error) {
	provenance, err := fn(ctx, store, chatID)
	if err != nil {
		return AutomationProvenance{}, err
	}
	if err := provenance.validate(); err != nil {
		return AutomationProvenance{}, err
	}
	return provenance, nil
}

// updateAttemptKey carries the *updateAttempt of the ChatMachine.Update
// attempt that owns the current transaction.
type updateAttemptKey struct{}

// updateAttempt records what one ChatMachine.Update attempt did that
// affects whether the attempt may be retried.
type updateAttempt struct {
	automationLocks bool
}

func updateAttemptFromContext(ctx context.Context) (*updateAttempt, bool) {
	attempt, ok := ctx.Value(updateAttemptKey{}).(*updateAttempt)
	return attempt, ok
}

// LockAutomations locks the given automation rows FOR UPDATE in
// ascending id order and returns the rows that exist, keyed by id.
// Duplicate ids are ignored and missing ids are simply absent from the
// result. Callers must already hold the chat row lock.
//
// It runs as chatd because promotion can be triggered by any chat
// collaborator or by the worker, and the lock must never be hidden by
// the caller's permissions.
func LockAutomations(ctx context.Context, store database.Store, ids []uuid.UUID) (map[uuid.UUID]database.ChatAutomation, error) {
	ids = slices.Clone(ids)
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	ids = slices.Compact(ids)
	locked := make(map[uuid.UUID]database.ChatAutomation, len(ids))
	if len(ids) == 0 {
		return locked, nil
	}
	if attempt, ok := updateAttemptFromContext(ctx); ok {
		// Mark before the query: the lock wait is where a deadlock
		// abort surfaces.
		attempt.automationLocks = true
	}
	//nolint:gocritic // Promotion must see every automation regardless of who triggered it.
	rows, err := store.GetChatAutomationsByIDsForUpdate(dbauthz.AsChatd(ctx), ids)
	if err != nil {
		return nil, xerrors.Errorf("lock chat automations: %w", err)
	}
	for _, row := range rows {
		locked[row.ID] = row
	}
	return locked, nil
}

// queuedAutomationIDs returns the automation ids referenced by rows.
func queuedAutomationIDs(rows []database.ChatQueuedMessage) []uuid.UUID {
	var ids []uuid.UUID
	for _, row := range rows {
		if row.AutomationID.Valid {
			ids = append(ids, row.AutomationID.UUID)
		}
	}
	return ids
}

// queuedRowPromotable reports whether row may be promoted. Rows without
// an automation always pass. Automation rows pass only when their
// automation exists, is enabled, and still has the queue generation the
// row was queued with.
func queuedRowPromotable(row database.ChatQueuedMessage, locked map[uuid.UUID]database.ChatAutomation) bool {
	if !row.AutomationID.Valid {
		return true
	}
	automation, ok := locked[row.AutomationID.UUID]
	if !ok || !automation.Enabled {
		return false
	}
	return row.QueueGeneration.Valid && row.QueueGeneration.Int64 == automation.QueueGeneration
}

// dropUnpromotable deletes the rows that fail the promotion guard and
// returns the passing rows in their original order. The automations of
// the automation rows must already be locked and present in locked.
func (tx *Tx) dropUnpromotable(
	rows []database.ChatQueuedMessage,
	locked map[uuid.UUID]database.ChatAutomation,
) ([]database.ChatQueuedMessage, error) {
	passing := make([]database.ChatQueuedMessage, 0, len(rows))
	for _, row := range rows {
		if queuedRowPromotable(row, locked) {
			passing = append(passing, row)
			continue
		}
		count, err := tx.store.DeleteChatQueuedMessageReturningCount(tx.ctx, database.DeleteChatQueuedMessageReturningCountParams{
			ID:     row.ID,
			ChatID: tx.chatID,
		})
		if err != nil {
			return nil, xerrors.Errorf("delete unpromotable queued message %d: %w", row.ID, err)
		}
		if count != 1 {
			return nil, xerrors.Errorf("delete unpromotable queued message %d: deleted %d rows, want 1", row.ID, count)
		}
	}
	return passing, nil
}

// guardQueuedRows is the queue promotion guard. Every transition that
// promotes queued rows must pass the rows it is about to promote
// through it. Rows without an automation pass without any database
// access. The automations of the remaining rows are locked in ascending
// id order; rows whose automation is missing, disabled, or on another
// queue generation are deleted and left out of the result. Passing rows
// keep their order.
func (tx *Tx) guardQueuedRows(rows []database.ChatQueuedMessage) ([]database.ChatQueuedMessage, error) {
	ids := queuedAutomationIDs(rows)
	if len(ids) == 0 {
		return rows, nil
	}
	locked, err := LockAutomations(tx.ctx, tx.store, ids)
	if err != nil {
		return nil, err
	}
	return tx.dropUnpromotable(rows, locked)
}

// nextPromotableQueueHead returns the queue head that the next
// head-promoting transition should promote, or false when no queued row
// can be promoted. Heads that fail the promotion guard are deleted until
// a passing head is found; rows behind it are untouched.
//
// A head without an automation is returned with no extra queries. When
// the head has an automation, the automations of every row in the queue
// are locked up front, so a loop over several stale heads never takes
// automation locks out of ascending order.
func (tx *Tx) nextPromotableQueueHead() (database.ChatQueuedMessage, bool, error) {
	head, err := tx.store.GetChatQueuedMessageHead(tx.ctx, tx.chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatQueuedMessage{}, false, nil
	}
	if err != nil {
		return database.ChatQueuedMessage{}, false, xerrors.Errorf("get queue head: %w", err)
	}
	if !head.AutomationID.Valid {
		return head, true, nil
	}
	queue, err := tx.store.GetChatQueuedMessagesByPosition(tx.ctx, tx.chatID)
	if err != nil {
		return database.ChatQueuedMessage{}, false, xerrors.Errorf("get queued messages: %w", err)
	}
	locked, err := LockAutomations(tx.ctx, tx.store, queuedAutomationIDs(queue))
	if err != nil {
		return database.ChatQueuedMessage{}, false, err
	}
	for _, row := range queue {
		passing, err := tx.dropUnpromotable([]database.ChatQueuedMessage{row}, locked)
		if err != nil {
			return database.ChatQueuedMessage{}, false, err
		}
		if len(passing) == 1 {
			return row, true, nil
		}
	}
	return database.ChatQueuedMessage{}, false, nil
}
