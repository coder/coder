package chatd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

// executionRecorder persists execute tool call execution records in
// the chat_tool_call_executions table so a retried task attempt can
// re-attach to a process started by a previous attempt.
type executionRecorder struct {
	db           database.Store
	chatID       uuid.UUID
	workspaceCtx *turnWorkspaceContext
	logger       slog.Logger
}

var _ chattool.ExecutionRecorder = (*executionRecorder)(nil)

func (server *Server) newExecutionRecorder(chatID uuid.UUID, workspaceCtx *turnWorkspaceContext) *executionRecorder {
	return &executionRecorder{
		db:           server.db,
		chatID:       chatID,
		workspaceCtx: workspaceCtx,
		logger:       server.logger,
	}
}

// Reserve inserts the execution record for the tool call, or returns
// the existing record when a previous attempt already reserved it.
// The unique violation on (chat_id, tool_call_id) is the
// compare-and-set that keeps two concurrent attempt owners from both
// acting as creator.
func (r *executionRecorder) Reserve(ctx context.Context, toolCallID string, command string, background bool, timeout time.Duration) (chattool.ExecutionRecord, bool, error) {
	row, err := r.db.InsertChatToolCallExecution(ctx, database.InsertChatToolCallExecutionParams{
		ChatID:     r.chatID,
		ToolCallID: toolCallID,
		Command:    command,
		Background: background,
		// Round up so a retry never reconstructs a shorter
		// deadline than the original attempt used; a sub-second
		// timeout would otherwise floor to 0s and make re-attach
		// treat a running process as instantly timed out.
		TimeoutSecs: int64((timeout + time.Second - 1) / time.Second),
		CreatedAt:   dbtime.Now(),
	})
	if err == nil {
		return executionRecordFromRow(row), true, nil
	}
	if !database.IsUniqueViolation(err) {
		return chattool.ExecutionRecord{}, false, xerrors.Errorf("insert chat tool call execution: %w", err)
	}
	row, err = r.db.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:     r.chatID,
		ToolCallID: toolCallID,
	})
	if err != nil {
		return chattool.ExecutionRecord{}, false, xerrors.Errorf("get chat tool call execution: %w", err)
	}
	if row.Command != command {
		// Diagnostics only; the tool call ID remains the key.
		r.logger.Warn(ctx, "execution record command mismatch",
			slog.F("chat_id", r.chatID),
			slog.F("tool_call_id", toolCallID),
		)
	}
	return executionRecordFromRow(row), false, nil
}

// RecordStart stores the process handle and the agent that owns it
// on the reserved record. The agent comes from the cached
// connection that just served StartProcess, not a latest-agent
// lookup, so the interrupt kill path signals the agent that is
// actually running the process even when the connection is pinned
// to an agent that is no longer the latest one.
func (r *executionRecorder) RecordStart(ctx context.Context, toolCallID string, processID string) error {
	agentID, ok := r.workspaceCtx.connAgentID()
	if !ok {
		return xerrors.New("no cached workspace connection to attribute the process to")
	}
	_, err := r.db.UpdateChatToolCallExecutionProcess(ctx, database.UpdateChatToolCallExecutionProcessParams{
		ChatID:           r.chatID,
		ToolCallID:       toolCallID,
		ProcessID:        processID,
		WorkspaceAgentID: agentID,
		StartedAt:        dbtime.Now(),
	})
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

func executionRecordFromRow(row database.ChatToolCallExecution) chattool.ExecutionRecord {
	rec := chattool.ExecutionRecord{
		Command:    row.Command,
		Background: row.Background,
		Timeout:    time.Duration(row.TimeoutSecs) * time.Second,
		CreatedAt:  row.CreatedAt,
	}
	if row.ProcessID.Valid {
		rec.ProcessID = row.ProcessID.String
	}
	if row.StartedAt.Valid {
		rec.StartedAt = row.StartedAt.Time
	}
	return rec
}
