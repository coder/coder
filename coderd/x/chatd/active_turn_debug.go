package chatd

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
)

type runnerDebugTurn struct {
	runnerCtx context.Context
	logger    slog.Logger

	mu sync.Mutex

	runContext  chatdebug.RunContext
	seedSummary map[string]any
	service     *chatdebug.Service

	created   bool
	disabled  bool
	finalized bool

	status    chatdebug.Status
	statusSet bool

	heartbeatDone chan struct{}
}

func newRunnerDebugTurn(runnerCtx context.Context, logger slog.Logger) *runnerDebugTurn {
	return &runnerDebugTurn{
		runnerCtx: runnerCtx,
		logger:    logger,
	}
}

func (d *runnerDebugTurn) Ensure(
	ctx context.Context,
	chat database.Chat,
	debug *generationDebug,
) context.Context {
	if d == nil {
		return ctx
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check finalized/disabled before created: once the turn is
	// finalized, new contexts must not be attributed to the
	// finalized run, even if it was created earlier.
	if d.disabled || d.finalized {
		return ctx
	}
	if d.created {
		return d.contextLocked(ctx)
	}
	if debug == nil || !debug.Enabled || debug.Service == nil ||
		chat.ID == uuid.Nil || debug.TriggerMessageID == 0 {
		d.disabled = true
		return ctx
	}

	seedSummary := chatdebug.SeedSummary(
		chatdebug.TruncateLabel(debug.TriggerLabel, chatdebug.MaxLabelLength),
	)
	rootChatID := uuid.Nil
	if chat.RootChatID.Valid {
		rootChatID = chat.RootChatID.UUID
	}
	parentChatID := uuid.Nil
	if chat.ParentChatID.Valid {
		parentChatID = chat.ParentChatID.UUID
	}

	createRunCtx, createRunCancel := context.WithTimeout(
		context.WithoutCancel(ctx), debugCreateRunTimeout,
	)
	run, createRunErr := debug.Service.CreateRun(createRunCtx, chatdebug.CreateRunParams{
		ChatID:              chat.ID,
		RootChatID:          rootChatID,
		ParentChatID:        parentChatID,
		ModelConfigID:       debug.ModelConfig.ID,
		TriggerMessageID:    debug.TriggerMessageID,
		HistoryTipMessageID: debug.HistoryTipMessageID,
		Kind:                chatdebug.KindChatTurn,
		Status:              chatdebug.StatusInProgress,
		Provider:            debug.Provider,
		Model:               debug.Model,
		Summary:             seedSummary,
	})
	createRunCancel()
	if createRunErr != nil {
		d.disabled = true
		d.logger.Warn(ctx, "failed to create chat debug run",
			slog.F("chat_id", chat.ID),
			slog.Error(createRunErr),
		)
		return ctx
	}

	d.service = debug.Service
	d.runContext = chatdebugRunContext(run)
	d.seedSummary = seedSummary
	d.created = true
	d.heartbeatDone = make(chan struct{})
	d.service.LaunchRunHeartbeat(d.runnerCtx, d.runContext.RunID, d.runContext.ChatID, d.heartbeatDone)
	return d.contextLocked(ctx)
}

func (d *runnerDebugTurn) Context(ctx context.Context) context.Context {
	if d == nil {
		return ctx
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.contextLocked(ctx)
}

func (d *runnerDebugTurn) contextLocked(ctx context.Context) context.Context {
	if !d.created || d.runContext.RunID == uuid.Nil {
		return ctx
	}
	runContext := d.runContext
	return chatdebug.ContextWithRun(ctx, &runContext)
}

func (d *runnerDebugTurn) RecordOutcome(status chatdebug.Status) {
	if d == nil || debugTurnOutcomePriority(status) == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finalized {
		return
	}
	if !d.statusSet || debugTurnOutcomePriority(status) > debugTurnOutcomePriority(d.status) {
		d.status = status
		d.statusSet = true
	}
}

func (d *runnerDebugTurn) Finalize(ctx context.Context) {
	if d == nil {
		return
	}

	d.mu.Lock()
	if d.finalized {
		d.mu.Unlock()
		return
	}
	d.finalized = true
	if d.heartbeatDone != nil {
		close(d.heartbeatDone)
		d.heartbeatDone = nil
	}
	if !d.created || d.service == nil || d.runContext.RunID == uuid.Nil {
		d.mu.Unlock()
		return
	}
	service := d.service
	runContext := d.runContext
	seedSummary := d.seedSummary
	status := chatdebug.StatusInterrupted
	if d.statusSet {
		status = d.status
	}
	logger := d.logger
	d.mu.Unlock()

	if finalizeErr := service.FinalizeRun(ctx, chatdebug.FinalizeRunParams{
		RunID:       runContext.RunID,
		ChatID:      runContext.ChatID,
		Status:      status,
		SeedSummary: seedSummary,
	}); finalizeErr != nil {
		logger.Warn(ctx, "failed to finalize chat debug run",
			slog.F("chat_id", runContext.ChatID),
			slog.F("run_id", runContext.RunID),
			slog.Error(finalizeErr),
		)
	}
}

func debugTurnOutcomePriority(status chatdebug.Status) int {
	switch status {
	case chatdebug.StatusCompleted:
		return 1
	case chatdebug.StatusInterrupted:
		return 2
	case chatdebug.StatusError:
		return 3
	default:
		return 0
	}
}
