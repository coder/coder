package guardrail

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/tidwall/sjson"
	"golang.org/x/xerrors"
)

// Member is a guardrail attached to a hook with its per-membership settings.
type Member struct {
	Guardrail Guardrail
	Mode      Mode
	FailMode  FailMode
	// Timeout bounds the guardrail's network call. Zero means no per-member
	// deadline (the parent context still applies).
	Timeout time.Duration
}

func (m Member) name() string { return m.Guardrail.Name() }

// Stage is the set of guardrails for a single hook. Guardrails run
// concurrently (they are network-bound); their results are merged
// deterministically.
type Stage struct {
	members []Member
}

// NewStage builds a stage from its members. It returns an error if two members
// share a name, since names namespace annotations and attribute blocks.
func NewStage(members ...Member) (*Stage, error) {
	seen := make(map[string]struct{}, len(members))
	for _, m := range members {
		if m.Guardrail == nil {
			return nil, xerrors.New("nil guardrail in stage")
		}
		n := m.name()
		if _, dup := seen[n]; dup {
			return nil, xerrors.Errorf("duplicate guardrail name %q in stage", n)
		}
		seen[n] = struct{}{}
	}
	return &Stage{members: members}, nil
}

// Empty reports whether the stage has no members (a no-op stage).
func (s *Stage) Empty() bool { return s == nil || len(s.members) == 0 }

// StageResult is the merged outcome of every guardrail in the stage.
type StageResult struct {
	// Block reports that an enforcing guardrail rejected the request.
	Block bool
	// BlockedBy is the name of the blocking guardrail (lowest in name order on
	// ties), or empty when not blocked.
	BlockedBy string
	// Reason is the blocking guardrail's reason.
	Reason string
	// Annotations maps each guardrail's name to its annotation map, ready to
	// thread into input.annotations.
	Annotations map[string]any
	// Body is the request body after applying every enforcing guardrail's
	// edits, or nil when nothing was rewritten.
	Body []byte
	// Errors holds every guardrail's evaluation failure (unreachable / timeout
	// / non-2xx), for host logging. A fail-closed failure here also sets Block;
	// a fail-open failure is recorded but not blocking. The client-facing Reason
	// stays generic so internal details (endpoints, etc.) are not leaked.
	Errors []GuardrailError
}

// GuardrailError pairs a guardrail's name with the error it returned.
type GuardrailError struct {
	Name string
	Err  error
}

// memberOutcome is one member's evaluation, captured before the deterministic
// merge.
type memberOutcome struct {
	name   string
	mode   Mode
	result Result
	err    error
}

// Run evaluates every guardrail concurrently against body and merges the
// results. It never returns an error for a guardrail-level failure; such
// failures are folded into the StageResult per the member's fail mode (a
// fail-closed failure becomes a block). An error is returned only for an
// internal failure applying body edits.
func (s *Stage) Run(ctx context.Context, body []byte, model string) (StageResult, error) {
	if s.Empty() {
		return StageResult{}, nil
	}

	req := Request{
		Body:  body,
		Texts: UserPromptTexts(body),
		Model: model,
	}

	outcomes := make([]memberOutcome, len(s.members))
	var wg sync.WaitGroup
	for i, m := range s.members {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes[i] = evalMember(ctx, m, req)
		}()
	}
	wg.Wait()

	return s.merge(outcomes, body)
}

// evalMember runs a single member under its timeout and downgrades the result
// to the member's mode (advisory members keep annotations only).
func evalMember(ctx context.Context, m Member, req Request) memberOutcome {
	if m.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.Timeout)
		defer cancel()
	}
	res, err := m.Guardrail.Evaluate(ctx, req)
	return memberOutcome{name: m.name(), mode: m.Mode, result: res, err: err}
}

// merge reduces member outcomes into a StageResult: BLOCK wins (lowest name on
// ties), annotations are unioned under each guardrail's name, and enforcing
// edits apply as an ordered chain.
func (s *Stage) merge(outcomes []memberOutcome, body []byte) (StageResult, error) {
	// Deterministic order by guardrail name for block attribution and the edit
	// chain.
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].name < outcomes[j].name })

	var (
		out     StageResult
		edited  = body
		mutated bool
	)
	for _, o := range outcomes {
		if o.err != nil {
			// Record every failure for host logging, then apply the fail mode:
			// fail-closed failures block; fail-open failures are skipped.
			out.Errors = append(out.Errors, GuardrailError{Name: o.name, Err: o.err})
			if s.failMode(o.name) == FailClosed {
				if !out.Block {
					out.Block = true
					out.BlockedBy = o.name
					out.Reason = "guardrail unavailable: " + o.name
				}
			}
			continue
		}

		if len(o.result.Annotations) > 0 {
			if out.Annotations == nil {
				out.Annotations = make(map[string]any, len(outcomes))
			}
			out.Annotations[o.name] = o.result.Annotations
		}

		// Advisory members contribute annotations only.
		if o.mode != ModeEnforcing {
			continue
		}

		if o.result.Action == ActionBlock && !out.Block {
			out.Block = true
			out.BlockedBy = o.name
			out.Reason = o.result.Reason
		}

		for _, e := range o.result.Edits {
			next, err := sjson.SetBytes(edited, e.Pointer, e.Value)
			if err != nil {
				return StageResult{}, xerrors.Errorf("apply edit %q from guardrail %q: %w", e.Pointer, o.name, err)
			}
			edited = next
			mutated = true
		}
	}

	// A blocked request is never forwarded, so its body rewrite is moot.
	if mutated && !out.Block {
		out.Body = edited
	}
	return out, nil
}

// failMode resolves a member's fail mode by name.
func (s *Stage) failMode(name string) FailMode {
	for _, m := range s.members {
		if m.name() == name {
			return m.FailMode
		}
	}
	return FailClosed
}
