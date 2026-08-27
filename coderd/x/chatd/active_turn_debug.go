package chatd

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
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
	return d.ensureLocked(ctx, chat, debug)
}

func (d *runnerDebugTurn) ensureLocked(
	ctx context.Context,
	chat database.Chat,
	debug *generationDebug,
) context.Context {
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
	// Carry per-server MCP connect outcomes (and their dropped
	// count) stashed by RecordMCPConnectSummaries before the run
	// existed, so slow or failing servers appear in the run instead
	// of as a silent gap before the first step. Seeded keys survive
	// FinalizeRun's summary aggregation.
	for key, stashed := range d.seedSummary {
		if seedSummary == nil {
			seedSummary = make(map[string]any, len(d.seedSummary))
		}
		seedSummary[key] = stashed
	}
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

// RecordMCPConnectSummaries merges one preparation's per-server MCP
// connect outcomes into the mcp_connect summary key, creating the
// debug run if it does not exist yet. Preparation invokes it as soon
// as its MCP connect phase completes, so every attempt is recorded:
// preparations that fail after connecting, decision errors, and
// actions that never reach Ensure (local tool execution,
// requires-action, turn finishing). Creating the run here matters
// when the first preparation connects and then fails: no model or
// compaction action ever runs Ensure, and Finalize discards the
// stash of a never-created run.
func (d *runnerDebugTurn) RecordMCPConnectSummaries(
	ctx context.Context,
	chat database.Chat,
	debug *generationDebug,
	summaries []mcpclient.ConnectSummary,
) {
	if d == nil || len(summaries) == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.disabled || d.finalized {
		return
	}
	// Merge before ensuring so the outcomes ride the seed summary
	// when this call is the one that creates the run.
	d.mergeMCPConnectSummariesLocked(summaries)
	d.ensureLocked(ctx, chat, debug)
}

// maxMCPConnectSummaryEntries bounds the retained per-preparation MCP
// connect outcomes. A turn may run up to 1,200 generation steps, each
// reconnecting to every selected server, so unbounded retention could
// grow one run summary to megabytes of database and API payload. The
// newest entries win because the tail of the history is what shows a
// server that degraded mid-turn; the mcp_connect_dropped count keeps
// the truncation visible.
const maxMCPConnectSummaryEntries = 100

// mergeMCPConnectSummariesLocked appends a preparation's per-server
// MCP connect outcomes to the mcp_connect summary key. chatd
// reconnects to every configured MCP server on each generation step
// while the run is created only once, so without this merge only
// one preparation's outcomes would survive to the finalized run and
// a server that degrades mid-turn would still be reported as
// connected.
func (d *runnerDebugTurn) mergeMCPConnectSummariesLocked(summaries []mcpclient.ConnectSummary) {
	if len(summaries) == 0 {
		return
	}
	if d.seedSummary == nil {
		d.seedSummary = make(map[string]any, 1)
	}
	existing, _ := d.seedSummary["mcp_connect"].([]mcpclient.ConnectSummary)
	existing = append(existing, summaries...)
	if over := len(existing) - maxMCPConnectSummaryEntries; over > 0 {
		existing = existing[over:]
		dropped, _ := d.seedSummary["mcp_connect_dropped"].(int)
		d.seedSummary["mcp_connect_dropped"] = dropped + over
	}
	d.seedSummary["mcp_connect"] = existing
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
