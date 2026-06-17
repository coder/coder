package guardrail

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/policy"
)

// Member is a guardrail attached to a hook with its per-membership settings.
// Members are evaluated in slice order, which the loader populates from the
// stored position column, so the operator controls the masking chain order.
type Member struct {
	Guardrail Guardrail
	FailMode  FailMode
	// Timeout bounds the guardrail's network call. Zero means no per-member
	// deadline (the parent context still applies).
	Timeout time.Duration
}

func (m Member) name() string { return m.Guardrail.Name() }

// Stage is the ordered set of guardrails for a single hook. Guardrails run as a
// sequential chain (matching litellm/Portkey): each member sees the request
// body as rewritten by the members before it, so two maskers compose
// deterministically instead of racing to clobber overlapping edits. Results
// project into the shared [policy.StageResult] and reduce through the single
// [policy.Reduce], exactly like the Rego policy pipeline. A deliberate BLOCK
// stops the chain (later vendor calls are pointless once the request is
// rejected); annotations from members that ran are always retained.
type Stage struct {
	members []Member
}

// NewStage builds a stage from its members, preserving their order. It returns
// an error if two members share a name, since names namespace annotations and
// attribute blocks.
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

// Run evaluates every guardrail in order against the request and reduces the
// results into a [policy.Result]. The body is threaded through the chain: each
// member's edits are applied before the next member runs (and its text spans
// re-extracted from the rewritten body), so a downstream masker scans what an
// upstream masker already redacted. identity is exposed to adapters that key on
// the actor (and the generic API's request_data block).
//
// It never returns an error for a guardrail-level failure; such failures are
// synthesized through the member's fail mode (a fail-closed failure becomes a
// BLOCK, fail-open a LOG). An error is returned only for an internal failure
// applying body edits.
func (s *Stage) Run(ctx context.Context, body []byte, model string, identity Identity) (policy.Result, error) {
	if s.Empty() {
		return policy.Result{}, nil
	}

	curBody := body
	staged := make([]policy.StageOutcome, 0, len(s.members))
	blocked := false
	mutated := false

	for _, m := range s.members {
		req := Request{
			Body:         curBody,
			Model:        model,
			Prompt:       UserPromptTexts(curBody),
			Conversation: ConversationTexts(curBody),
			Identity:     identity,
		}
		res := projectMember(ctx, m, req)
		staged = append(staged, policy.StageOutcome{Name: m.name(), Result: res})

		if res.Verdict.Blocks() {
			// BLOCK freezes effects: stop the chain, the request is rejected and
			// its body is moot. Remaining members would only waste vendor calls.
			blocked = true
			break
		}
		// Thread this member's edits into the body so the next member (and the
		// downstream policy pipeline) sees the rewritten request.
		if len(res.Edits) > 0 {
			edited, changed, err := policy.ApplyEdits(curBody, res.Edits)
			if err != nil {
				return policy.Result{}, err
			}
			if changed {
				curBody = edited
				mutated = true
			}
		}
	}

	reduced := policy.Reduce(staged)
	out := policy.Result{
		Verdict:     reduced.Verdict,
		BlockedBy:   reduced.BlockedBy,
		Message:     reduced.Message,
		Annotations: reduced.Annotations,
		Errors:      reduced.Errors,
	}
	// Surface the rewritten body only when the request proceeds and a member
	// actually changed it (a blocked request is never forwarded).
	if !blocked && !reduced.Verdict.Blocks() && mutated {
		out.RequestBody = curBody
	}
	return out, nil
}

// project evaluates a member under its timeout and maps the outcome to a
// [policy.StageResult] via [policy.Resolve], which stamps the guardrail's
// namespace from its name. A failure resolves a [policy.Failure] through the
// member's fail mode; otherwise the outcome is decoded into a
// [policy.GuardrailOutcome] whose intrinsic effects (annotations always; an
// ActionBlock blocks; Edits mask) Resolve namespaces. The guardrail package
// builds the Projector but never constructs or namespaces a StageResult itself.
func projectMember(ctx context.Context, m Member, req Request) policy.StageResult {
	res, err := evalMember(ctx, m, req)
	if err != nil {
		return policy.Resolve(m.name(), policy.Failure{FailMode: failModeToPolicy(m.FailMode), Err: err})
	}

	out := policy.GuardrailOutcome{
		Annotations: res.Annotations,
		Block:       res.Action == ActionBlock,
		Message:     res.Reason,
	}
	for _, e := range res.Edits {
		out.Edits = append(out.Edits, policy.Edit{Pointer: e.Pointer, Value: e.Value})
	}
	return policy.Resolve(m.name(), out)
}

// evalMember runs a single member under its timeout.
func evalMember(ctx context.Context, m Member, req Request) (Result, error) {
	if m.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.Timeout)
		defer cancel()
	}
	return m.Guardrail.Evaluate(ctx, req)
}

// failModeToPolicy maps a guardrail fail mode to the policy fail mode the shared
// synthesizer expects.
func failModeToPolicy(fm FailMode) policy.FailMode {
	if fm == FailOpen {
		return policy.FailOpen
	}
	return policy.FailClosed
}
