package chatadvisor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/codersdk"
)

// RunAdvisorOptions carries optional streaming callbacks for a
// single RunAdvisor invocation.
type RunAdvisorOptions struct {
	OnAdviceDelta func(delta string)
	OnAdviceReset func()
}

// RunAdvisor executes a single, tool-less nested advisor call.
func (rt *Runtime) RunAdvisor(
	ctx context.Context,
	question string,
	conversationSnapshot []fantasy.Message,
	opts *RunAdvisorOptions,
) (AdvisorResult, error) {
	// Model, MaxUsesPerRun, and MaxOutputTokens are validated by NewRuntime.
	// Runtime fields are unexported so callers cannot bypass that.
	question = strings.TrimSpace(question)
	if question == "" {
		return AdvisorResult{}, xerrors.New("advisor question is required")
	}
	question = stringutil.Truncate(question, advisorQuestionMaxRunes)

	if !rt.tryAcquire() {
		return AdvisorResult{
			Type:          ResultTypeLimitReached,
			RemainingUses: 0,
		}, nil
	}

	// resetProviderOptionsForNestedCall mutates its argument; give it a
	// clone so the Runtime's stored options stay unchanged across calls.
	nestedProviderOptions := cloneProviderOptions(rt.cfg.ProviderOptions)
	resetProviderOptionsForNestedCall(nestedProviderOptions)

	assistantOpts := chatloop.GenerateAssistantOptions{
		Model:           rt.cfg.Model,
		Messages:        BuildAdvisorMessages(question, conversationSnapshot),
		ModelConfig:     rt.cfg.ModelConfig,
		ProviderOptions: nestedProviderOptions,
	}
	if opts != nil && opts.OnAdviceDelta != nil {
		assistantOpts.PublishMessagePart = func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			if role != codersdk.ChatMessageRoleAssistant ||
				part.Type != codersdk.ChatMessagePartTypeText ||
				part.Text == "" {
				return
			}
			opts.OnAdviceDelta(part.Text)
		}
	}

	var outcome chatloop.AssistantOutcome
	if err := chatretry.Retry(ctx, func(retryCtx context.Context) error {
		var err error
		outcome, err = chatloop.GenerateAssistant(retryCtx, assistantOpts)
		return err
	}, func(int, error, chatretry.ClassifiedError, time.Duration) {
		if opts != nil && opts.OnAdviceReset != nil {
			opts.OnAdviceReset()
		}
	}); err != nil {
		// Refund the use so a transient provider failure does not
		// permanently exhaust the per-run advisor budget.
		rt.release()
		return AdvisorResult{
			Type:          ResultTypeError,
			Error:         err.Error(),
			RemainingUses: rt.RemainingUses(),
		}, nil
	}

	advice := extractAdvisorText(outcome.Step)
	if advice == "" {
		// Refund: the run did not produce advice, so the contract
		// "increments on every successful advisor call" treats this
		// as not consuming a use.
		rt.release()
		return AdvisorResult{
			Type: ResultTypeError,
			Error: fmt.Sprintf(
				"advisor produced no text output (%s)",
				describeTextlessOutcome(outcome),
			),
			RemainingUses: rt.RemainingUses(),
		}, nil
	}

	return AdvisorResult{
		Type:          ResultTypeAdvice,
		Advice:        advice,
		AdvisorModel:  rt.cfg.Model.Provider() + "/" + rt.cfg.Model.Model(),
		RemainingUses: rt.RemainingUses(),
	}, nil
}

func extractAdvisorText(step chatloop.PersistedStep) string {
	parts := make([]string, 0, len(step.Content))
	for _, content := range step.Content {
		text, ok := fantasy.AsContentType[fantasy.TextContent](content)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(text.Text)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// describeTextlessOutcome summarizes a step that yielded no usable advice
// text so the error pinpoints the failure mode. A reasoning-only step means
// the model spent its turn deciding on an action (such as a tool call it
// cannot perform in this tool-less run) without answering; a length finish
// means the output was truncated before any text was produced.
func describeTextlessOutcome(outcome chatloop.AssistantOutcome) string {
	var text, reasoning, toolCalls, other int
	for _, content := range outcome.Step.Content {
		switch content.(type) {
		case fantasy.TextContent:
			text++
		case fantasy.ReasoningContent:
			reasoning++
		case fantasy.ToolCallContent:
			toolCalls++
		default:
			other++
		}
	}
	if len(outcome.ToolCalls) > toolCalls {
		toolCalls = len(outcome.ToolCalls)
	}

	kinds := make([]string, 0, 4)
	appendKind := func(name string, count int) {
		if count > 0 {
			kinds = append(kinds, fmt.Sprintf("%s=%d", name, count))
		}
	}
	// Text parts can only reach here blank, so label them accordingly.
	appendKind("blank_text", text)
	appendKind("reasoning", reasoning)
	appendKind("tool_call", toolCalls)
	appendKind("other", other)

	summary := "none"
	if len(kinds) > 0 {
		summary = strings.Join(kinds, ", ")
	}
	return fmt.Sprintf("finish_reason=%s; parts: %s", outcome.FinishReason, summary)
}
