package chatloop

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	defaultCompactionThresholdPercent = int32(70)
	minCompactionThresholdPercent     = int32(0)
	// CompactionDisabledThresholdPercent turns a compaction trigger off.
	CompactionDisabledThresholdPercent = int32(100)

	// compactionDebugCreateRunTimeout caps the compaction debug
	// CreateRun budget. Debug instrumentation is best-effort;
	// running without the debug row is preferable to blocking
	// compaction on a slow or locked DB.
	compactionDebugCreateRunTimeout = 5 * time.Second

	// minSummaryOutputTokens is the least room for the summary that the
	// full summary request must leave in the window. With less, the cap
	// clamp would truncate the summary, so the elided request goes first.
	minSummaryOutputTokens = int64(16_000)

	// elidedToolResultMinBytes is what an elided tool result keeps at
	// least: enough for its leading summary and trailing status.
	elidedToolResultMinBytes = 1 << 10

	// mediaTokenEstimate approximates one image or file part, about what
	// providers bill for a typical screenshot. Providers bill images by
	// dimensions, so the encoded size would overcount them by orders of
	// magnitude.
	mediaTokenEstimate = int64(1_600)

	// compactionContextTooLargeMessage tells the user how to recover a
	// conversation that is too large to summarize.
	compactionContextTooLargeMessage = "The conversation is too large for the model's context window and could not be compacted. " +
		"Edit an earlier message to remove large content, or start a new conversation. " +
		"If this keeps happening, ask an administrator to check the model's context limit."

	defaultCompactionSummaryPrompt = "You are performing a context compaction. " +
		"Summarize the conversation so a new assistant can seamlessly " +
		"continue the work in progress.\n\n" +
		"Include:\n" +
		// The constraints bullet below is deliberately verbose: offline replay
		// of production chats showed compaction summaries dropping or softening
		// user-stated constraints, and this wording measurably improved their
		// survival (see PR #27230). Reword only with re-validation.
		"- User constraints, corrections, and prohibitions: rules, " +
		"scope limits, style rules, and process corrections stated by " +
		"the user. Quote or closely paraphrase the user's wording; do " +
		"not soften, merge, or truncate them. Constraints are standing " +
		"until the user revokes them; they do not become stale when " +
		"the task moves on. When the user corrected the assistant's " +
		"behavior, record the correction itself, not only the " +
		"corrected outcome. Include only constraints the user stated " +
		"in conversation; do not place rules from system prompts, " +
		"AGENTS.md, or other configuration files in this section. " +
		"Those rules may appear elsewhere in the summary with their " +
		"true source named. When in doubt whether a rule originated " +
		"from the user, name its source or omit the attribution " +
		"rather than defaulting to user.\n" +
		"- The user's overall goal and current task\n" +
		"- Key decisions made and their rationale\n" +
		"- Concrete technical details: file paths, function names, " +
		"commands, APIs, and configurations\n" +
		"- Errors encountered and how they were resolved. Keep error " +
		"notes specific: name the file, the error, and the fix. Do not " +
		"generalize from a specific failure to a blanket tool-avoidance " +
		"rule (e.g. \"tool X is unreliable\" or \"always use Y instead " +
		"of Z\")\n" +
		"- Current state of the work: what is DONE, what is IN PROGRESS, " +
		"and what REMAINS to be done\n" +
		"- The specific action the assistant was performing or about to " +
		"perform when this summary was triggered\n\n" +
		"Be dense and factual. Every sentence should convey essential " +
		"context for continuation. Do not include pleasantries or " +
		"conversational filler. For content that can be reproduced " +
		"(repo files, command output, API responses), reference how to " +
		"obtain it (file path, command, URL) rather than inlining the " +
		"full content. Include brief inline summaries when the content " +
		"itself would exceed a few lines."
	defaultCompactionSystemSummaryPrefix = "The following is a summary of " +
		"the earlier conversation. Continue from it:"
)

// CompactionSource identifies what triggered a compaction. It is
// recorded in the persisted chat_summarized tool JSON and the
// streamed synthetic parts so clients can render manual compactions
// distinctly.
type CompactionSource string

const (
	CompactionSourceAutomatic CompactionSource = "automatic"
	CompactionSourceManual    CompactionSource = "manual"
)

type CompactionOptions struct {
	ThresholdPercent    int32
	ContextLimit        int64
	SummaryPrompt       string
	SummaryHint         string
	SystemSummaryPrefix string
	DebugSvc            *chatdebug.Service
	ChatID              uuid.UUID
	HistoryTipMessageID int64

	ResolvedProvider string
	ResolvedModel    string
	ModelConfigID    uuid.UUID
	SummaryCall      fantasy.Call
	// ToolDefinitions is copied from the parent generation request so the
	// summary call uses the exact same ordered definitions.
	ToolDefinitions []fantasy.Tool

	// SummaryWindow is the summary model's context window, or zero when
	// unknown. It bounds the summary cap and the elided request.
	SummaryWindow int64
	// EstimatedInputTokens estimates the input of the full summary
	// request: the trigger step's prompt and output, the trailing
	// messages, and the summary prompt.
	EstimatedInputTokens int64
	// ElideFirst sends the elided summary request before the full one
	// because the full one leaves too little of SummaryWindow for the
	// summary.
	ElideFirst bool

	// Logger records summary requests that fail.
	Logger slog.Logger

	// Clock and StreamSilenceTimeout guard the summary stream against a
	// provider that opens the stream but stops yielding parts. Zero values
	// fall back to a real clock and DefaultStreamSilenceTimeout.
	Clock                quartz.Clock
	StreamSilenceTimeout time.Duration

	// Force skips the threshold gate (including the threshold=100
	// disable and the zero-usage early return). Set for manual,
	// user-requested compactions.
	Force bool
	// Source labels what triggered the compaction. Defaults to
	// CompactionSourceAutomatic when empty.
	Source CompactionSource

	// ToolCallID and ToolName identify the synthetic tool call
	// used to represent compaction in the message stream.
	ToolCallID string
	ToolName   string

	// PublishMessagePart publishes streaming parts to connected
	// clients so they see "Summarizing..." / "Summarized" UI
	// transitions during compaction.
	PublishMessagePart func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
}

// CompactionResult is one generated summary. ThresholdPercent and
// TriggerContextLimit echo the requested trigger, not the normalized one, so
// chatd's summaryRecordedUnderOtherTrigger compares like with like;
// UsagePercent is still measured against the resolved limit.
type CompactionResult struct {
	SystemSummary    string
	SummaryReport    string
	Source           CompactionSource
	ThresholdPercent int32
	UsagePercent     float64
	ContextTokens    int64
	ChatContextLimit int64
	// EstimatedContextTokens covers only SystemSummary, not the full prompt.
	EstimatedContextTokens int64
	TriggerContextLimit    int64
	// Runtime is the wall-clock duration of the summarization model
	// call, the compaction step's billable runtime (see
	// PersistedStep.Runtime). Zero when the run was gated off before
	// calling the model.
	Runtime time.Duration
	// ProviderResponseID identifies the summary response. See
	// PersistedStep.ProviderResponseID.
	ProviderResponseID string
}

// CompactionToolResult is the chat_summarized tool result payload, both
// streamed and persisted.
type CompactionToolResult struct {
	Summary string           `json:"summary"`
	Source  CompactionSource `json:"source"`
	// ThresholdPercent is relative to TriggerContextLimitTokens.
	ThresholdPercent int32 `json:"threshold_percent"`
	// UsagePercent is relative to the resolved limit, which equals
	// TriggerContextLimitTokens when it is positive.
	UsagePercent  float64 `json:"usage_percent"`
	ContextTokens int64   `json:"context_tokens"`
	// ContextLimitTokens is the chat model's window.
	ContextLimitTokens        int64 `json:"context_limit_tokens"`
	EstimatedContextTokens    int64 `json:"estimated_context_tokens"`
	TriggerContextLimitTokens int64 `json:"trigger_context_limit_tokens"`
}

// GenerateCompaction generates one context summary and returns it without
// persisting. It publishes compaction progress parts when configured.
// Threshold gating (including the threshold=100 disable and the
// zero-usage early return) is skipped when opts.Force is set.
func GenerateCompaction(ctx context.Context, opts GenerateCompactionOptions) (CompactionResult, error) {
	if opts.Model == nil {
		return CompactionResult{}, xerrors.New("chat model is required")
	}
	if opts.Clock == nil {
		return CompactionResult{}, xerrors.New("clock is required")
	}
	config, ok := normalizedCompactionGenerateConfig(opts)
	if !ok {
		return CompactionResult{}, nil
	}

	contextTokens := contextTokensFromUsage(opts.StepUsage)
	if contextTokens <= 0 && !config.Force {
		return CompactionResult{}, nil
	}
	contextLimit := resolveContextLimit(
		config.ContextLimit,
		opts.ContextLimitFallback,
	)
	usagePercent, _ := shouldCompact(
		contextTokens,
		contextLimit,
		config.ThresholdPercent,
	)
	// contextTokens predates the trailing messages, which the next model
	// call also sends.
	_, compact := shouldCompact(
		contextTokens+opts.TrailingTokens,
		contextLimit,
		config.ThresholdPercent,
	)
	if !compact && !config.Force {
		return CompactionResult{}, nil
	}

	// contextTokens covers only the trigger step's prompt, so the summary
	// request's input also includes that step's output, the trailing
	// messages, and the summary prompt appended by
	// generateCompactionSummary.
	config.EstimatedInputTokens = contextTokens + opts.StepUsage.OutputTokens + opts.TrailingTokens +
		estimateTokens(len(config.SummaryPrompt)+len(config.SummaryHint))
	config.SummaryWindow = contextLimit
	if opts.SummaryContextLimit > 0 {
		config.SummaryWindow = opts.SummaryContextLimit
	}
	minRoom := minSummaryOutputTokens
	if maxOutput := config.SummaryCall.MaxOutputTokens; maxOutput != nil && *maxOutput < minRoom {
		minRoom = *maxOutput
	}
	config.ElideFirst = config.SummaryWindow > 0 && config.SummaryWindow-config.EstimatedInputTokens < minRoom

	if config.PublishMessagePart != nil && config.ToolCallID != "" {
		config.PublishMessagePart(
			codersdk.ChatMessageRoleAssistant,
			codersdk.ChatMessageToolCall(config.ToolCallID, config.ToolName, nil),
		)
	}

	summaryStart := opts.Clock.Now()
	if opts.OnModelStreamStart != nil {
		opts.OnModelStreamStart()
	}
	summary, responseID, err := generateCompactionSummary(ctx, opts.Model, opts.Messages, config)
	if err != nil {
		publishCompactionError(config, "failed to generate compaction summary")
		return CompactionResult{}, err
	}
	summaryRuntime := opts.Clock.Since(summaryStart)
	if summary == "" {
		publishCompactionError(config, "compaction produced an empty summary")
		return CompactionResult{}, xerrors.New("compaction produced an empty summary")
	}

	result := CompactionResult{
		SystemSummary: strings.TrimSpace(
			config.SystemSummaryPrefix + "\n\n" + summary,
		),
		SummaryReport:       summary,
		Source:              config.Source,
		ThresholdPercent:    opts.ThresholdPercent,
		UsagePercent:        usagePercent,
		ContextTokens:       contextTokens,
		ChatContextLimit:    opts.ChatContextLimit,
		TriggerContextLimit: opts.ContextLimit,
		Runtime:             summaryRuntime,
		ProviderResponseID:  responseID,
	}
	result.EstimatedContextTokens = estimateTokens(len(result.SystemSummary))
	if config.PublishMessagePart != nil && config.ToolCallID != "" {
		resultJSON, _ := json.Marshal(CompactionToolResult{
			Summary:                   summary,
			Source:                    result.Source,
			ThresholdPercent:          result.ThresholdPercent,
			UsagePercent:              usagePercent,
			ContextTokens:             contextTokens,
			ContextLimitTokens:        result.ChatContextLimit,
			EstimatedContextTokens:    result.EstimatedContextTokens,
			TriggerContextLimitTokens: result.TriggerContextLimit,
		})
		config.PublishMessagePart(
			codersdk.ChatMessageRoleTool,
			codersdk.ChatMessageToolResult(config.ToolCallID, config.ToolName, resultJSON, false, false),
		)
	}
	return result, nil
}

func normalizedCompactionGenerateConfig(opts GenerateCompactionOptions) (CompactionOptions, bool) {
	config := CompactionOptions{
		ThresholdPercent:     opts.ThresholdPercent,
		ContextLimit:         opts.ContextLimit,
		SummaryPrompt:        opts.SummaryPrompt,
		SummaryHint:          opts.SummaryHint,
		SystemSummaryPrefix:  opts.SystemSummaryPrefix,
		DebugSvc:             opts.DebugSvc,
		ChatID:               opts.ChatID,
		HistoryTipMessageID:  opts.HistoryTipMessageID,
		ResolvedProvider:     opts.ResolvedProvider,
		ResolvedModel:        opts.ResolvedModel,
		ModelConfigID:        opts.ModelConfigID,
		SummaryCall:          opts.SummaryCall,
		ToolDefinitions:      opts.ToolDefinitions,
		Force:                opts.Force,
		Source:               opts.Source,
		ToolCallID:           opts.ToolCallID,
		ToolName:             opts.ToolName,
		PublishMessagePart:   opts.PublishMessagePart,
		Clock:                opts.Clock,
		StreamSilenceTimeout: opts.StreamSilenceTimeout,
		Logger:               opts.Logger,
	}
	if strings.TrimSpace(config.SummaryPrompt) == "" {
		config.SummaryPrompt = defaultCompactionSummaryPrompt
	}
	if strings.TrimSpace(config.SystemSummaryPrefix) == "" {
		config.SystemSummaryPrefix = defaultCompactionSystemSummaryPrefix
	}
	if config.Source == "" {
		config.Source = CompactionSourceAutomatic
	}
	if config.ThresholdPercent < minCompactionThresholdPercent ||
		config.ThresholdPercent > CompactionDisabledThresholdPercent {
		config.ThresholdPercent = defaultCompactionThresholdPercent
	}
	// threshold=100 disables automatic compaction; a forced run
	// still proceeds because the user asked explicitly.
	if config.ThresholdPercent == CompactionDisabledThresholdPercent && !config.Force {
		return CompactionOptions{}, false
	}
	return config, true
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

// EstimateTrailingTokens estimates the prompt tokens of the messages after
// the last assistant message, typically the results of its tool calls. They
// are input to the next model call but absent from the usage reported for
// the step that produced that message.
func EstimateTrailingTokens(messages []fantasy.Message) int64 {
	start := len(messages)
	for start > 0 && messages[start-1].Role != fantasy.MessageRoleAssistant {
		start--
	}
	return estimateMessagesTokens(messages[start:])
}

func estimateTokens(bytes int) int64 {
	return int64((bytes + bytesPerTokenEstimate - 1) / bytesPerTokenEstimate)
}

// estimateMessagesTokens approximates the prompt tokens of messages from
// their content size.
func estimateMessagesTokens(messages []fantasy.Message) int64 {
	textBytes := 0
	mediaParts := int64(0)
	for _, msg := range messages {
		for _, part := range msg.Content {
			partBytes, partMedia := messagePartSize(part)
			textBytes += partBytes
			mediaParts += partMedia
		}
	}
	return estimateTokens(textBytes) + mediaParts*mediaTokenEstimate
}

// messagePartSize returns the text bytes and the number of media
// attachments that part adds to a prompt.
func messagePartSize(part fantasy.MessagePart) (textBytes int, mediaParts int64) {
	if text, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok {
		return len(text.Text), 0
	}
	if reasoning, ok := fantasy.AsMessagePart[fantasy.ReasoningPart](part); ok {
		return len(reasoning.Text), 0
	}
	if _, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
		return 0, 1
	}
	if call, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok {
		return len(call.ToolName) + len(call.Input), 0
	}
	result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
	if !ok {
		return 0, 0
	}
	if output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Output); ok {
		return len(output.Text), 0
	}
	if output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](result.Output); ok && output.Error != nil {
		return len(output.Error.Error()), 0
	}
	if output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](result.Output); ok {
		return len(output.Text), 1
	}
	return 0, 0
}

// elideToolResults returns a copy of messages in which the largest tool
// result texts are cut to one common size, keeping their head and tail, so
// the copy's estimate fits budgetTokens. Cut results keep at least
// elidedToolResultMinBytes, so the copy can still exceed the budget. It
// reports the bytes removed; zero means nothing was cut and messages is
// returned unchanged.
func elideToolResults(messages []fantasy.Message, budgetTokens int64) ([]fantasy.Message, int) {
	excessBytes := (estimateMessagesTokens(messages) - budgetTokens) * bytesPerTokenEstimate
	if excessBytes <= 0 {
		return messages, 0
	}
	var sizes []int
	for _, msg := range messages {
		for _, part := range msg.Content {
			if _, text, ok := elidableToolResult(part); ok {
				sizes = append(sizes, len(text))
			}
		}
	}
	if len(sizes) == 0 {
		return messages, 0
	}
	keepBytes := toolResultElisionSize(sizes, excessBytes)

	elided := slices.Clone(messages)
	removed := 0
	for i, msg := range elided {
		var content []fantasy.MessagePart
		for j, part := range msg.Content {
			result, text, ok := elidableToolResult(part)
			if !ok || len(text) <= keepBytes {
				continue
			}
			truncated, _ := TruncateToolResultText(text, keepBytes)
			result.Output = fantasy.ToolResultOutputContentText{Text: truncated}
			if content == nil {
				// The content slices are shared with the caller's messages.
				content = slices.Clone(msg.Content)
			}
			content[j] = result
			removed += len(text) - len(truncated)
		}
		if content != nil {
			elided[i].Content = content
		}
	}
	return elided, removed
}

// elidableToolResult returns a locally executed tool result and its text
// when the text is longer than elidedToolResultMinBytes.
func elidableToolResult(part fantasy.MessagePart) (fantasy.ToolResultPart, string, bool) {
	result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
	if !ok || result.ProviderExecuted {
		return fantasy.ToolResultPart{}, "", false
	}
	output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Output)
	if !ok || len(output.Text) <= elidedToolResultMinBytes {
		return fantasy.ToolResultPart{}, "", false
	}
	return result, output.Text, true
}

// toolResultElisionSize returns the largest size such that cutting every
// result above it to that size removes at least excessBytes, but never less
// than elidedToolResultMinBytes. Every size must exceed that minimum.
func toolResultElisionSize(sizes []int, excessBytes int64) int {
	slices.SortFunc(sizes, func(a, b int) int { return cmp.Compare(b, a) })
	var cutBytes int64
	for i, size := range sizes {
		cutBytes += int64(size)
		// Cutting the i+1 largest results to keepBytes removes at least
		// excessBytes. The size holds when no smaller result needs a cut.
		keepBytes := (cutBytes - excessBytes) / int64(i+1)
		nextBytes := int64(elidedToolResultMinBytes)
		if i+1 < len(sizes) {
			nextBytes = max(nextBytes, int64(sizes[i+1]))
		}
		if keepBytes >= nextBytes {
			return int(keepBytes)
		}
	}
	return elidedToolResultMinBytes
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

// resolveContextLimit returns the configured limit when positive, then
// the fallback, or zero when neither is positive.
func resolveContextLimit(configLimit, fallback int64) int64 {
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

	// Prefer the caller-supplied summary model identity; it can differ
	// from the parent run's chat model under a compaction override.
	provider := parentRun.Provider
	if options.ResolvedProvider != "" {
		provider = options.ResolvedProvider
	}
	model := parentRun.Model
	if options.ResolvedModel != "" {
		model = options.ResolvedModel
	}
	modelConfigID := parentRun.ModelConfigID
	if options.ModelConfigID != uuid.Nil {
		modelConfigID = options.ModelConfigID
	}

	// Use a separate short-lived context for the debug insert so a
	// slow or locked DB cannot block the model call. Detached from
	// the parent so cancellation of the compaction run still lets
	// the insert reach a terminal state, matching the best-effort
	// contract of debug instrumentation.
	createRunCtx, createRunCancel := context.WithTimeout(
		context.WithoutCancel(ctx), compactionDebugCreateRunTimeout,
	)
	run, err := options.DebugSvc.CreateRun(createRunCtx, chatdebug.CreateRunParams{
		ChatID:              options.ChatID,
		RootChatID:          parentRun.RootChatID,
		ParentChatID:        parentRun.ParentChatID,
		ModelConfigID:       modelConfigID,
		TriggerMessageID:    parentRun.TriggerMessageID,
		HistoryTipMessageID: historyTipMessageID,
		Kind:                chatdebug.KindCompaction,
		Status:              chatdebug.StatusInProgress,
		Provider:            provider,
		Model:               model,
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
		ModelConfigID:       modelConfigID,
		TriggerMessageID:    parentRun.TriggerMessageID,
		HistoryTipMessageID: historyTipMessageID,
		Provider:            provider,
		Model:               model,
	})

	return compactionCtx, func(runErr error) {
		status := chatdebug.ClassifyError(runErr)
		// Debug instrumentation must not surface as a compaction failure.
		_ = options.DebugSvc.FinalizeRun(compactionCtx, chatdebug.FinalizeRunParams{
			RunID:  run.ID,
			ChatID: options.ChatID,
			Status: status,
		})
	}
}

// Summary request kinds, from the largest request to the smallest.
const (
	summaryRequestFull         = "full"
	summaryRequestWithoutTools = "without_tools"
	summaryRequestElided       = "elided"
)

// summaryRequest is the shape of one summary request.
type summaryRequest struct {
	kind            string
	history         []fantasy.Message
	tools           []fantasy.Tool
	maxOutputTokens *int64
	inputTokens     int64
	elidedBytes     int
}

// newSummaryRequest builds the summary request of kind. It reports false
// when kind cannot shrink the request: no tools to drop or no tool results
// to elide.
func newSummaryRequest(kind string, messages []fantasy.Message, options CompactionOptions) (summaryRequest, bool) {
	request := summaryRequest{kind: kind, history: messages, inputTokens: options.EstimatedInputTokens}
	switch kind {
	case summaryRequestFull:
		request.tools = options.ToolDefinitions
	case summaryRequestWithoutTools:
		if len(options.ToolDefinitions) == 0 {
			return summaryRequest{}, false
		}
	case summaryRequestElided:
		// Halving leaves room for the summary and absorbs both the
		// estimate's error and a configured window larger than the
		// provider's real one.
		budget := estimateMessagesTokens(messages)
		if options.SummaryWindow > 0 {
			budget = min(budget, options.SummaryWindow)
		}
		request.history, request.elidedBytes = elideToolResults(messages, budget/2)
		if request.elidedBytes == 0 {
			return summaryRequest{}, false
		}
		request.inputTokens = estimateMessagesTokens(request.history) +
			estimateTokens(len(options.SummaryPrompt)+len(options.SummaryHint))
	default:
		return summaryRequest{}, false
	}
	request.maxOutputTokens = summaryOutputCap(options.SummaryCall.MaxOutputTokens, options.SummaryWindow, request.inputTokens)
	return request, true
}

// summaryOutputCap bounds maxOutputTokens by the room inputTokens leaves in
// window, since sum-enforcing providers reject requests whose input plus
// max_tokens exceeds the context window. An unknown window or no room left
// leaves the cap unchanged.
func summaryOutputCap(maxOutputTokens *int64, window, inputTokens int64) *int64 {
	if maxOutputTokens == nil || window <= 0 {
		return maxOutputTokens
	}
	remaining := window - inputTokens
	if remaining > 0 && remaining < *maxOutputTokens {
		return &remaining
	}
	return maxOutputTokens
}

func (r summaryRequest) attributes(sent int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int(AttrSummaryRequests, sent),
		attribute.String(AttrSummaryRequest, r.kind),
		attribute.Int64(AttrSummaryEstimatedInputTokens, r.inputTokens),
		attribute.Int64(AttrSummaryMaxOutputTokens, ptr.NilToEmpty(r.maxOutputTokens)),
		attribute.Int(AttrSummaryToolCount, len(r.tools)),
		attribute.Int(AttrSummaryElidedBytes, r.elidedBytes),
	}
}

func (r summaryRequest) logFields(err error) []slog.Field {
	return []slog.Field{
		slog.F("summary_request", r.kind),
		slog.F("estimated_input_tokens", r.inputTokens),
		slog.F("max_output_tokens", ptr.NilToEmpty(r.maxOutputTokens)),
		slog.F("tool_count", len(r.tools)),
		slog.F("elided_bytes", r.elidedBytes),
		slog.Error(err),
	}
}

// generateCompactionSummary asks the model to summarize the
// conversation so far. The provided messages should contain the
// complete history (system prompt, user/assistant turns, tool
// results). A final user message with the summary prompt is appended
// before calling the model.
//
// Compaction is the recovery path for an over-limit conversation, so a
// request the model rejects for its size falls back to smaller ones:
// first without tool definitions, then with the largest tool results
// elided. options.ElideFirst starts with the elided request.
func generateCompactionSummary(
	ctx context.Context,
	model fantasy.LanguageModel,
	messages []fantasy.Message,
	options CompactionOptions,
) (summary string, responseID string, err error) {
	summaryParts := []fantasy.MessagePart{fantasy.TextPart{Text: options.SummaryPrompt}}
	if strings.TrimSpace(options.SummaryHint) != "" {
		summaryParts = append(summaryParts, fantasy.TextPart{Text: options.SummaryHint})
	}
	applyPromptCaching := promptCachingStrategyFor(model, model.Model())
	summaryPromptFor := func(history []fantasy.Message) []fantasy.Message {
		summaryPrompt := make([]fantasy.Message, 0, len(history)+1)
		summaryPrompt = append(summaryPrompt, history...)
		summaryPrompt = append(summaryPrompt, fantasy.Message{
			Role:    fantasy.MessageRoleUser,
			Content: summaryParts,
		})
		// Claude only reads the cache at explicit breakpoints, so without
		// these the shared tool and history prefix is never a cache hit.
		if applyPromptCaching != nil {
			applyPromptCaching(summaryPrompt)
		}
		return summaryPrompt
	}

	summaryCtx, finishDebugRun := startCompactionDebugRun(ctx, options)
	defer func() {
		// If model.Stream (or anything else below) panics, the
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

	clock := options.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	timeout := options.StreamSilenceTimeout
	if timeout == 0 {
		timeout = DefaultStreamSilenceTimeout
	}
	// TTFT is an assistant-generation measurement, so the summary stream
	// records neither its histogram nor its stage.
	streamSummaryText := func(call fantasy.Call) (string, error) {
		attempt, err := guardedStream(
			summaryCtx,
			options.ResolvedProvider,
			options.ResolvedModel,
			clock,
			timeout,
			func(attemptCtx context.Context) (fantasy.StreamResponse, error) {
				return model.Stream(attemptCtx, call)
			},
			NopMetrics(),
			(*StageTracer)(nil),
			StageModel{},
		)
		if err != nil {
			return "", xerrors.Errorf("stream summary text: %w", wrapProviderStreamError(options.ResolvedProvider, err))
		}
		defer attempt.release()

		textPartIndexes := make(map[string]int)
		textParts := make([]string, 0, 1)
		var (
			reasoningSeen bool
			finishSeen    bool
			finishReason  fantasy.FinishReason
			streamErr     error
		)
		for part := range attempt.stream {
			switch part.Type {
			case fantasy.StreamPartTypeTextStart:
				if _, ok := textPartIndexes[part.ID]; ok {
					continue
				}
				textPartIndexes[part.ID] = len(textParts)
				textParts = append(textParts, part.Delta)
			case fantasy.StreamPartTypeTextDelta:
				index, ok := textPartIndexes[part.ID]
				if !ok {
					index = len(textParts)
					textPartIndexes[part.ID] = index
					textParts = append(textParts, "")
				}
				textParts[index] += part.Delta
			case fantasy.StreamPartTypeReasoningStart,
				fantasy.StreamPartTypeReasoningDelta,
				fantasy.StreamPartTypeReasoningEnd:
				reasoningSeen = true
			case fantasy.StreamPartTypeFinish:
				finishSeen = true
				finishReason = part.FinishReason
				responseID = providerResponseID(part)
			case fantasy.StreamPartTypeError:
				streamErr = part.Error
				if streamErr == nil {
					streamErr = xerrors.New("model returned an error part")
				}
			}
			if streamErr != nil {
				break
			}
		}
		if err := attempt.finish(streamErr); err != nil {
			// Providers can surface remote stream resets as bare
			// context.Canceled; wrap them like GenerateAssistant so the
			// generation loop retries instead of terminally erroring.
			return "", xerrors.Errorf("stream summary text: %w", wrapProviderStreamError(options.ResolvedProvider, err))
		}
		if !finishSeen {
			// A stream that ends without a finish part was interrupted.
			// Committing its partial text would compact history against an
			// incomplete summary.
			if ctxErr := summaryCtx.Err(); ctxErr != nil {
				return "", xerrors.Errorf("stream summary text: %w", ctxErr)
			}
			return "", xerrors.New("compaction summary stream ended without a finish part")
		}

		parts := make([]string, 0, len(textParts))
		for _, text := range textParts {
			text = strings.TrimSpace(text)
			if text != "" {
				parts = append(parts, text)
			}
		}
		joined := strings.TrimSpace(strings.Join(parts, " "))
		if joined == "" && (finishReason == fantasy.FinishReasonLength || reasoningSeen) {
			return "", xerrors.New("compaction summary was truncated at the output token cap")
		}
		return joined, nil
	}
	kinds := []string{summaryRequestFull, summaryRequestWithoutTools, summaryRequestElided}
	if options.ElideFirst {
		// The full requests are larger, so they run only when there is
		// nothing to elide.
		kinds = []string{summaryRequestElided, summaryRequestFull, summaryRequestWithoutTools}
	}
	span := trace.SpanFromContext(ctx)
	sent := 0
	for _, kind := range kinds {
		request, ok := newSummaryRequest(kind, messages, options)
		if !ok {
			continue
		}
		sent++
		span.SetAttributes(request.attributes(sent)...)
		call := options.SummaryCall
		call.Prompt = summaryPromptFor(request.history)
		call.Tools = request.tools
		call.MaxOutputTokens = request.maxOutputTokens
		summary, err = streamSummaryText(call)
		if err == nil {
			return summary, responseID, nil
		}
		options.Logger.Warn(ctx, "compaction summary request failed", request.logFields(err)...)
		// Tool definitions keep the summary request on the parent turn's
		// cacheable prefix, but the tools and the full history both make
		// it larger than the turn that overflowed. The rejection can
		// arrive from the stream open or as an in-stream error part, so
		// the next request repeats the whole attempt.
		if kind == summaryRequestElided || !isContextTooLargeError(err) {
			break
		}
	}
	if isContextTooLargeError(err) {
		classified := chaterror.Classify(err)
		classified.Kind = codersdk.ChatErrorKindConfig
		classified.Message = compactionContextTooLargeMessage
		classified.Retryable = false
		classified.RetryAfter = 0
		err = chaterror.WithClassification(err, classified)
	}
	return "", responseID, err
}

// contextTooLargePhrases are context-window rejections fantasy does not
// parse, such as the OpenAI Responses API "Your input exceeds the context
// window of this model.", Anthropic-family "too long" rejections without
// token counts, and the Anthropic "input length and `max_tokens` exceed
// context limit" rejection. The response body is included so the OpenAI
// error code "context_length_exceeded" matches regardless of wording.
var contextTooLargePhrases = []string{
	"context window",
	"context length",
	"context_length",
	"context limit",
	"too long",
}

// isContextTooLargeError reports whether a provider rejected a request for
// its size: the prompt exceeded the model's context window, or the body
// exceeded a request size limit on the provider or a gateway in front of it.
// The rejection can arrive as an HTTP 400 or inside an HTTP 200 stream,
// where fantasy reports it with no status code or, for spec-shaped OpenAI
// Responses error events, as a plain *fantasy.Error.
func isContextTooLargeError(err error) bool {
	if providerErr, ok := errors.AsType[*fantasy.ProviderError](err); ok {
		if providerErr.IsContextTooLarge() || providerErr.StatusCode == http.StatusRequestEntityTooLarge {
			return true
		}
		if providerErr.IsRetryable() ||
			(providerErr.StatusCode != http.StatusBadRequest && providerErr.StatusCode != 0) {
			return false
		}
		return hasContextTooLargePhrase(providerErr.Error() + " " + string(providerErr.ResponseBody))
	}
	if fantasyErr, ok := errors.AsType[*fantasy.Error](err); ok {
		return hasContextTooLargePhrase(fantasyErr.Error())
	}
	return false
}

func hasContextTooLargePhrase(text string) bool {
	text = strings.ToLower(text)
	for _, phrase := range contextTooLargePhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
