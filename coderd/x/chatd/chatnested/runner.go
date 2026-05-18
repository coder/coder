// Package chatnested runs one-step nested text model calls for chat features.
package chatnested

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/codersdk"
)

// RunTextOptions configures a nested one-step, tool-less text call.
type RunTextOptions struct {
	Model                fantasy.LanguageModel
	Messages             []fantasy.Message
	ModelConfig          codersdk.ChatModelCallConfig
	ProviderOptions      fantasy.ProviderOptions
	ContextLimitFallback int64
	Logger               slog.Logger
	Metrics              *chatloop.Metrics

	OnTextDelta func(delta string)
	OnTextReset func()
}

// RunTextResult contains the final text and accounting metadata for a nested
// text call.
type RunTextResult struct {
	Text               string
	Usage              codersdk.ChatMessageUsage
	ContextLimit       sql.NullInt64
	ProviderResponseID string
	Runtime            time.Duration
}

// RunText executes a one-step nested model call without any tools or provider
// side conversation storage.
func RunText(ctx context.Context, opts RunTextOptions) (RunTextResult, error) {
	if opts.Model == nil {
		return RunTextResult{}, xerrors.New("nested text model is required")
	}

	providerOptions := cloneProviderOptions(opts.ProviderOptions)
	resetProviderOptions(providerOptions)

	assistantOpts := chatloop.GenerateAssistantOptions{
		Model:                opts.Model,
		Messages:             opts.Messages,
		ModelConfig:          opts.ModelConfig,
		ProviderOptions:      providerOptions,
		ContextLimitFallback: opts.ContextLimitFallback,
		Logger:               opts.Logger,
		Metrics:              opts.Metrics,
	}
	if opts.OnTextDelta != nil {
		assistantOpts.PublishMessagePart = func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			if role != codersdk.ChatMessageRoleAssistant ||
				part.Type != codersdk.ChatMessagePartTypeText ||
				part.Text == "" {
				return
			}
			opts.OnTextDelta(part.Text)
		}
	}

	var outcome chatloop.AssistantOutcome
	if err := chatretry.Retry(ctx, func(retryCtx context.Context) error {
		var err error
		outcome, err = chatloop.GenerateAssistant(retryCtx, assistantOpts)
		return err
	}, func(int, error, chatretry.ClassifiedError, time.Duration) {
		if opts.OnTextReset != nil {
			opts.OnTextReset()
		}
	}); err != nil {
		return RunTextResult{}, err
	}
	step := outcome.Step
	return RunTextResult{
		Text:               extractText(step),
		Usage:              fantasyUsageToChatMessageUsage(step.Usage),
		ContextLimit:       step.ContextLimit,
		ProviderResponseID: step.ProviderResponseID,
		Runtime:            step.Runtime,
	}, nil
}

func cloneProviderOptions(opts fantasy.ProviderOptions) fantasy.ProviderOptions {
	if opts == nil {
		return nil
	}
	cloned := make(fantasy.ProviderOptions, len(opts))
	for key, value := range opts {
		switch typed := value.(type) {
		case *fantasyopenai.ResponsesProviderOptions:
			if typed == nil {
				cloned[key] = value
				continue
			}
			copied := *typed
			cloned[key] = &copied
		default:
			cloned[key] = value
		}
	}
	return cloned
}

func resetProviderOptions(opts fantasy.ProviderOptions) {
	for _, value := range opts {
		if typed, ok := value.(*fantasyopenai.ResponsesProviderOptions); ok && typed != nil {
			storeDisabled := false
			typed.PreviousResponseID = nil
			typed.Store = &storeDisabled
		}
	}
}

func extractText(step chatloop.PersistedStep) string {
	parts := make([]string, 0, len(step.Content))
	for _, content := range step.Content {
		text, ok := fantasy.AsContentType[fantasy.TextContent](content)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(text.Text)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func fantasyUsageToChatMessageUsage(usage fantasy.Usage) codersdk.ChatMessageUsage {
	return codersdk.ChatMessageUsage{
		InputTokens:         int64PtrIfPositive(usage.InputTokens),
		OutputTokens:        int64PtrIfPositive(usage.OutputTokens),
		TotalTokens:         int64PtrIfPositive(usage.TotalTokens),
		ReasoningTokens:     int64PtrIfPositive(usage.ReasoningTokens),
		CacheCreationTokens: int64PtrIfPositive(usage.CacheCreationTokens),
		CacheReadTokens:     int64PtrIfPositive(usage.CacheReadTokens),
	}
}

func int64PtrIfPositive(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}
