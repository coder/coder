package chatloop

import (
	"context"
	"strings"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"
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
	OnError             func(error)
}

type CompactionResult struct {
	SystemSummary    string
	SummaryReport    string
	ThresholdPercent int32
	UsagePercent     float64
	ContextTokens    int64
	ContextLimit     int64
}

func maybeCompact(
	ctx context.Context,
	runOpts RunOptions,
	runResult *fantasy.AgentResult,
) error {
	if runResult == nil || runOpts.Compaction == nil {
		return nil
	}

	config := *runOpts.Compaction
	if config.Persist == nil {
		return xerrors.New("compaction persist callback is required")
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

	if config.ThresholdPercent >= maxCompactionThresholdPercent {
		return nil
	}
	if runOpts.MaxSteps > 0 && len(runResult.Steps) >= runOpts.MaxSteps {
		lastStep := runResult.Steps[len(runResult.Steps)-1]
		if lastStep.FinishReason == fantasy.FinishReasonToolCalls &&
			len(lastStep.Content.ToolCalls()) > 0 {
			return nil
		}
	}

	contextTokens := int64(0)
	contextLimitFromMetadata := int64(0)
	for i := len(runResult.Steps) - 1; i >= 0; i-- {
		usage := runResult.Steps[i].Usage
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
			hasContextTokens = true
		}
		if !hasContextTokens || total <= 0 {
			continue
		}

		contextTokens = total
		metadataLimit := extractContextLimit(runResult.Steps[i].ProviderMetadata)
		if metadataLimit.Valid && metadataLimit.Int64 > 0 {
			contextLimitFromMetadata = metadataLimit.Int64
		}
		break
	}
	if contextTokens <= 0 {
		return nil
	}

	contextLimit := contextLimitFromMetadata
	if contextLimit <= 0 && config.ContextLimit > 0 {
		contextLimit = config.ContextLimit
	}
	if contextLimit <= 0 && runOpts.ContextLimitFallback > 0 {
		contextLimit = runOpts.ContextLimitFallback
	}
	if contextLimit <= 0 {
		return nil
	}

	usagePercent := (float64(contextTokens) / float64(contextLimit)) * 100
	if usagePercent < float64(config.ThresholdPercent) {
		return nil
	}

	summary, err := generateCompactionSummary(
		ctx,
		runOpts.Model,
		runOpts.Messages,
		runResult.Steps,
		config,
	)
	if err != nil {
		return err
	}
	if summary == "" {
		return nil
	}

	systemSummary := strings.TrimSpace(
		config.SystemSummaryPrefix + "\n\n" + summary,
	)

	return config.Persist(ctx, CompactionResult{
		SystemSummary:    systemSummary,
		SummaryReport:    summary,
		ThresholdPercent: config.ThresholdPercent,
		UsagePercent:     usagePercent,
		ContextTokens:    contextTokens,
		ContextLimit:     contextLimit,
	})
}

func generateCompactionSummary(
	ctx context.Context,
	model fantasy.LanguageModel,
	messages []fantasy.Message,
	steps []fantasy.StepResult,
	options CompactionOptions,
) (string, error) {
	summaryPrompt := make([]fantasy.Message, 0, len(messages)+len(steps)+1)
	summaryPrompt = append(summaryPrompt, messages...)
	for _, step := range steps {
		summaryPrompt = append(summaryPrompt, step.Messages...)
	}
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
