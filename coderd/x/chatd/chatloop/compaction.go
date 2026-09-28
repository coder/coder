package chatloop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	defaultCompactionThresholdPercent = int32(70)
	minCompactionThresholdPercent     = int32(0)
	maxCompactionThresholdPercent     = int32(100)

	// compactionDebugCreateRunTimeout caps the compaction debug
	// CreateRun budget. Debug instrumentation is best-effort;
	// running without the debug row is preferable to blocking
	// compaction on a slow or locked DB.
	compactionDebugCreateRunTimeout = 5 * time.Second

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

type CompactionResult struct {
	SystemSummary    string
	SummaryReport    string
	Source           CompactionSource
	ThresholdPercent int32
	UsagePercent     float64
	ContextTokens    int64
	ContextLimit     int64
	// EstimatedContextTokens covers only SystemSummary, not the full prompt.
	EstimatedContextTokens int64
	// Runtime is the wall-clock duration of the summarization model
	// call, the compaction step's billable runtime (see
	// PersistedStep.Runtime). Zero when the run was gated off before
	// calling the model.
	Runtime time.Duration
	// ProviderResponseID identifies the summary response. See
	// PersistedStep.ProviderResponseID.
	ProviderResponseID string
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
	usagePercent, compact := shouldCompact(
		contextTokens,
		contextLimit,
		config.ThresholdPercent,
	)
	if !compact && !config.Force {
		return CompactionResult{}, nil
	}

	// Sum-enforcing providers reject requests whose input plus
	// max_tokens exceeds the context window, so bound the summary cap
	// by the remaining window. contextTokens covers only the trigger
	// step's prompt, so also reserve that step's output, the tool
	// results executed after it, and the summary prompt appended by
	// generateCompactionSummary, all of which become input to the
	// summary request. Degenerate cases (unknown limit, no room left)
	// leave the cap unchanged.
	if config.SummaryCall.MaxOutputTokens != nil {
		promptBytes := len(config.SummaryPrompt) + len(config.SummaryHint)
		promptTokens := int64((promptBytes + bytesPerTokenEstimate - 1) / bytesPerTokenEstimate)
		reserved := opts.StepUsage.OutputTokens + promptTokens + trailingToolResultTokens(opts.Messages)
		remaining := contextLimit - contextTokens - reserved
		if remaining > 0 && remaining < *config.SummaryCall.MaxOutputTokens {
			config.SummaryCall.MaxOutputTokens = &remaining
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
		SummaryReport:      summary,
		Source:             config.Source,
		ThresholdPercent:   config.ThresholdPercent,
		UsagePercent:       usagePercent,
		ContextTokens:      contextTokens,
		ContextLimit:       contextLimit,
		Runtime:            summaryRuntime,
		ProviderResponseID: responseID,
	}
	result.EstimatedContextTokens = int64((len(result.SystemSummary) + bytesPerTokenEstimate - 1) / bytesPerTokenEstimate)
	if config.PublishMessagePart != nil && config.ToolCallID != "" {
		resultJSON, _ := json.Marshal(map[string]any{
			"summary":                  summary,
			"source":                   config.Source,
			"threshold_percent":        config.ThresholdPercent,
			"usage_percent":            usagePercent,
			"context_tokens":           contextTokens,
			"context_limit_tokens":     contextLimit,
			"estimated_context_tokens": result.EstimatedContextTokens,
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
		config.ThresholdPercent > maxCompactionThresholdPercent {
		config.ThresholdPercent = defaultCompactionThresholdPercent
	}
	// threshold=100 disables automatic compaction; a forced run
	// still proceeds because the user asked explicitly.
	if config.ThresholdPercent == maxCompactionThresholdPercent && !config.Force {
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

// trailingToolResultTokens estimates tokens for the tool-result
// messages that follow the usage-measured assistant step. Local tools
// execute before compaction, so their results are input to the summary
// request but absent from StepUsage.
func trailingToolResultTokens(messages []fantasy.Message) int64 {
	totalBytes := 0
	for i := len(messages) - 1; i >= 0 && messages[i].Role == fantasy.MessageRoleTool; i-- {
		for _, part := range messages[i].Content {
			result, ok := part.(fantasy.ToolResultPart)
			if !ok {
				resultPtr, okPtr := part.(*fantasy.ToolResultPart)
				if !okPtr || resultPtr == nil {
					continue
				}
				result = *resultPtr
			}
			switch output := result.Output.(type) {
			case fantasy.ToolResultOutputContentText:
				totalBytes += len(output.Text)
			case fantasy.ToolResultOutputContentError:
				if output.Error != nil {
					totalBytes += len(output.Error.Error())
				}
			case fantasy.ToolResultOutputContentMedia:
				totalBytes += len(output.Data) + len(output.Text)
			}
		}
	}
	return int64((totalBytes + bytesPerTokenEstimate - 1) / bytesPerTokenEstimate)
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
) (summary string, responseID string, err error) {
	summaryPrompt := make([]fantasy.Message, 0, len(messages)+1)
	summaryPrompt = append(summaryPrompt, messages...)
	summaryParts := []fantasy.MessagePart{fantasy.TextPart{Text: options.SummaryPrompt}}
	if strings.TrimSpace(options.SummaryHint) != "" {
		summaryParts = append(summaryParts, fantasy.TextPart{Text: options.SummaryHint})
	}
	summaryPrompt = append(summaryPrompt, fantasy.Message{
		Role:    fantasy.MessageRoleUser,
		Content: summaryParts,
	})
	// Anthropic only reads the cache at explicit breakpoints, so without
	// these the shared tool and history prefix is never a cache hit.
	if shouldApplyAnthropicPromptCaching(model) {
		addAnthropicPromptCaching(summaryPrompt)
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

	call := options.SummaryCall
	call.Prompt = summaryPrompt
	call.Tools = options.ToolDefinitions
	clock := options.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}
	timeout := options.StreamSilenceTimeout
	if timeout == 0 {
		timeout = DefaultStreamSilenceTimeout
	}
	// NopMetrics: TTFT is an assistant-generation metric, so the summary
	// stream must not record into it.
	streamSummaryText := func() (string, error) {
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
	summary, err = streamSummaryText()
	if err != nil && len(call.Tools) > 0 && isContextTooLargeError(err) {
		// Tool definitions keep the summary request on the parent turn's
		// cacheable prefix, but they also make it larger than the turn
		// that just overflowed. Compaction is the recovery path for an
		// over-limit conversation, so fall back to the tool-less request,
		// which still fits whenever the history alone does. The rejection
		// can arrive from the stream open or as an in-stream error part,
		// so the retry wraps the whole attempt.
		call.Tools = nil
		summary, err = streamSummaryText()
	}
	return summary, responseID, err
}

// contextTooLargePhrases are context-window rejections fantasy does not
// parse, such as the OpenAI Responses API "Your input exceeds the context
// window of this model." and Anthropic-family "too long" rejections
// without token counts. The response body is included so the OpenAI
// error code "context_length_exceeded" matches regardless of wording.
var contextTooLargePhrases = []string{
	"context window",
	"context length",
	"context_length",
	"too long",
}

// isContextTooLargeError reports whether a provider rejected a request for
// its size: the prompt exceeded the model's context window, or the body
// exceeded a request size limit on the provider or a gateway in front of it.
func isContextTooLargeError(err error) bool {
	providerErr, ok := errors.AsType[*fantasy.ProviderError](err)
	if !ok {
		return false
	}
	if providerErr.IsContextTooLarge() || providerErr.StatusCode == http.StatusRequestEntityTooLarge {
		return true
	}
	if providerErr.StatusCode != http.StatusBadRequest {
		return false
	}
	text := strings.ToLower(providerErr.Error() + " " + string(providerErr.ResponseBody))
	for _, phrase := range contextTooLargePhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
