package aibridge

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Source identifies the call site that asked aibridge for a transport. It is
// attached to the request context so downstream handlers and logs can attribute
// traffic without changing behavior based on the value.
type Source string

// SourceAgents is chatd traffic originating from a Coder agent.
const SourceAgents Source = "agents"

type sourceCtxKey struct{}

// WithSource returns a copy of ctx carrying the given Source. Use this on the
// request context before invoking a downstream handler so [SourceFromContext]
// can recover it for logging.
func WithSource(ctx context.Context, src Source) context.Context {
	return context.WithValue(ctx, sourceCtxKey{}, src)
}

// SourceFromContext returns the Source attached by [WithSource], or the empty
// string when no Source is set.
func SourceFromContext(ctx context.Context) Source {
	src, _ := ctx.Value(sourceCtxKey{}).(Source)
	return src
}

type delegatedAPIKeyIDCtxKey struct{}

// WithDelegatedAPIKeyID returns a copy of ctx carrying an API key ID on whose
// behalf the request is being made. The in-process aibridge transport requires
// this on every RoundTrip and rejects calls whose context lacks it.
//
// The caller is responsible for having established that the user owning this
// key authorized the request: aibridged validates only that the key exists,
// has not expired, and has not been revoked. It does not verify the key
// secret, because the caller never has it.
func WithDelegatedAPIKeyID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, delegatedAPIKeyIDCtxKey{}, id)
}

// DelegatedAPIKeyIDFromContext returns the API key ID attached by
// [WithDelegatedAPIKeyID] and whether one was set.
func DelegatedAPIKeyIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(delegatedAPIKeyIDCtxKey{}).(string)
	return id, ok && id != ""
}

// TransportFactory returns an [http.RoundTripper] that dispatches an aibridge
// request in-process for a given ai_providers row.
//
// Implementations live in coderd/aibridged. coderd registers an in-process
// factory on coderd.API.AIBridgeTransportFactory at startup so callers route
// traffic through the daemon without going through the gated HTTP route.
//
// Source is informational: implementations must not gate on it. It is attached
// to the request context so handlers can include it in logs and metrics.
type TransportFactory interface {
	TransportFor(providerID uuid.UUID, source Source) (http.RoundTripper, error)
}
