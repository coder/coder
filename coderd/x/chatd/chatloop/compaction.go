package chatloop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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

	// compactionContextTooLargeMessage tells the user how to recover a
	// conversation that is too large to summarize.
	compactionContextTooLargeMessage = "This conversation is too large for the model to compact. " +
		"Send /clear to keep the transcript and start the next message with a fresh context. " +
		"You can also edit an earlier message, which deletes every message after it, or start a new conversation. " +
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
		windowLimit := contextLimit
		if opts.SummaryContextLimit > 0 {
			windowLimit = opts.SummaryContextLimit
		}
		remaining := windowLimit - contextTokens - reserved
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
	result.EstimatedContextTokens = int64((len(result.SystemSummary) + bytesPerTokenEstimate - 1) / bytesPerTokenEstimate)
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

func normalizedCompactionGenerateConfig(config GenerateCompactionOptions) (GenerateCompactionOptions, bool) {
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
		return GenerateCompactionOptions{}, false
	}
	return config, true
}

// publishCompactionError sends a tool-result error part so
// connected clients see that compaction failed.
func publishCompactionError(config GenerateCompactionOptions, msg string) {
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
	options GenerateCompactionOptions,
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

	return options.DebugSvc.StartRun(ctx, chatdebug.CreateRunParams{
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
	}, nil, 0)
}

// summaryToolResultCaps are the sizes, in bytes, that successive summary
// requests cut large tool results to after the model rejects the request
// without tool definitions for its size.
var summaryToolResultCaps = []int{32 << 10, 4 << 10}

// generateCompactionSummary asks the model to summarize the
// conversation so far. The provided messages should contain the
// complete history (system prompt, user/assistant turns, tool
// results). A final user message with the summary prompt is appended
// before calling the model.
//
// Compaction is the recovery path for an over-limit conversation, so each
// request the model rejects for its size is followed by a thinner one:
// first without tool definitions, then with large tool results cut to each
// of summaryToolResultCaps.
func generateCompactionSummary(
	ctx context.Context,
	model fantasy.LanguageModel,
	messages []fantasy.Message,
	options GenerateCompactionOptions,
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
	request := summaryRequest{history: messages, tools: options.ToolDefinitions}
	for {
		call := options.SummaryCall
		call.Prompt = summaryPromptFor(request.history)
		call.Tools = request.tools
		summary, err = streamSummaryText(call)
		// The rejection can arrive from the stream open or as an in-stream
		// error part, so the next request repeats the whole attempt.
		if !isContextTooLargeError(err) {
			return summary, responseID, err
		}
		options.Logger.Warn(ctx, "compaction summary request rejected for its size",
			slog.F("summary_tool_count", len(request.tools)),
			slog.F("summary_tool_result_max_bytes", request.toolResultMaxBytes),
			slog.F("summary_elided_bytes", request.elidedBytes),
			slog.Error(err),
		)
		next, ok := request.thinner(messages)
		if !ok {
			break
		}
		request = next
	}
	classified := chaterror.Classify(err)
	classified.Kind = codersdk.ChatErrorKindConfig
	classified.Message = compactionContextTooLargeMessage
	classified.Retryable = false
	classified.RetryAfter = 0
	return "", responseID, chaterror.WithClassification(err, classified)
}

// summaryRequest is the history and tool definitions of one summary
// request, and how much of the history's tool results it cut.
type summaryRequest struct {
	history            []fantasy.Message
	tools              []fantasy.Tool
	toolResultMaxBytes int
	elidedBytes        int
}

// thinner returns the request to send after the model rejects r for its
// size: r without tool definitions, then messages with their tool results
// cut to the first size in summaryToolResultCaps that cuts more than r did.
// It reports false when no request is smaller than r.
func (r summaryRequest) thinner(messages []fantasy.Message) (summaryRequest, bool) {
	if len(r.tools) > 0 {
		r.tools = nil
		return r, true
	}
	for _, maxBytes := range summaryToolResultCaps {
		history, elidedBytes := cutToolResults(messages, maxBytes)
		if elidedBytes > r.elidedBytes {
			return summaryRequest{history: history, toolResultMaxBytes: maxBytes, elidedBytes: elidedBytes}, true
		}
	}
	return summaryRequest{}, false
}

// cutToolResults returns messages with every locally executed text or error
// tool result longer than maxBytes cut to its head and tail, and the number
// of bytes removed. It copies what it changes, so the caller's messages
// stay intact.
func cutToolResults(messages []fantasy.Message, maxBytes int) ([]fantasy.Message, int) {
	var cut []fantasy.Message
	removed := 0
	for i, msg := range messages {
		var content []fantasy.MessagePart
		for j, part := range msg.Content {
			cutPart, partRemoved := cutToolResult(part, maxBytes)
			if partRemoved == 0 {
				continue
			}
			if content == nil {
				content = slices.Clone(msg.Content)
			}
			content[j] = cutPart
			removed += partRemoved
		}
		if content == nil {
			continue
		}
		if cut == nil {
			cut = slices.Clone(messages)
		}
		cut[i].Content = content
	}
	if cut == nil {
		return messages, 0
	}
	return cut, removed
}

// cutToolResult returns part cut to maxBytes when it is a locally executed
// text or error tool result longer than that, and the number of bytes
// removed. An error result stays an error.
func cutToolResult(part fantasy.MessagePart, maxBytes int) (fantasy.MessagePart, int) {
	result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
	if !ok || result.ProviderExecuted {
		return part, 0
	}
	if output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Output); ok {
		text, truncated := TruncateToolResultText(output.Text, maxBytes)
		if !truncated {
			return part, 0
		}
		result.Output = fantasy.ToolResultOutputContentText{Text: text}
		return result, len(output.Text) - len(text)
	}
	if output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](result.Output); ok && output.Error != nil {
		message := output.Error.Error()
		text, truncated := TruncateToolResultText(message, maxBytes)
		if !truncated {
			return part, 0
		}
		result.Output = fantasy.ToolResultOutputContentError{Error: xerrors.New(text)}
		return result, len(message) - len(text)
	}
	return part, 0
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
