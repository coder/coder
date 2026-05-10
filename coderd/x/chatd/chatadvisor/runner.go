package chatadvisor

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
)

// RunAdvisor executes a single, tool-less nested advisor call.
func (rt *Runtime) RunAdvisor(
	ctx context.Context,
	question string,
	conversationSnapshot []fantasy.Message,
) (AdvisorResult, error) {
	// Model, MaxUsesPerRun, and MaxOutputTokens are validated by NewRuntime.
	// Runtime fields are unexported so callers cannot bypass that.
	if strings.TrimSpace(question) == "" {
		return AdvisorResult{}, xerrors.New("advisor question is required")
	}

	if !rt.tryAcquire() {
		return AdvisorResult{
			Type:          ResultTypeLimitReached,
			RemainingUses: 0,
		}, nil
	}

	// Clone per invocation and reset inherited state so chatloop cannot
	// mutate the Runtime's stored options across calls, and so the nested
	// call never runs as a chain-mode continuation against stale parent
	// state or persists an orphan stored response on the provider side.
	nestedProviderOptions := cloneProviderOptions(rt.cfg.ProviderOptions)
	resetProviderOptionsForNestedCall(nestedProviderOptions)

	var persistedStep chatloop.PersistedStep
	runOpts := chatloop.RunOptions{
		Model:           rt.cfg.Model,
		Messages:        BuildAdvisorMessages(question, conversationSnapshot),
		MaxSteps:        1,
		ModelConfig:     rt.cfg.ModelConfig,
		ProviderOptions: nestedProviderOptions,
		PersistStep: func(_ context.Context, step chatloop.PersistedStep) error {
			persistedStep = step
			return nil
		},
	}

	if err := chatloop.Run(ctx, runOpts); err != nil {
		// Refund the use so a transient provider failure does not
		// permanently exhaust the per-run advisor budget.
		rt.release()
		return AdvisorResult{
			Type:          ResultTypeError,
			Error:         err.Error(),
			RemainingUses: rt.RemainingUses(),
		}, nil
	}

	advice := extractAdvisorText(persistedStep)
	if advice == "" {
		// Refund: the run did not produce advice, so the contract
		// "increments on every successful advisor call" treats this
		// as not consuming a use.
		rt.release()
		return AdvisorResult{
			Type:          ResultTypeError,
			Error:         "advisor produced no text output",
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
