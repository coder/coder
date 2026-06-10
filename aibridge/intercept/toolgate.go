package intercept

import (
	"context"
	"encoding/json"
	"time"
)

const (
	// HoldMaxBytes caps the encoded bytes buffered while holding a single
	// client-bound tool block for gating. Tool arguments are normally small but
	// are attacker-influenceable, so the cap bounds memory; exceeding it is
	// resolved by the gate's fail mode. Generous by design.
	HoldMaxBytes = 4 << 20 // 4 MiB
	// HoldDeadline bounds how long a single tool block may be held before its
	// arguments complete. It converts a silently stalled upstream into an
	// attributable error rather than policing normal generation. A fully silent
	// upstream is additionally bounded by the request/stream context.
	HoldDeadline = 5 * time.Minute
)

// ToolCall is a single model-requested tool call presented to a [ToolGate] for
// evaluation. Arguments is the raw JSON arguments object as assembled from the
// upstream stream; Index is the zero-based position of the call within its turn.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
	Index     int
}

// ToolGateDecision is the outcome of gating a single tool call.
type ToolGateDecision struct {
	// Block reports whether the tool call must not be released to the client.
	Block bool
	// BlockedBy names the policy that produced the BLOCK verdict, when known.
	BlockedBy string
	// Reason is a human-readable explanation surfaced to the client.
	Reason string
}

// ToolGate evaluates an assembled, client-bound tool call at the pre-tool hook.
// Implementations are pure with respect to the call (no upstream effect); a
// BLOCK decision causes the interceptor to terminate the turn with an error.
//
// FailOpen reports the aggregate fail mode of the gate: when true, an internal
// gate failure (e.g. an over-cap or incomplete held block that cannot be
// evaluated) releases the call unevaluated; when false the call is blocked.
type ToolGate interface {
	EvaluateToolCall(ctx context.Context, call ToolCall) (ToolGateDecision, error)
	FailOpen() bool
	// ObserveHold records how long a client-bound tool block was held for
	// gating, in seconds. It lets the interceptor report timing without a direct
	// metrics dependency.
	ObserveHold(seconds float64)
}

// ToolHoldOutcome tells a streaming interceptor how to proceed with a held
// client-bound tool block after gating.
type ToolHoldOutcome int

const (
	// ToolHoldRelease flushes the held events to the client and resumes
	// streaming.
	ToolHoldRelease ToolHoldOutcome = iota
	// ToolHoldTerminate discards the held events, emits a terminal error, and
	// ends the turn.
	ToolHoldTerminate
)

// ResolveHeldToolCall decides the fate of an assembled, held client-bound tool
// call: it applies the byte/time caps, the gate's fail mode, and the gate
// verdict. A cap breach or evaluation error honors the gate's fail mode
// (fail-open releases unevaluated; fail-closed terminates). A BLOCK verdict
// terminates. Otherwise the call is released.
func ResolveHeldToolCall(ctx context.Context, gate ToolGate, call ToolCall, heldBytes int, holdStart time.Time) (ToolHoldOutcome, ToolGateDecision) {
	if HoldOverflowed(heldBytes, holdStart) {
		if gate.FailOpen() {
			return ToolHoldRelease, ToolGateDecision{}
		}
		return ToolHoldTerminate, ToolGateDecision{Block: true, Reason: "tool call exceeded hold limit"}
	}
	dec, err := gate.EvaluateToolCall(ctx, call)
	if err != nil {
		if gate.FailOpen() {
			return ToolHoldRelease, ToolGateDecision{}
		}
		return ToolHoldTerminate, ToolGateDecision{Block: true, Reason: "tool gate evaluation failed"}
	}
	if dec.Block {
		return ToolHoldTerminate, dec
	}
	return ToolHoldRelease, dec
}

// HoldOverflowed reports whether a held tool block has exceeded the byte or time
// cap. holdStart is the zero value before any bytes are held.
func HoldOverflowed(heldBytes int, holdStart time.Time) bool {
	if heldBytes > HoldMaxBytes {
		return true
	}
	return !holdStart.IsZero() && time.Since(holdStart) > HoldDeadline
}

type toolGateContextKey struct{}

// WithToolGate returns a context carrying gate. A nil gate is a no-op (the hook
// is absent), which [ToolGateFromContext] surfaces as nil.
func WithToolGate(ctx context.Context, gate ToolGate) context.Context {
	if gate == nil {
		return ctx
	}
	return context.WithValue(ctx, toolGateContextKey{}, gate)
}

// ToolGateFromContext returns the [ToolGate] carried by ctx, or nil when no
// pre-tool pipeline is configured for the request's provider.
func ToolGateFromContext(ctx context.Context) ToolGate {
	g, ok := ctx.Value(toolGateContextKey{}).(ToolGate)
	if !ok {
		return nil
	}
	return g
}
