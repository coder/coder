package chatloop

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultCompactionThresholdPercent = int32(70)
	minCompactionThresholdPercent     = int32(0)
	maxCompactionThresholdPercent     = int32(100)

	// compactionDebugCreateRunTimeout caps the compaction debug
	// CreateRun budget so a slow or locked DB cannot consume the
	// compaction's configured Timeout and cause model.Generate to
	// fail with deadline exceeded. Debug instrumentation is
	// best-effort; running without the debug row is preferable to
	// failing the compaction.
	compactionDebugCreateRunTimeout = 5 * time.Second

	// defaultCompactionSummaryPrompt instructs the summarizing model to
	// produce a structured document with typed sections. The schema
	// separates user-stated decisions (which must include a quote) from
	// model-inferred notes (which expire after N compaction cycles).
	// This prevents model workarounds from being promoted to standing
	// policy across compactions and model swaps.
	defaultCompactionSummaryPrompt = "You are performing a context compaction. " +
		"Summarize the conversation so a different model instance can seamlessly " +
		"continue the work in progress.\n\n" +
		"Use EXACTLY these sections in this order:\n\n" +
		"## INVARIANTS\n" +
		"Verifiable, stable facts: file paths, package manager, framework, " +
		"repo location, validation command, git author. " +
		"Do NOT include tool-usage prescriptions here.\n\n" +
		"## USER_DECISIONS\n" +
		"ONLY things the user explicitly stated. " +
		"Each entry MUST include a verbatim or close-paraphrase quote and a timestamp. " +
		"If you cannot find a user quote for something, it does not belong here.\n\n" +
		"## COMPLETED_TASKS\n" +
		"What is provably done; include how it was verified " +
		"(e.g., tests passed, grep confirmed).\n\n" +
		"## PENDING_TASKS\n" +
		"What remains. No line numbers unless from a command run in this session.\n\n" +
		"## STATE_SNAPSHOT\n" +
		"One-shot facts at compaction time (file sizes, build status, last commit). " +
		"These will drift. Prefix each with \"at compaction time:\".\n\n" +
		"## MODEL_NOTES\n" +
		"Provisional observations from this session. Tag each (EXPIRES: N) where N " +
		"is the number of compaction cycles it should survive without re-validation. " +
		"Use N=3 for tool-failure observations, N=1 for highly session-specific details. " +
		"If the previous summary had MODEL_NOTES, carry each forward only if you saw " +
		"direct evidence of it in recent messages; otherwise decrement N by 1 and drop " +
		"at 0. NEVER escalate a model_note to USER_DECISIONS or INVARIANTS.\n\n" +
		"CRITICAL RULES:\n" +
		"- Do NOT write \"All X via Y\" or \"Always use Y for X\" in any section unless " +
		"the user explicitly said those words. Use MODEL_NOTES with N=1 instead.\n" +
		"- Do NOT write in first person. Write \"the previous model\" not \"I\".\n" +
		"- Do NOT broaden the scope of a model_note from the previous summary. " +
		"If it said \"failed for path X\", do not write \"always fails for this task\".\n" +
		"- Errors the previous model encountered are HISTORY, not POLICY."
	defaultCompactionSystemSummaryPrefix = "The following is a summary of " +
		"the earlier conversation. The assistant was actively working when " +
		"the context was compacted. Continue the work described below:"
	defaultCompactionTimeout = 90 * time.Second
)

type CompactionOptions struct {
	ThresholdPercent    int32
	ContextLimit        int64
	SummaryPrompt       string
	SystemSummaryPrefix string
	Timeout             time.Duration
	Persist             func(context.Context, CompactionResult) error
	DebugSvc            *chatdebug.Service
	ChatID              uuid.UUID
	HistoryTipMessageID int64

	// ToolCallID and ToolName identify the synthetic tool call
	// used to represent compaction in the message stream.
	ToolCallID string
	ToolName   string

	// PublishMessagePart publishes streaming parts to connected
	// clients so they see "Summarizing..." / "Summarized" UI
	// transitions during compaction.
	PublishMessagePart func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)

	OnError func(error)
}

type CompactionResult struct {
	SystemSummary    string
	SummaryReport    string
	ThresholdPercent int32
	UsagePercent     float64
	ContextTokens    int64
	ContextLimit     int64
}

// tryCompact checks whether context usage exceeds the compaction
// threshold and, if so, generates and persists a summary. Returns
// (true, nil) when compaction was performed, (false, nil) when not
// needed, and (false, err) on failure.
func tryCompact(
	ctx context.Context,
	model fantasy.LanguageModel,
	compaction *CompactionOptions,
	contextLimitFallback int64,
	stepUsage fantasy.Usage,
	stepMetadata fantasy.ProviderMetadata,
	allMessages []fantasy.Message,
) (bool, error) {
	config, ok := normalizedCompactionConfig(compaction)
	if !ok {
		return false, nil
	}

	contextTokens := contextTokensFromUsage(stepUsage)
	if contextTokens <= 0 {
		return false, nil
	}

	metadataLimit := extractContextLimit(stepMetadata)
	contextLimit := resolveContextLimit(
		metadataLimit.Int64,
		config.ContextLimit,
		contextLimitFallback,
	)

	usagePercent, compact := shouldCompact(
		contextTokens, contextLimit, config.ThresholdPercent,
	)
	if !compact {
		return false, nil
	}

	// Publish the "Summarizing..." tool-call indicator so
	// connected clients see activity during summary generation.
	if config.PublishMessagePart != nil && config.ToolCallID != "" {
		config.PublishMessagePart(
			codersdk.ChatMessageRoleAssistant,
			codersdk.ChatMessageToolCall(config.ToolCallID, config.ToolName, nil),
		)
	}

	summary, err := generateCompactionSummary(
		ctx, model, allMessages, config,
	)
	if err != nil {
		return false, err
	}
	if summary == "" {
		// Publish a tool-result error so connected clients
		// see the compaction failure.
		publishCompactionError(config, "compaction produced an empty summary")
		return false, xerrors.New("compaction produced an empty summary")
	}

	systemSummary := strings.TrimSpace(
		config.SystemSummaryPrefix + "\n\n" + summary,
	)

	persistCtx := context.WithoutCancel(ctx)
	err = config.Persist(persistCtx, CompactionResult{
		SystemSummary:    systemSummary,
		SummaryReport:    summary,
		ThresholdPercent: config.ThresholdPercent,
		UsagePercent:     usagePercent,
		ContextTokens:    contextTokens,
		ContextLimit:     contextLimit,
	})
	if err != nil {
		publishCompactionError(config, "failed to persist compaction result")
		return false, xerrors.Errorf("persist compaction: %w", err)
	}

	// Publish the "Summarized" tool-result part so the client
	// transitions from the in-progress indicator to the final
	// state.
	if config.PublishMessagePart != nil && config.ToolCallID != "" {
		resultJSON, _ := json.Marshal(map[string]any{
			"summary":              summary,
			"source":               "automatic",
			"threshold_percent":    config.ThresholdPercent,
			"usage_percent":        usagePercent,
			"context_tokens":       contextTokens,
			"context_limit_tokens": contextLimit,
		})
		config.PublishMessagePart(
			codersdk.ChatMessageRoleTool,
			codersdk.ChatMessageToolResult(config.ToolCallID, config.ToolName, resultJSON, false, false),
		)
	}

	return true, nil
}

// publishCompactionError sends a tool-result error part so
// connected clients see that compaction failed.
func publishCompactionError(config CompactionOptions, msg string) {
	if config.PublishMessagePart == nil || config.ToolCallID == "" {
		return
	}
	errJSON, _ := json.Marshal(map[string]any{
		"error": msg,
	})
	config.PublishMessagePart(
		codersdk.ChatMessageRoleTool,
		codersdk.ChatMessageToolResult(config.ToolCallID, config.ToolName, errJSON, true, false),
	)
}

// normalizedCompactionConfig returns a copy of the compaction options
// with defaults applied. The bool is false when compaction is
// disabled (nil options, missing Persist callback, or threshold at
// 100%).
func normalizedCompactionConfig(opts *CompactionOptions) (CompactionOptions, bool) {
	if opts == nil {
		return CompactionOptions{}, false
	}

	config := *opts
	if config.Persist == nil {
		return CompactionOptions{}, false
	}
	if strings.TrimSpace(config.SummaryPrompt) == "" {
		config.SummaryPrompt = defaultCompactionSummaryPrompt
	}
	if strings.TrimSpace(config.SystemSummaryPrefix) == "" {
		config.SystemSummaryPrefix = defaultCompactionSystemSummaryPrefix
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultCompactionTimeout
	}
	if config.ThresholdPercent < minCompactionThresholdPercent ||
		config.ThresholdPercent > maxCompactionThresholdPercent {
		config.ThresholdPercent = defaultCompactionThresholdPercent
	}
	if config.ThresholdPercent == maxCompactionThresholdPercent {
		return CompactionOptions{}, false
	}

	return config, true
}

// contextTokensFromUsage returns the total context token count from
// a step's usage report. It sums input, cache-read, and
// cache-creation tokens when available, falling back to TotalTokens
// if none of the granular fields are set.
func contextTokensFromUsage(usage fantasy.Usage) int64 {
	total := int64(0)
	hasContextTokens := false

	if usage.InputTokens > 0 {
		total += usage.InputTokens
		hasContextTokens = true
	}
	if usage.CacheReadTokens > 0 {
		total += usage.CacheReadTokens
		hasContextTokens = true
	}
	if usage.CacheCreationTokens > 0 {
		total += usage.CacheCreationTokens
		hasContextTokens = true
	}
	if !hasContextTokens && usage.TotalTokens > 0 {
		total = usage.TotalTokens
	}

	return total
}

// resolveContextLimit picks the first positive value from metadata,
// configured limit, and fallback — in that priority order. Returns
// 0 when none are positive.
func resolveContextLimit(metadataLimit, configLimit, fallback int64) int64 {
	if metadataLimit > 0 {
		return metadataLimit
	}
	if configLimit > 0 {
		return configLimit
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}

// shouldCompact returns the usage percentage and whether it exceeds
// the threshold. Returns (0, false) when contextLimit is
// non-positive.
func shouldCompact(contextTokens, contextLimit int64, thresholdPercent int32) (float64, bool) {
	if contextLimit <= 0 {
		return 0, false
	}
	usagePercent := (float64(contextTokens) / float64(contextLimit)) * 100
	return usagePercent, usagePercent >= float64(thresholdPercent)
}

func startCompactionDebugRun(
	ctx context.Context,
	options CompactionOptions,
) (context.Context, func(error)) {
	if options.DebugSvc == nil || options.ChatID == uuid.Nil {
		return ctx, func(error) {}
	}

	parentRun, ok := chatdebug.RunFromContext(ctx)
	if !ok {
		return ctx, func(error) {}
	}

	historyTipMessageID := options.HistoryTipMessageID
	if historyTipMessageID == 0 {
		historyTipMessageID = parentRun.HistoryTipMessageID
	}

	// Use a separate short-lived context for the debug insert so a
	// slow or locked DB cannot consume the compaction timeout budget
	// and turn debug slowness into a compaction failure via
	// model.Generate hitting a deadline exceeded. Detached from the
	// parent so cancellation of the compaction run still lets the
	// insert reach a terminal state, matching the best-effort
	// contract of debug instrumentation.
	createRunCtx, createRunCancel := context.WithTimeout(
		context.WithoutCancel(ctx), compactionDebugCreateRunTimeout,
	)
	run, err := options.DebugSvc.CreateRun(createRunCtx, chatdebug.CreateRunParams{
		ChatID:              options.ChatID,
		RootChatID:          parentRun.RootChatID,
		ParentChatID:        parentRun.ParentChatID,
		ModelConfigID:       parentRun.ModelConfigID,
		TriggerMessageID:    parentRun.TriggerMessageID,
		HistoryTipMessageID: historyTipMessageID,
		Kind:                chatdebug.KindCompaction,
		Status:              chatdebug.StatusInProgress,
		Provider:            parentRun.Provider,
		Model:               parentRun.Model,
	})
	createRunCancel()
	if err != nil {
		// Debug instrumentation must not surface as a compaction failure.
		return ctx, func(error) {}
	}

	compactionCtx := chatdebug.ContextWithRun(ctx, &chatdebug.RunContext{
		RunID:               run.ID,
		ChatID:              options.ChatID,
		RootChatID:          parentRun.RootChatID,
		ParentChatID:        parentRun.ParentChatID,
		ModelConfigID:       parentRun.ModelConfigID,
		TriggerMessageID:    parentRun.TriggerMessageID,
		HistoryTipMessageID: historyTipMessageID,
		Kind:                chatdebug.KindCompaction,
		Provider:            parentRun.Provider,
		Model:               parentRun.Model,
	})

	return compactionCtx, func(runErr error) {
		status := chatdebug.ClassifyError(runErr)
		if runErr != nil && xerrors.Is(runErr, ErrInterrupted) {
			status = chatdebug.StatusInterrupted
		}
		// Debug instrumentation must not surface as a compaction failure.
		_ = options.DebugSvc.FinalizeRun(compactionCtx, chatdebug.FinalizeRunParams{
			RunID:  run.ID,
			ChatID: options.ChatID,
			Status: status,
		})
	}
}

// generateCompactionSummary asks the model to summarize the
// conversation so far. The provided messages should contain the
// complete history (system prompt, user/assistant turns, tool
// results). A final user message with the summary prompt is appended
// before calling the model.
func generateCompactionSummary(
	ctx context.Context,
	model fantasy.LanguageModel,
	messages []fantasy.Message,
	options CompactionOptions,
) (summary string, err error) {
	summaryPrompt := make([]fantasy.Message, 0, len(messages)+1)
	summaryPrompt = append(summaryPrompt, messages...)
	summaryPrompt = append(summaryPrompt, fantasy.Message{
		Role: fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: options.SummaryPrompt},
		},
	})
	toolChoice := fantasy.ToolChoiceNone

	summaryCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	summaryCtx, finishDebugRun := startCompactionDebugRun(summaryCtx, options)
	defer func() {
		// If model.Generate (or anything else below) panics, the
		// named err return is still nil at this point. Without the
		// recover hook we would finalize the debug run as Completed
		// in the exact crash path operators rely on to diagnose
		// failures. Finalize with the panic as an error status and
		// re-panic so the caller's recovery still observes the
		// original panic value.
		if r := recover(); r != nil {
			finishDebugRun(xerrors.Errorf("panic during compaction summary: %v", r))
			panic(r)
		}
		finishDebugRun(err)
	}()

	response, err := model.Generate(summaryCtx, fantasy.Call{
		Prompt:     summaryPrompt,
		ToolChoice: &toolChoice,
	})
	if err != nil {
		return "", xerrors.Errorf("generate summary text: %w", err)
	}

	parts := make([]string, 0, len(response.Content))
	for _, block := range response.Content {
		textBlock, ok := fantasy.AsContentType[fantasy.TextContent](block)
		if !ok {
			continue
		}
		text := strings.TrimSpace(textBlock.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, " ")), nil
}
