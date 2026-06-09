// Package guardrail implements the networked head-of-hook guardrail stage of
// the AI Gateway policy engine. A guardrail is an external safety/DLP check
// (e.g. Presidio PII masking) reached over the network. Unlike a Rego policy it
// is not a "kind": a single guardrail invocation may simultaneously block,
// emit annotations, and rewrite the request body. Its result is reduced into
// the same plumbing the policy pipeline uses (BLOCK > LOG > ALLOW, annotation
// threading, body mutation + re-validation).
//
// This package is the runtime adapter layer only. Persistence, versioning, and
// the management API live elsewhere and construct a [Stage] from stored config.
package guardrail

import (
	"context"
)

// Action is a guardrail's verdict on a request.
type Action int

const (
	// ActionAllow lets the request proceed. The guardrail may still have
	// attached annotations or body edits.
	ActionAllow Action = iota
	// ActionBlock stops the request. The host returns HTTP 400 (the request
	// content was rejected), distinct from a policy BLOCK's 403.
	ActionBlock
)

// Mode is the per-membership authority a guardrail is granted.
type Mode int

const (
	// ModeAdvisory restricts a guardrail to annotations only: its block and
	// body edits are discarded, leaving a downstream Rego decide policy to turn
	// the annotated signal into a verdict.
	ModeAdvisory Mode = iota
	// ModeEnforcing lets a guardrail block and rewrite the body on its own
	// authority, in addition to annotating.
	ModeEnforcing
)

// FailMode controls behavior when a guardrail cannot produce a result
// (unreachable, timeout, 5xx). It mirrors policy.FailMode.
type FailMode int

const (
	// FailClosed blocks the request on guardrail error. Default.
	FailClosed FailMode = iota
	// FailOpen skips the failing guardrail and continues.
	FailOpen
)

// Request is the input to a guardrail evaluation. It is built once per stage
// and shared across every guardrail in the stage. Texts are extracted from the
// raw body so each adapter does not re-parse provider-specific message shapes;
// Body is retained so an adapter can address edits back into the original
// structure via [Edit.Pointer].
type Request struct {
	// Body is the raw JSON request body.
	Body []byte
	// Texts are the user-authored text spans the stage extracted from Body
	// (the latest user prompt; see [UserPromptTexts]), each paired with the
	// sjson pointer it was read from so a masking guardrail can write a redacted
	// value back to the same location.
	Texts []TextRef
	// Model is the request's target model, for adapters that vary behavior by
	// model.
	Model string
}

// TextRef is a single extracted text span and the sjson-addressable location
// it came from within the request body.
type TextRef struct {
	// Pointer is an sjson path (e.g. "messages.2.content") locating Value in
	// the request body.
	Pointer string
	// Value is the extracted text.
	Value string
}

// Edit is a single rewrite of the request body: set Pointer to Value. Edits
// from concurrent guardrails are applied as an ordered chain so two masking
// guardrails do not clobber each other via whole-body replacement.
type Edit struct {
	Pointer string
	Value   string
}

// Result is the outcome of one guardrail evaluation. A guardrail may set any
// combination of fields; the [Stage] reduces results across guardrails and
// applies the per-membership [Mode].
type Result struct {
	// Action is the guardrail's verdict.
	Action Action
	// Reason explains a block, surfaced in the 400 body and audit log.
	Reason string
	// Annotations are arbitrary classifier output (scores, detected entity
	// counts). They are threaded into input.annotations under the guardrail's
	// name so a Rego decide policy can read them.
	Annotations map[string]any
	// Edits rewrite the request body (masking/redaction).
	Edits []Edit
}

// Guardrail is a single networked check. Implementations must be safe for
// concurrent use and must honor ctx cancellation/deadline (the Stage bounds
// each call with the membership's network timeout).
type Guardrail interface {
	// Name identifies the guardrail; it namespaces the guardrail's annotations
	// and labels its metrics.
	Name() string
	// Evaluate runs the check against req. It must not mutate req. A transport
	// error (unreachable, timeout, non-2xx) is returned as a non-nil error so
	// the Stage can apply the membership fail mode.
	Evaluate(ctx context.Context, req Request) (Result, error)
}
