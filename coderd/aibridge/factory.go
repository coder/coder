package aibridge

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Attribution carries trusted per-request AI Gateway attribution. The zero
// value means no workspace is bound.
type Attribution struct {
	// WorkspaceID is the workspace bound to the chat, or uuid.Nil when no
	// workspace is bound.
	WorkspaceID uuid.UUID
}

type attributionCtxKey struct{}

// WithAttribution returns a copy of ctx carrying the trusted Attribution.
// Attribution is set by authentication or delegated in-process code, never by
// client-provided HTTP headers.
func WithAttribution(ctx context.Context, attr Attribution) context.Context {
	return context.WithValue(ctx, attributionCtxKey{}, attr)
}

// AttributionFromContext returns the Attribution attached by [WithAttribution]
// and whether a non-zero WorkspaceID was present.
func AttributionFromContext(ctx context.Context) (Attribution, bool) {
	attr, ok := ctx.Value(attributionCtxKey{}).(Attribution)
	return attr, ok && attr.WorkspaceID != uuid.Nil
}

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

type delegatedRequest struct {
	apiKeyID    string
	attribution Attribution
}

type delegatedRequestCtxKey struct{}

// WithDelegatedAPIKeyID returns a copy of ctx carrying an API key ID on whose
// behalf the request is being made. The in-process aibridge transport requires
// this on every RoundTrip and rejects calls whose context lacks it.
//
// The caller is responsible for having established that the user owning this
// key authorized the request: aibridged validates only that the key exists,
// has not expired, and belongs to a non-deleted, non-system user. It does not
// verify the key secret, because the caller never has it.
func WithDelegatedAPIKeyID(ctx context.Context, id string) context.Context {
	return WithDelegatedRequest(ctx, id, Attribution{})
}

// WithDelegatedRequest returns a copy of ctx carrying a delegated API key ID
// and its trusted request attribution as one value.
func WithDelegatedRequest(ctx context.Context, id string, attr Attribution) context.Context {
	return context.WithValue(ctx, delegatedRequestCtxKey{}, delegatedRequest{
		apiKeyID:    id,
		attribution: attr,
	})
}

// DelegatedAPIKeyIDFromContext returns the API key ID attached by
// [WithDelegatedAPIKeyID] and whether a non-empty value was set.
func DelegatedAPIKeyIDFromContext(ctx context.Context) (string, bool) {
	req, ok := ctx.Value(delegatedRequestCtxKey{}).(delegatedRequest)
	return req.apiKeyID, ok && req.apiKeyID != ""
}

// DelegatedAttributionFromContext returns the trusted attribution attached to
// the delegated request. Its boolean reports whether delegated authentication,
// rather than attribution itself, was present.
func DelegatedAttributionFromContext(ctx context.Context) (Attribution, bool) {
	req, ok := ctx.Value(delegatedRequestCtxKey{}).(delegatedRequest)
	return req.attribution, ok && req.apiKeyID != ""
}

// TransportFactory returns an [http.RoundTripper] that dispatches an aibridge
// request in-process for a given provider instance name.
//
// Implementations live in coderd/aibridged. coderd registers an in-process
// factory on coderd.API.AIBridgeTransportFactory at startup so callers route
// traffic through the daemon without going through the gated HTTP route.
//
// The returned RoundTripper is responsible for adapting the caller's request
// to the aibridge daemon's mount path: callers hand it an upstream-shaped
// request and the transport rewrites URL.Path to "/api/v2/ai-gateway/<name>/..."
// before dispatching. Routing keys on the provider's instance name so callers
// can use the same string the proxy daemon and the bridge mount use.
//
// Source is informational: implementations must not gate on it. It is attached
// to the request context so handlers can include it in logs and metrics.
type TransportFactory interface {
	TransportFor(providerName string, source Source) (http.RoundTripper, error)
}
