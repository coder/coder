package chatd

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

// executionRecorder is the chatd implementation of the execute
// tool's execution ledger, backed by the chat_tool_call_executions
// table. Rows are addressed by lineage: the chat, the assistant
// message that issued the call, and the provider tool call ID.
// Provider tool call IDs may repeat across regenerated assistant
// messages, so the assistant message ID keeps every regeneration on
// its own row.
type executionRecorder struct {
	db     database.Store
	chatID uuid.UUID
	logger slog.Logger

	mu sync.Mutex
	// assistantMessageID is the lineage anchor for all recorder
	// operations. It is bound by the generation step before tools
	// execute.
	assistantMessageID int64
}

var _ chattool.ExecutionRecorder = (*executionRecorder)(nil)

func (server *Server) newExecutionRecorder(chatID uuid.UUID) *executionRecorder {
	return &executionRecorder{
		db:     server.db,
		chatID: chatID,
		logger: server.logger,
	}
}

// bindAssistantMessage anchors subsequent recorder operations to the
// assistant message whose tool calls are about to execute.
func (r *executionRecorder) bindAssistantMessage(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assistantMessageID = id
}

func (r *executionRecorder) lineage() (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assistantMessageID == 0 {
		return 0, xerrors.New("no assistant message bound for execution recording")
	}
	return r.assistantMessageID, nil
}

// Claim takes dispatch ownership of the tool call's ledger row via
// the claim CAS: only a reserved intent (or a missing row, for
// assistant messages that predate the ledger) is claimable. Stale
// starting claims are never taken over here because their owner may
// have dispatched a process whose handle was lost; the tool resolves
// those to unknown instead.
func (r *executionRecorder) Claim(ctx context.Context, toolCallID string, inputSHA256 string, command string, background bool, timeout time.Duration) (chattool.ExecutionRecord, bool, error) {
	msgID, err := r.lineage()
	if err != nil {
		return chattool.ExecutionRecord{}, false, err
	}
	now := dbtime.Now()
	row, err := r.db.ClaimChatToolCallExecution(ctx, database.ClaimChatToolCallExecutionParams{
		ID:                 uuid.New(),
		ChatID:             r.chatID,
		AssistantMessageID: msgID,
		ToolCallID:         toolCallID,
		InputSha256:        inputSHA256,
		Command:            command,
		Background:         background,
		// Round up so a retry never reconstructs a shorter
		// deadline than the original attempt used; a sub-second
		// timeout would otherwise floor to 0s and make re-attach
		// treat a running process as instantly timed out.
		TimeoutSecs: int64((timeout + time.Second - 1) / time.Second),
		Now:         now,
		// The zero time disables stale-claim takeover: no
		// claimed_at precedes it, so the CAS only accepts
		// reserved rows.
		StaleBefore: time.Time{},
	})
	if err == nil {
		return executionRecordFromRow(row), true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return chattool.ExecutionRecord{}, false, xerrors.Errorf("claim chat tool call execution: %w", err)
	}
	row, err = r.db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             r.chatID,
		AssistantMessageID: msgID,
		ToolCallID:         toolCallID,
	})
	if err != nil {
		return chattool.ExecutionRecord{}, false, xerrors.Errorf("get chat tool call execution: %w", err)
	}
	if row.InputSha256 != inputSHA256 {
		return chattool.ExecutionRecord{}, false, xerrors.Errorf(
			"tool call %s targets execution %s: %w", toolCallID, row.ID, chattool.ErrExecutionInputMismatch,
		)
	}
	return executionRecordFromRow(row), false, nil
}

// Get reads the current ledger row without claiming it.
func (r *executionRecorder) Get(ctx context.Context, toolCallID string) (chattool.ExecutionRecord, error) {
	msgID, err := r.lineage()
	if err != nil {
		return chattool.ExecutionRecord{}, err
	}
	row, err := r.db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             r.chatID,
		AssistantMessageID: msgID,
		ToolCallID:         toolCallID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return chattool.ExecutionRecord{}, xerrors.Errorf("tool call %s: %w", toolCallID, chattool.ErrExecutionRecordNotFound)
	}
	if err != nil {
		return chattool.ExecutionRecord{}, xerrors.Errorf("get chat tool call execution: %w", err)
	}
	return executionRecordFromRow(row), nil
}

// RecordStart stores the process identity and the agent that owns it
// on the claim that dispatched the process. The agent comes from the
// cached connection that just served StartProcess, not a
// latest-agent lookup, so the interrupt kill path signals the agent
// that is actually running the process even when the connection is
// pinned to an agent that is no longer the latest one. The claim
// epoch guard means a superseded claimer cannot overwrite the
// current claim's identity.
func (r *executionRecorder) RecordStart(ctx context.Context, toolCallID string, claimEpoch int64, processID string, agentID uuid.UUID) error {
	msgID, err := r.lineage()
	if err != nil {
		return err
	}
	if agentID == uuid.Nil {
		return xerrors.New("no workspace agent to attribute the process to")
	}
	_, err = r.db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
		ChatID:             r.chatID,
		AssistantMessageID: msgID,
		ToolCallID:         toolCallID,
		ClaimEpoch:         claimEpoch,
		ProcessID:          processID,
		WorkspaceAgentID:   agentID,
		StartedAt:          dbtime.Now(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("claim epoch %d was superseded before the process start was recorded", claimEpoch)
	}
	if err != nil {
		return xerrors.Errorf("update chat tool call execution process: %w", err)
	}
	r.logger.Debug(ctx, "recorded execute process start",
		slog.F("chat_id", r.chatID),
		slog.F("tool_call_id", toolCallID),
		slog.F("process_id", processID),
	)
	return nil
}

// terminalObservationSources lists which lifecycle states each
// tool-observable terminal status may transition from. Interrupt
// outcomes (cancel_requested, canceled) are written only by the
// interrupt path and always outrank tool observations.
var terminalObservationSources = map[chattool.ExecutionStatus][]database.ChatToolCallExecutionStatus{
	chattool.ExecutionStatusExited: {
		database.ChatToolCallExecutionStatusStarting,
		database.ChatToolCallExecutionStatusRunning,
		database.ChatToolCallExecutionStatusDetached,
	},
	chattool.ExecutionStatusDetached: {
		database.ChatToolCallExecutionStatusStarting,
		database.ChatToolCallExecutionStatusRunning,
	},
	chattool.ExecutionStatusUnknown: {
		database.ChatToolCallExecutionStatusReserved,
		database.ChatToolCallExecutionStatusStarting,
		database.ChatToolCallExecutionStatusRunning,
		database.ChatToolCallExecutionStatusExited,
		database.ChatToolCallExecutionStatusDetached,
	},
	chattool.ExecutionStatusNoEffect: {
		database.ChatToolCallExecutionStatusReserved,
		database.ChatToolCallExecutionStatusStarting,
	},
}

// MarkTerminal applies a tool lifecycle observation. A zero-row
// update means the row is already in a state that outranks the
// observation (for example the interrupt reconciler resolved it);
// the observation is dropped, not forced.
func (r *executionRecorder) MarkTerminal(ctx context.Context, toolCallID string, status chattool.ExecutionStatus) error {
	sources, ok := terminalObservationSources[status]
	if !ok {
		return xerrors.Errorf("status %q is not a tool-observable terminal status", status)
	}
	msgID, err := r.lineage()
	if err != nil {
		return err
	}
	_, err = r.db.UpdateChatToolCallExecutionStatus(ctx, database.UpdateChatToolCallExecutionStatusParams{
		ChatID:             r.chatID,
		AssistantMessageID: msgID,
		ToolCallID:         toolCallID,
		Status:             database.ChatToolCallExecutionStatus(status),
		FromStatuses:       sources,
		UpdatedAt:          dbtime.Now(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		r.logger.Debug(ctx, "execution lifecycle observation dropped",
			slog.F("chat_id", r.chatID),
			slog.F("tool_call_id", toolCallID),
			slog.F("status", string(status)),
		)
		return nil
	}
	if err != nil {
		return xerrors.Errorf("update chat tool call execution status: %w", err)
	}
	return nil
}

// MarkStaleClaimUnknown resolves a stale starting claim to unknown.
// Only starting rows match: a concurrent RecordStart outranks the
// staleness verdict, so the caller gets the fresh row back and
// resumes from it instead of downgrading a running process.
func (r *executionRecorder) MarkStaleClaimUnknown(ctx context.Context, toolCallID string) (chattool.ExecutionRecord, bool, error) {
	msgID, err := r.lineage()
	if err != nil {
		return chattool.ExecutionRecord{}, false, err
	}
	row, err := r.db.UpdateChatToolCallExecutionStatus(ctx, database.UpdateChatToolCallExecutionStatusParams{
		ChatID:             r.chatID,
		AssistantMessageID: msgID,
		ToolCallID:         toolCallID,
		Status:             database.ChatToolCallExecutionStatusUnknown,
		FromStatuses:       []database.ChatToolCallExecutionStatus{database.ChatToolCallExecutionStatusStarting},
		UpdatedAt:          dbtime.Now(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		latest, getErr := r.Get(ctx, toolCallID)
		if getErr != nil {
			return chattool.ExecutionRecord{}, false, getErr
		}
		return latest, false, nil
	}
	if err != nil {
		return chattool.ExecutionRecord{}, false, xerrors.Errorf("mark stale execution claim unknown: %w", err)
	}
	return executionRecordFromRow(row), true, nil
}

func executionRecordFromRow(row database.ChatToolCallExecution) chattool.ExecutionRecord {
	rec := chattool.ExecutionRecord{
		ID:         row.ID.String(),
		Status:     chattool.ExecutionStatus(row.Status),
		Command:    row.Command,
		Background: row.Background,
		Timeout:    time.Duration(row.TimeoutSecs) * time.Second,
		ClaimEpoch: row.ClaimEpoch,
	}
	if row.ProcessID.Valid {
		rec.ProcessID = row.ProcessID.String
	}
	if row.ClaimedAt.Valid {
		rec.ClaimedAt = row.ClaimedAt.Time
	}
	if row.StartedAt.Valid {
		rec.StartedAt = row.StartedAt.Time
	}
	if row.WorkspaceAgentID.Valid {
		rec.WorkspaceAgentID = row.WorkspaceAgentID.UUID
	}
	return rec
}
