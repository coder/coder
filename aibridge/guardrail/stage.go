package guardrail

import (
	"context"
	"sort"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/policy"
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
// concurrently (they are network-bound); their results project into the shared
// [policy.StageResult] and reduce through the single [policy.Reduce] + edits
// applier, exactly like the Rego policy pipeline.
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

// memberOutcome is one member's evaluation, captured before the deterministic
// merge.
type memberOutcome struct {
	name   string
	mode   Mode
	result Result
	err    error
}

// Run evaluates every guardrail concurrently against body and reduces the
// results into a [policy.Result]. It never returns an error for a
// guardrail-level failure; such failures are synthesized through the member's
// fail mode (a fail-closed failure becomes a BLOCK, fail-open a LOG). An error
// is returned only for an internal failure applying body edits.
func (s *Stage) Run(ctx context.Context, body []byte, model string) (policy.Result, error) {
	if s.Empty() {
		return policy.Result{}, nil
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

	return s.reduce(outcomes, body)
}

// evalMember runs a single member under its timeout.
func evalMember(ctx context.Context, m Member, req Request) memberOutcome {
	if m.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.Timeout)
		defer cancel()
	}
	res, err := m.Guardrail.Evaluate(ctx, req)
	return memberOutcome{name: m.name(), mode: m.Mode, result: res, err: err}
}

// reduce projects each member outcome into a [policy.StageResult] (a failure is
// synthesized through its fail mode; an advisory member contributes annotations
// only; an enforcing member may also block and emit edits), then runs the
// single shared reducer. Edits apply as a deterministic ordered chain by
// guardrail name and only when the request is not blocked (BLOCK freezes
// effects).
func (s *Stage) reduce(outcomes []memberOutcome, body []byte) (policy.Result, error) {
	// Deterministic order by guardrail name for block attribution and the edit
	// chain.
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].name < outcomes[j].name })

	staged := make([]policy.StageOutcome, 0, len(outcomes))
	var edits []policy.Edit
	for _, o := range outcomes {
		res := s.project(o)
		staged = append(staged, policy.StageOutcome{Name: o.name, Result: res})
		edits = append(edits, res.Edits...)
	}

	reduced := policy.Reduce(staged)
	out := policy.Result{
		Verdict:     reduced.Verdict,
		BlockedBy:   reduced.BlockedBy,
		Message:     reduced.Message,
		Annotations: reduced.Annotations,
		Errors:      reduced.Errors,
	}
	// A blocked request is never forwarded, so its body rewrite is moot.
	if !reduced.Verdict.Blocks() && len(edits) > 0 {
		edited, mutated, err := policy.ApplyEdits(body, edits)
		if err != nil {
			return policy.Result{}, err
		}
		if mutated {
			out.RequestBody = edited
		}
	}
	return out, nil
}

// project maps a member outcome to a [policy.StageResult] under its effect
// mask. A failure is synthesized through the member's fail mode; an advisory
// member keeps annotations only; an enforcing member may also block and emit
// edits. Annotations are stamped under the guardrail's namespace.
func (s *Stage) project(o memberOutcome) policy.StageResult {
	if o.err != nil {
		return policy.Synthesize(o.name, failModeToPolicy(s.failMode(o.name)), o.err)
	}

	res := policy.StageResult{}
	if len(o.result.Annotations) > 0 {
		res.Annotations = map[string]any{o.name: o.result.Annotations}
	}

	// Advisory members contribute annotations only.
	if o.mode != ModeEnforcing {
		return res
	}

	if o.result.Action == ActionBlock {
		res.Verdict = policy.VerdictBlock
		res.Message = o.result.Reason
	}
	for _, e := range o.result.Edits {
		res.Edits = append(res.Edits, policy.Edit{Pointer: e.Pointer, Value: e.Value})
	}
	return res
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

// failModeToPolicy maps a guardrail fail mode to the policy fail mode the shared
// synthesizer expects.
func failModeToPolicy(fm FailMode) policy.FailMode {
	if fm == FailOpen {
		return policy.FailOpen
	}
	return policy.FailClosed
}
