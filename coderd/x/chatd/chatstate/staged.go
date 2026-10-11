package chatstate

import (
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// Staged writes change chat_messages or chat_queued_messages without
// touching the chats row. Each sets historyChanged or queueChanged so the
// transaction's commit write (see commit.go) records the change; no
// trigger does. They must run before that commit write, or
// [ChatMachine.Update] fails with [ErrStagedAfterCommitWrite]. The
// automation promotion guard's stage* helpers live in automation.go.

// stageInsertMessages inserts the given Message batch under the current
// chat. It sets historyChanged because no trigger records the change;
// the commit write must.
func (tx *Tx) stageInsertMessages(messages []Message) ([]database.ChatMessage, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	inserted, err := tx.store.InsertChatMessages(tx.ctx, toInsertParams(tx.chatID, messages))
	if err != nil {
		return nil, xerrors.Errorf("insert messages: %w", err)
	}
	tx.historyChanged = true
	return fromInsertedRows(inserted), nil
}

// stageSoftDeleteMessageAndSuffix soft-deletes the target message and every
// message after it. It sets historyChanged because no trigger records
// the change; the commit write must.
func (tx *Tx) stageSoftDeleteMessageAndSuffix(targetID int64) error {
	if err := tx.store.SoftDeleteChatMessageByID(tx.ctx, targetID); err != nil {
		return xerrors.Errorf("soft-delete target: %w", err)
	}
	if err := tx.store.SoftDeleteChatMessagesAfterID(tx.ctx, database.SoftDeleteChatMessagesAfterIDParams{
		ChatID:  tx.chatID,
		AfterID: targetID,
	}); err != nil {
		return xerrors.Errorf("soft-delete suffix: %w", err)
	}
	tx.historyChanged = true
	return nil
}

// stageRemoveQueuedMessage deletes one queued message. It sets queueChanged
// when a row was removed because no trigger records the change; the
// commit write must.
func (tx *Tx) stageRemoveQueuedMessage(id int64) (int64, error) {
	rows, err := tx.store.DeleteChatQueuedMessageReturningCount(tx.ctx, database.DeleteChatQueuedMessageReturningCountParams{
		ID:     id,
		ChatID: tx.chatID,
	})
	if err != nil {
		return 0, err
	}
	if rows > 0 {
		tx.queueChanged = true
	}
	return rows, nil
}

// stageClearQueue deletes all queued messages on the chat and returns the
// IDs that were deleted in queue order.
func (tx *Tx) stageClearQueue() ([]int64, error) {
	queued, err := tx.store.GetChatQueuedMessagesByPosition(tx.ctx, tx.chatID)
	if err != nil {
		return nil, xerrors.Errorf("get queued for clear: %w", err)
	}
	if len(queued) == 0 {
		return nil, nil
	}
	if _, err := tx.store.DeleteAllChatQueuedMessagesReturningCount(tx.ctx, tx.chatID); err != nil {
		return nil, xerrors.Errorf("delete queued: %w", err)
	}
	tx.queueChanged = true
	ids := make([]int64, len(queued))
	for i, q := range queued {
		ids[i] = q.ID
	}
	return ids, nil
}

// stageInsertQueuedMessage inserts a queued user message. created_by falls
// back to chats.owner_id only when the message does not supply one.
func (tx *Tx) stageInsertQueuedMessage(ownerFallback uuid.UUID, m Message, maxQueueSize int) (database.ChatQueuedMessage, error) {
	createdBy := ownerFallback
	if m.CreatedBy.Valid {
		createdBy = m.CreatedBy.UUID
	}
	rawContent := m.Content.RawMessage
	if !m.Content.Valid || len(rawContent) == 0 {
		rawContent = json.RawMessage("null")
	}
	if err := tx.requireQueueCapacity(maxQueueSize); err != nil {
		return database.ChatQueuedMessage{}, err
	}
	var (
		automationID    uuid.NullUUID
		inputID         uuid.NullUUID
		queueGeneration sql.NullInt64
	)
	if m.Automation != nil {
		if err := m.Automation.validate(); err != nil {
			return database.ChatQueuedMessage{}, err
		}
		automationID = uuid.NullUUID{UUID: m.Automation.AutomationID, Valid: true}
		inputID = uuid.NullUUID{UUID: m.Automation.InputID, Valid: true}
		queueGeneration = sql.NullInt64{Int64: m.Automation.QueueGeneration, Valid: true}
	}
	queued, err := tx.store.InsertChatQueuedMessageWithCreator(tx.ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:          tx.chatID,
		Content:         rawContent,
		ModelConfigID:   m.ModelConfigID,
		ReasoningEffort: m.ReasoningEffort,
		CreatedBy:       createdBy,
		AutomationID:    automationID,
		InputID:         inputID,
		QueueGeneration: queueGeneration,
	})
	if err != nil {
		return database.ChatQueuedMessage{}, err
	}
	tx.queueChanged = true
	return queued, nil
}

// stageDeletePromotedQueuedMessage deletes the queue row a promotion just
// copied into history. The row was read under the chat row lock, so any
// count other than one means the queue changed underneath the lock; the
// error rolls back the promotion instead of leaving a duplicate or a
// message linked to a row that was never removed.
func (tx *Tx) stageDeletePromotedQueuedMessage(id int64) error {
	rows, err := tx.stageRemoveQueuedMessage(id)
	if err != nil {
		return err
	}
	if rows != 1 {
		return xerrors.Errorf("promoted queued message %d: deleted %d rows, want 1", id, rows)
	}
	return nil
}

// stageReorderQueuedMessageToHead moves a queued message to the queue
// head. It sets queueChanged only when the order changed.
func (tx *Tx) stageReorderQueuedMessageToHead(id int64) error {
	moved, err := tx.store.ReorderChatQueuedMessageToHead(tx.ctx, database.ReorderChatQueuedMessageToHeadParams{
		ID:     id,
		ChatID: tx.chatID,
	})
	if err != nil {
		return err
	}
	if moved > 0 {
		tx.queueChanged = true
	}
	return nil
}
