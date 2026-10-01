package chatstate

import (
	"database/sql"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// Commit writes are the statements that update the chats row and advance
// snapshot_version. Each transition ends with one, after its staged
// writes, and it records the history and queue changes those writes
// flagged (see staged.go). Update issues BumpChatSnapshotVersion when the
// callback made no commit write.

// executionStateUpdate is the input to commitExecutionState, so
// transition methods do not repeat the UpdateChatExecutionState
// boilerplate.
// The state machine writes status, archived, last_error, ownership
// identifiers, the requires-action deadline, and the manual
// compaction request marker as one atomic update. That update is the
// transition's commit write: it advances snapshot_version and records
// the history and queue changes the transaction made so far.
//
// CompactionRequestedAt is one-shot by construction: leaving it at
// its zero value clears any pending manual compaction request, so a
// stale request can never replay on a later turn. Transitions that
// must keep a pending request alive (archive toggles, ownership
// changes, queue appends) explicitly carry the current value forward.
type executionStateUpdate struct {
	Status                   database.ChatStatus
	Archived                 bool
	WorkerID                 uuid.NullUUID
	RunnerID                 uuid.NullUUID
	LastError                pqtype.NullRawMessage
	RequiresActionDeadlineAt sql.NullTime
	CompactionRequestedAt    sql.NullTime
	GrantHistoryEpoch        bool
}

// commitExecutionState issues the transition's commit write.
func (tx *Tx) commitExecutionState(u executionStateUpdate) (database.Chat, error) {
	historyChanged, queueChanged := tx.takeVersionFlags()
	updated, err := tx.store.UpdateChatExecutionState(tx.ctx, database.UpdateChatExecutionStateParams{
		ID:                       tx.chatID,
		Status:                   u.Status,
		Archived:                 u.Archived,
		WorkerID:                 u.WorkerID,
		RunnerID:                 u.RunnerID,
		LastError:                u.LastError,
		RequiresActionDeadlineAt: u.RequiresActionDeadlineAt,
		CompactionRequestedAt:    u.CompactionRequestedAt,
		HistoryChanged:           u.GrantHistoryEpoch || historyChanged,
		QueueChanged:             queueChanged,
		StaleSeconds:             HeartbeatStaleSeconds,
	})
	if err != nil {
		return database.Chat{}, err
	}
	tx.recordCommit(updated.Chat, updated.HasQueued, updated.OwnershipStale)
	return updated.Chat, nil
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
