package chatadvisor

import (
	"sync/atomic"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// RuntimeConfig configures a single advisor runtime instance.
type RuntimeConfig struct {
	Model           fantasy.LanguageModel
	ModelConfig     codersdk.ChatModelCallConfig
	ProviderOptions fantasy.ProviderOptions
	MaxUsesPerRun   int
	MaxOutputTokens int64
}

// Runtime executes nested, tool-less advisor runs against the configured
// language model.
type Runtime struct {
	cfg  RuntimeConfig
	used atomic.Int64
}

// NewRuntime validates and normalizes advisor runtime configuration.
func NewRuntime(cfg RuntimeConfig) (*Runtime, error) {
	if cfg.Model == nil {
		return nil, xerrors.New("advisor model is required")
	}
	if cfg.MaxUsesPerRun <= 0 {
		return nil, xerrors.New("advisor max uses per run must be positive")
	}
	if cfg.MaxOutputTokens <= 0 {
		return nil, xerrors.New("advisor max output tokens must be positive")
	}
	if cfg.ModelConfig.MaxOutputTokens != nil &&
		*cfg.ModelConfig.MaxOutputTokens != cfg.MaxOutputTokens {
		return nil, xerrors.Errorf(
			"advisor model_config.max_output_tokens (%d) must match runtime max output tokens (%d)",
			*cfg.ModelConfig.MaxOutputTokens,
			cfg.MaxOutputTokens,
		)
	}

	normalized := cfg
	normalized.ModelConfig = cfg.ModelConfig
	normalized.ProviderOptions = cloneProviderOptions(cfg.ProviderOptions)
	maxOutputTokens := cfg.MaxOutputTokens
	normalized.ModelConfig.MaxOutputTokens = &maxOutputTokens

	return &Runtime{cfg: normalized}, nil
}

// cloneProviderOptions returns a copy of opts in which pointer entries for
// known, in-place mutated provider option types are deep-copied. chatloop
// mutates the OpenAI Responses entry (PreviousResponseID) on chain-mode
// exit, so sharing the pointer with the parent run would let an advisor
// call corrupt the parent's chain state.
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

// RemainingUses reports how many advisor calls are still available for the
// current runtime.
func (rt *Runtime) RemainingUses() int {
	if rt == nil || rt.cfg.MaxUsesPerRun <= 0 {
		return 0
	}

	remaining := int64(rt.cfg.MaxUsesPerRun) - rt.used.Load()
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}

func (rt *Runtime) tryAcquire() bool {
	for {
		used := rt.used.Load()
		if used >= int64(rt.cfg.MaxUsesPerRun) {
			return false
		}
		if rt.used.CompareAndSwap(used, used+1) {
			return true
		}
	}
}
