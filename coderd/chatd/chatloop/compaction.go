package chatloop

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultCompactionThresholdPercent = int32(70)
	minCompactionThresholdPercent     = int32(0)
	maxCompactionThresholdPercent     = int32(100)

	defaultCompactionSummaryPrompt = "Summarize the current chat so a " +
		"new assistant can continue seamlessly. Include the user's goals, " +
		"decisions made, concrete technical details (files, commands, APIs), " +
		"errors encountered and fixes, and open questions. Be dense and factual. " +
		"Omit pleasantries and next-step suggestions."
	defaultCompactionSystemSummaryPrefix = "Summary of earlier chat context:"
	defaultCompactionTimeout             = 90 * time.Second
)

type CompactionOptions struct {
	ThresholdPercent    int32
	ContextLimit        int64
	SummaryPrompt       string
	SystemSummaryPrefix string
	Timeout             time.Duration
	Persist             func(context.Context, CompactionResult) error

	// ToolCallID and ToolName identify the synthetic tool call
	// used to represent compaction in the message stream.
	ToolCallID string
	ToolName   string

	// PublishMessagePart publishes streaming parts to connected
	// clients so they see "Summarizing..." / "Summarized" UI
	// transitions during compaction.
	PublishMessagePart func(fantasy.MessageRole, codersdk.ChatMessagePart)

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
			fantasy.MessageRoleAssistant,
			codersdk.ChatMessagePart{
				Type:       codersdk.ChatMessagePartTypeToolCall,
				ToolCallID: config.ToolCallID,
				ToolName:   config.ToolName,
			},
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

	err = config.Persist(ctx, CompactionResult{
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
			fantasy.MessageRoleTool,
			codersdk.ChatMessagePart{
				Type:       codersdk.ChatMessagePartTypeToolResult,
				ToolCallID: config.ToolCallID,
				ToolName:   config.ToolName,
				Result:     resultJSON,
			},
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
		fantasy.MessageRoleTool,
		codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: config.ToolCallID,
			ToolName:   config.ToolName,
			Result:     errJSON,
			IsError:    true,
		},
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
) (string, error) {
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
