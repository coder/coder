package chatadvisor

import (
	"sync/atomic"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"golang.org/x/xerrors"
)

// RuntimeConfig configures a single advisor runtime instance.
type RuntimeConfig struct {
	Model fantasy.LanguageModel
	// CallTemplate's provider options are cloned for each nested call.
	CallTemplate         fantasy.Call
	MaxUsesPerRun        int
	MaxOutputTokens      int64
	StreamSilenceTimeout time.Duration
	// MaxRetries is the maximum number of retries after a transient
	// provider error within one advisor call. It must be positive.
	MaxRetries int
	// PriorUses is the number of successful advisor calls already made
	// in the current turn. chatd builds a new Runtime for every model
	// step, so the per-turn quota is seeded from history.
	PriorUses int
}

// Runtime executes nested, tool-less advisor runs against the configured
// language model.
//
// The MaxUsesPerRun counter starts at PriorUses, increments on every
// successful advisor call, and is never reset. There is intentionally no
// Reset method: the quota is a safety bound on a single turn, not a
// rolling window.
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
	if cfg.MaxRetries < 1 {
		return nil, xerrors.New("advisor max retries must be positive")
	}
	if cfg.CallTemplate.MaxOutputTokens != nil &&
		*cfg.CallTemplate.MaxOutputTokens != cfg.MaxOutputTokens {
		return nil, xerrors.Errorf(
			"advisor call template max output tokens (%d) must match runtime max output tokens (%d)",
			*cfg.CallTemplate.MaxOutputTokens,
			cfg.MaxOutputTokens,
		)
	}

	normalized := cfg
	normalized.CallTemplate.ProviderOptions = cloneProviderOptions(cfg.CallTemplate.ProviderOptions)
	maxOutputTokens := cfg.MaxOutputTokens
	normalized.CallTemplate.MaxOutputTokens = &maxOutputTokens

	rt := &Runtime{cfg: normalized}
	rt.used.Store(int64(cfg.PriorUses))
	return rt, nil
}

// cloneProviderOptions returns a copy of opts with pointer entries for
// known, in-place mutated provider option types replaced by shallow struct
// copies, so a nested advisor call that disables Store does so on its own
// copy rather than the parent run's entry. Value fields such as Metadata
// and Include remain shared; callers that need true deep-copy semantics
// must handle those fields explicitly.
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

// resetProviderOptionsForNestedCall forces Store off so ephemeral advisor
// calls leave no stored response behind on the provider. It mutates opts
// in place, so it must be called on a cloned map, never on options shared
// with the parent run.
func resetProviderOptionsForNestedCall(opts fantasy.ProviderOptions) {
	for _, value := range opts {
		if typed, ok := value.(*fantasyopenai.ResponsesProviderOptions); ok && typed != nil {
			storeDisabled := false
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
// does not permanently consume quota for the turn.
func (rt *Runtime) release() {
	rt.used.Add(-1)
}
