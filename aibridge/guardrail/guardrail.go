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

	"github.com/coder/coder/v2/aibridge/policy"
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

// FailMode controls behavior when a guardrail cannot produce a result
// (unreachable, timeout, 5xx). It mirrors policy.FailMode.
type FailMode int

const (
	// FailClosed blocks the request on guardrail error. Default.
	FailClosed FailMode = iota
	// FailOpen skips the failing guardrail and continues.
	FailOpen
)

// Identity is the resolved end-user identity made available to guardrail
// adapters (some vendors vary behavior by actor, and the generic API forwards a
// request_data block). It mirrors [policy.Identity].
type Identity = policy.Identity

// Role classifies a [TextRef]'s originating message role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Request is the input to a guardrail evaluation. It is built once per stage
// invocation and shared across every guardrail in that invocation. The host
// extracts text spans from the raw body so each adapter does not re-parse
// provider-specific message shapes; Body is retained so an adapter that rewrites
// whole structures can address edits back via [Edit.Pointer].
//
// Two extraction scopes are provided so an adapter reads exactly what it needs
// without re-walking Body:
//
//   - Prompt is the latest user prompt only (the masking-safe default; redacted
//     spans are written straight back to these pointers).
//   - Conversation is every role-tagged text span (system + all turns), for
//     scanners that need context (prompt-injection, jailbreak, secret scanning)
//     and would otherwise be forced to re-parse Body.
type Request struct {
	// Body is the raw JSON request body.
	Body []byte
	// Model is the request's target model, for adapters that vary behavior by
	// model.
	Model string
	// Prompt is the latest user prompt span(s) (see [UserPromptTexts]).
	Prompt []TextRef
	// Conversation is every role-tagged text span in the body (see
	// [ConversationTexts]).
	Conversation []TextRef
	// Identity is the resolved actor, for vendors/the generic API that key on
	// it. Decoupled from upstream-forwarded actor metadata; never sent upstream.
	Identity Identity
}

// TextRef is a single extracted text span and the sjson-addressable location it
// came from within the request body.
type TextRef struct {
	// Pointer is an sjson path (e.g. "messages.2.content") locating Value in
	// the request body.
	Pointer string
	// Role is the originating message role (empty is treated as user).
	Role Role
	// Value is the extracted text.
	Value string
}

// Edit is a single rewrite of the request body: set Pointer to Value. A root
// edit (Pointer == "") replaces the whole body. Value is any JSON value: text
// masking sets a string, but a vendor that rewrites a structured node (content
// array, whole body) sets the structured value directly. Guardrails run as a
// sequential chain in store order, each seeing the body as rewritten by the
// prior one, so two maskers compose rather than clobber.
type Edit struct {
	Pointer string
	Value   any
}

// Result is the outcome of one guardrail evaluation. A guardrail may set any
// combination of fields; the [Stage] reduces results across guardrails. A
// guardrail's authority is intrinsic to what its adapter returns (annotations
// always thread; an ActionBlock blocks; Edits mask), not a per-membership mode.
type Result struct {
	// Action is the guardrail's verdict.
	Action Action
	// Reason explains a block, surfaced in the 400 body and audit log.
	Reason string
	// Annotations are arbitrary classifier output (scores, detected entity
	// counts). They are threaded into input.annotations under the guardrail's
	// name so a Rego decide policy can read them.
	Annotations map[string]any
	// Edits rewrite the request body (masking/redaction). Only meaningful for a
	// mutating guardrail (see [Guardrail.Mutates]).
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
