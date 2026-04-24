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
//
// Each Runtime instance is scoped to a single outer chat run. The
// MaxUsesPerRun counter increments on every successful advisor call and
// is never reset, so callers must construct a fresh Runtime (via
// NewRuntime) for each outer run. There is intentionally no Reset method:
// the per-run quota is a safety bound on a single run, not a rolling
// window.
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
	normalized.ProviderOptions = cloneProviderOptions(cfg.ProviderOptions)
	maxOutputTokens := cfg.MaxOutputTokens
	normalized.ModelConfig.MaxOutputTokens = &maxOutputTokens

	return &Runtime{cfg: normalized}, nil
}

// cloneProviderOptions returns a copy of opts with pointer entries for known,
// in-place mutated provider option types replaced by a shallow struct copy.
// chatloop mutates the OpenAI Responses entry (PreviousResponseID) on
// chain-mode exit, so sharing the pointer with the parent run would let an
// advisor call corrupt the parent's chain state. Value fields such as
// Metadata and Include are still shared with the parent; nothing in this
// package mutates them, but callers that need true deep-copy semantics must
// handle those fields explicitly.
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

// resetProviderOptionsForNestedCall strips inherited state from opts that
// does not apply to an ephemeral advisor call. PreviousResponseID is
// cleared so the nested call is not sent as a chain-mode continuation
// (BuildAdvisorMessages sends the full history, not an incremental turn).
// Store is forced off so the advisor call does not persist an orphan
// response on the provider side. Must be called on a cloned map to avoid
// mutating shared parent state.
func resetProviderOptionsForNestedCall(opts fantasy.ProviderOptions) {
	for _, value := range opts {
		if typed, ok := value.(*fantasyopenai.ResponsesProviderOptions); ok && typed != nil {
			storeDisabled := false
			typed.PreviousResponseID = nil
			typed.Store = &storeDisabled
		}
	}
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

// MaxOutputTokens reports the resolved output-token cap applied to each
// advisor call. NewRuntime validates that this value is positive and that
// it matches ModelConfig.MaxOutputTokens when both are set, so the
// accessor always returns the value the runtime will actually send.
func (rt *Runtime) MaxOutputTokens() int64 {
	if rt == nil {
		return 0
	}
	return rt.cfg.MaxOutputTokens
}

// ProviderOptions reports the resolved provider options applied to each
// advisor call. NewRuntime clones the supplied options so the returned
// map reflects what nested calls will actually receive; callers must not
// mutate the map or its entries.
func (rt *Runtime) ProviderOptions() fantasy.ProviderOptions {
	if rt == nil {
		return nil
	}
	return rt.cfg.ProviderOptions
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

// release returns a previously acquired use to the pool. Callers must
// invoke this at most once per successful tryAcquire when the advisor
// call did not complete successfully, so a transient provider failure
// does not permanently consume quota for the run.
func (rt *Runtime) release() {
	rt.used.Add(-1)
}
