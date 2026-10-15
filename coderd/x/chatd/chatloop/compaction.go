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
	"github.com/coder/coder/v2/coderd/x/chatfiles"
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

	// elidedToolResultMinBytes is what an elided tool result or text
	// attachment keeps at least: enough for its leading summary and
	// trailing status.
	elidedToolResultMinBytes = 1 << 10

	// mediaTokenEstimate approximates one image or PDF part, about what
	// providers bill for a typical screenshot. Providers bill images by
	// dimensions and PDFs by pages, which their size overstates by an order
	// of magnitude or more, so a size estimate would compact a chat for
	// every large attachment. A long PDF counts low until the next step
	// measures it.
	mediaTokenEstimate = int64(1_600)

	// compactionContextTooLargeMessage tells the user how to recover a
	// conversation that is too large to summarize.
	compactionContextTooLargeMessage = "This conversation is too large for the model to compact. " +
		"Edit an earlier message, which deletes every message after it, or start a new conversation. " +
		"If the conversation is short, ask an administrator to check the model's context limit."

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

	// GenerateCompaction sets the next three fields.
	//
	// summaryWindow is the summary model's context window, or zero when
	// unknown. It bounds the summary cap and the elided request.
	summaryWindow int64
	// estimatedInputTokens estimates the input of the full summary
	// request: the trigger step's prompt and output, the trailing
	// messages, and the summary prompt.
	estimatedInputTokens int64
	// elideFirst sends the elided summary request before the full one
	// because the full one leaves too little of summaryWindow for the
	// summary.
	elideFirst bool

	// Logger records summary requests the model rejects for their size.
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
	// calls, including requests the model rejected for their size, and
	// the compaction step's billable runtime (see PersistedStep.Runtime).
	// Zero when the run was gated off before calling the model.
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
	// TriggerContextLimitTokens when it is positive. Like ContextTokens,
	// it measures the trigger step's prompt, so it can be below
	// ThresholdPercent when the step's output or the messages after it
	// triggered the compaction.
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
	// The result reports the measured usage, but the gate also counts what
	// the next model call sends after the trigger step's prompt.
	usagePercent, _ := shouldCompact(
		contextTokens,
		contextLimit,
		config.ThresholdPercent,
	)
	_, compact := shouldCompact(
		contextTokens+opts.TrailingTokens,
		contextLimit,
		config.ThresholdPercent,
	)
	if !compact && !config.Force {
		return CompactionResult{}, nil
	}

	// The summary request's input is the trigger step's prompt and output,
	// the messages after it, and the summary prompt appended by
	// generateCompactionSummary.
	config.estimatedInputTokens = stepTokensFromUsage(opts.StepUsage) + opts.SummaryTrailingTokens +
		estimateTokens(len(config.SummaryPrompt)+len(config.SummaryHint))
	config.summaryWindow = contextLimit
	if opts.SummaryContextLimit > 0 {
		config.summaryWindow = opts.SummaryContextLimit
	}
	minRoom := minSummaryOutputTokens
	if maxOutput := config.SummaryCall.MaxOutputTokens; maxOutput != nil {
		minRoom = min(minRoom, *maxOutput)
	}
	config.elideFirst = config.summaryWindow > 0 && config.summaryWindow-config.estimatedInputTokens < minRoom
	if config.elideFirst && opts.OmitToolDefinitions {
		// The step's usage also counts the tool definitions that the
		// summary requests omit. They stay in the estimate as a margin for
		// the output cap while the summary has room to spare. Near the
		// window the margin would shrink the cap below the floor, or leave
		// no room and so no clamp at all, so they come out.
		toolTokens, err := estimateToolTokens(opts.ToolDefinitions)
		if err != nil {
			config.Logger.Warn(ctx, "failed to estimate omitted tool definitions", slog.Error(err))
		} else {
			config.estimatedInputTokens -= toolTokens
			config.elideFirst = config.summaryWindow-config.estimatedInputTokens < minRoom
		}
	}

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
		Force:                opts.Force,
		Source:               opts.Source,
		ToolCallID:           opts.ToolCallID,
		ToolName:             opts.ToolName,
		PublishMessagePart:   opts.PublishMessagePart,
		Clock:                opts.Clock,
		StreamSilenceTimeout: opts.StreamSilenceTimeout,
		Logger:               opts.Logger,
	}
	if !opts.OmitToolDefinitions {
		config.ToolDefinitions = opts.ToolDefinitions
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
// the assistant message whose step reported the latest usage, typically the
// results of its tool calls. They are input to the next model call but
// absent from that usage. unmeasuredAssistants counts the assistant messages
// after that one, such as the partial response of an interrupted step,
// which persist without usage and so count as trailing too.
func EstimateTrailingTokens(messages []fantasy.Message, unmeasuredAssistants int) int64 {
	start := len(messages)
	for ; start > 0; start-- {
		if messages[start-1].Role != fantasy.MessageRoleAssistant {
			continue
		}
		if unmeasuredAssistants == 0 {
			break
		}
		unmeasuredAssistants--
	}
	return estimateMessagesTokens(messages[start:])
}

// estimateToolTokens approximates the prompt tokens of tool definitions
// from their JSON encoding.
func estimateToolTokens(tools []fantasy.Tool) (int64, error) {
	if len(tools) == 0 {
		return 0, nil
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return 0, xerrors.Errorf("marshal tool definitions: %w", err)
	}
	return estimateTokens(len(encoded)), nil
}

func estimateTokens(byteCount int) int64 {
	return int64((byteCount + bytesPerTokenEstimate - 1) / bytesPerTokenEstimate)
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
	if file, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
		if isMediaFile(file) {
			return 0, 1
		}
		// Other attachments, such as text documents, are text the model
		// reads in full.
		return len(file.Data), 0
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
// result and text attachment texts are cut to one common size, keeping
// their head and tail, so the copy's estimate fits budgetTokens. Cut texts
// keep at least elidedToolResultMinBytes, so the copy can still exceed the
// budget. It reports the bytes removed; zero means nothing was cut and
// messages is returned unchanged.
func elideToolResults(messages []fantasy.Message, budgetTokens int64) ([]fantasy.Message, int) {
	excessBytes := (estimateMessagesTokens(messages) - budgetTokens) * bytesPerTokenEstimate
	if excessBytes <= 0 {
		return messages, 0
	}
	var sizes []int
	for _, msg := range messages {
		for _, part := range msg.Content {
			if text, ok := elidableText(part); ok {
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
			text, ok := elidableText(part)
			if !ok || len(text) <= keepBytes {
				continue
			}
			truncated, _ := TruncateToolResultText(text, keepBytes)
			if content == nil {
				// The content slices are shared with the caller's messages.
				content = slices.Clone(msg.Content)
			}
			content[j] = withElidedText(part, truncated)
			removed += len(text) - len(truncated)
		}
		if content != nil {
			elided[i].Content = content
		}
	}
	return elided, removed
}

// elidableText returns the text of a locally executed tool result or a text
// attachment when that is longer than elidedToolResultMinBytes.
func elidableText(part fantasy.MessagePart) (string, bool) {
	var text string
	if file, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok && !isMediaFile(file) {
		text = string(file.Data)
	} else if result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok && !result.ProviderExecuted {
		if output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Output); ok {
			text = output.Text
		} else if output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](result.Output); ok && output.Error != nil {
			text = output.Error.Error()
		}
	}
	if len(text) <= elidedToolResultMinBytes {
		return "", false
	}
	return text, true
}

// withElidedText returns part, which elidableText accepted, with its text
// replaced. Attachments keep their filename and media type, and error
// results stay errors.
func withElidedText(part fantasy.MessagePart, text string) fantasy.MessagePart {
	if file, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
		file.Data = []byte(text)
		return file
	}
	result, _ := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
	if _, isError := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](result.Output); isError {
		result.Output = fantasy.ToolResultOutputContentError{Error: xerrors.New(text)}
	} else {
		result.Output = fantasy.ToolResultOutputContentText{Text: text}
	}
	return result
}

// isMediaFile reports whether file is an image or PDF, which providers
// tokenize as media rather than as the text of its bytes.
func isMediaFile(file fantasy.FilePart) bool {
	mediaType := chatfiles.BaseMediaType(file.MediaType)
	return strings.HasPrefix(mediaType, "image/") || mediaType == "application/pdf"
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
			nextBytes = int64(sizes[i+1])
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

// stepTokensFromUsage returns the tokens of a step's prompt and output,
// which are both input to the request after it. The TotalTokens fallback
// of contextTokensFromUsage already counts the output.
func stepTokensFromUsage(usage fantasy.Usage) int64 {
	if usage.InputTokens <= 0 && usage.CacheReadTokens <= 0 && usage.CacheCreationTokens <= 0 && usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return contextTokensFromUsage(usage) + usage.OutputTokens
}

// StepOutputTokens returns the tokens of a step's output that the prompt
// count of its usage leaves out. A usage report with only TotalTokens
// already counts them.
func StepOutputTokens(usage fantasy.Usage) int64 {
	return stepTokensFromUsage(usage) - contextTokensFromUsage(usage)
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

// summaryRequestKind names the shape of a summary request.
type summaryRequestKind string

// Summary request kinds, from the largest request to the smallest.
const (
	summaryRequestFull         summaryRequestKind = "full"
	summaryRequestWithoutTools summaryRequestKind = "without_tools"
	summaryRequestElided       summaryRequestKind = "elided"
)

// summaryRequest is the shape of one summary request.
type summaryRequest struct {
	kind            summaryRequestKind
	history         []fantasy.Message
	tools           []fantasy.Tool
	maxOutputTokens *int64
	inputTokens     int64
	elidedBytes     int
}

// newSummaryRequest builds the summary request of kind. It reports false
// when kind cannot shrink the request: no tools to drop or no tool results
// or text attachments to elide.
func newSummaryRequest(kind summaryRequestKind, messages []fantasy.Message, options CompactionOptions) (summaryRequest, bool) {
	request := summaryRequest{kind: kind, history: messages, inputTokens: options.estimatedInputTokens}
	switch kind {
	case summaryRequestFull:
		request.tools = options.ToolDefinitions
	case summaryRequestWithoutTools:
		if len(options.ToolDefinitions) == 0 {
			return summaryRequest{}, false
		}
	case summaryRequestElided:
		summaryTokens := estimateTokens(len(options.SummaryPrompt) + len(options.SummaryHint))
		historyTokens := estimateMessagesTokens(messages)
		if options.summaryWindow > 0 {
			// The summary prompt and hint go into the same request.
			historyTokens = min(historyTokens, max(options.summaryWindow-summaryTokens, 0))
		}
		// Halving leaves room for the summary and absorbs both the
		// estimate's error and a configured window larger than the
		// provider's real one.
		request.history, request.elidedBytes = elideToolResults(messages, historyTokens/2)
		if request.elidedBytes == 0 {
			return summaryRequest{}, false
		}
		request.inputTokens = estimateMessagesTokens(request.history) + summaryTokens
	default:
		return summaryRequest{}, false
	}
	request.maxOutputTokens = summaryOutputCap(options.SummaryCall.MaxOutputTokens, options.summaryWindow, request.inputTokens)
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
		attribute.Int(AttrSummaryRequestsSent, sent),
		attribute.String(AttrSummaryRequestKind, string(r.kind)),
		attribute.Int64(AttrSummaryEstimatedInputTokens, r.inputTokens),
		attribute.Int64(AttrSummaryMaxOutputTokens, ptr.NilToEmpty(r.maxOutputTokens)),
		attribute.Int(AttrSummaryToolCount, len(r.tools)),
		attribute.Int(AttrSummaryElidedBytes, r.elidedBytes),
	}
}

func (r summaryRequest) logFields(err error) []slog.Field {
	return []slog.Field{
		slog.F("summary_request_kind", string(r.kind)),
		slog.F("summary_estimated_input_tokens", r.inputTokens),
		slog.F("summary_max_output_tokens", ptr.NilToEmpty(r.maxOutputTokens)),
		slog.F("summary_tool_count", len(r.tools)),
		slog.F("summary_elided_bytes", r.elidedBytes),
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
// first without tool definitions, then with the largest tool results and
// text attachments elided. options.elideFirst starts with the elided request.
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
	kinds := []summaryRequestKind{summaryRequestFull, summaryRequestWithoutTools, summaryRequestElided}
	if options.elideFirst {
		// The full requests are larger, so they run only when there is
		// nothing to elide.
		kinds = []summaryRequestKind{summaryRequestElided, summaryRequestFull, summaryRequestWithoutTools}
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
		// Tool definitions keep the summary request on the parent turn's
		// cacheable prefix, but the tools and the full history both make
		// it larger than the turn that overflowed. The rejection can
		// arrive from the stream open or as an in-stream error part, so
		// the next request repeats the whole attempt.
		if !isContextTooLargeError(err) {
			break
		}
		options.Logger.Warn(ctx, "compaction summary request rejected for its size", request.logFields(err)...)
		// The elided request is the smallest, so its rejection ends the
		// attempts.
		if kind == summaryRequestElided {
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
// context limit" rejection. The OpenAI error code "context_length_exceeded"
// in a provider error's response body matches regardless of wording.
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
// Responses error events, as a plain *fantasy.Error whose message ends with
// the event's error code when it has one. The code then decides, so a rate
// limit or server error whose text mentions the context window is not taken
// for an overflow.
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
		if code := responsesErrorCode(fantasyErr.Message); code != "" {
			return code == "context_length_exceeded"
		}
		return hasContextTooLargePhrase(fantasyErr.Error())
	}
	return false
}

// responsesErrorCode returns the error code fantasy appends to the message
// of an OpenAI Responses failure as " (code: <code>)", or "" when there is
// none.
func responsesErrorCode(message string) string {
	const marker = " (code: "
	rest, ok := strings.CutSuffix(message, ")")
	if !ok {
		return ""
	}
	i := strings.LastIndex(rest, marker)
	if i < 0 {
		return ""
	}
	return rest[i+len(marker):]
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
