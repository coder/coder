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
	if rt == nil {
		return AdvisorResult{}, xerrors.New("advisor runtime is nil")
	}
	if rt.cfg.Model == nil {
		return AdvisorResult{}, xerrors.New("advisor runtime model is nil")
	}
	if rt.cfg.MaxUsesPerRun <= 0 {
		return AdvisorResult{}, xerrors.New("advisor runtime max uses per run is invalid")
	}
	if ctx == nil {
		return AdvisorResult{}, xerrors.New("advisor context is nil")
	}

	if !rt.tryAcquire() {
		return AdvisorResult{
			Type:          ResultTypeLimitReached,
			RemainingUses: 0,
		}, nil
	}

	// Clone per invocation and strip chain-mode markers so chatloop cannot
	// mutate the Runtime's stored options across calls, and so the nested
	// call never runs as a chain-mode continuation against stale parent state.
	nestedProviderOptions := cloneProviderOptions(rt.cfg.ProviderOptions)
	clearChainOnlyProviderOptions(nestedProviderOptions)

	var persistedStep chatloop.PersistedStep
	runOpts := chatloop.RunOptions{
		Model:           rt.cfg.Model,
		Messages:        BuildAdvisorMessages(question, conversationSnapshot),
		Tools:           nil,
		ProviderTools:   nil,
		MaxSteps:        1,
		ModelConfig:     rt.cfg.ModelConfig,
		ProviderOptions: nestedProviderOptions,
		PersistStep: func(_ context.Context, step chatloop.PersistedStep) error {
			persistedStep = step
			return nil
		},
	}
	if len(runOpts.Tools) != 0 {
		panic("chatadvisor: nested advisor run must not include tools")
	}
	if len(runOpts.ProviderTools) != 0 {
		panic("chatadvisor: nested advisor run must not include provider tools")
	}

	if err := chatloop.Run(ctx, runOpts); err != nil {
		return AdvisorResult{
			Type:          ResultTypeError,
			Error:         err.Error(),
			RemainingUses: rt.RemainingUses(),
		}, nil
	}

	advice := extractAdvisorText(persistedStep)
	if advice == "" {
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
