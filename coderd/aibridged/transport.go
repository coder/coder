package aibridged

import (
	"net/http"
	"net/url"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/util/xhttp"
)

// NewTransportFactory returns an [aibridge.TransportFactory] whose RoundTripper
// dispatches requests to handler in-process through [xhttp.HandlerTransport],
// so SSE/NDJSON/chunked responses propagate token-by-token just as they would
// over the wire.
//
// handler is typically the aibridged HTTP entrypoint registered via
// [API.RegisterInMemoryAIBridgedHTTPHandler].
func NewTransportFactory(handler http.Handler) aibridge.TransportFactory {
	return &transportFactory{handler: handler}
}

type transportFactory struct {
	handler http.Handler
}

// TransportFor returns an in-process [http.RoundTripper] that dispatches
// requests through the aibridged handler. The provider name is the routing
// key the daemon mounts on; the round-tripper rewrites each request's URL
// path to "/api/v2/ai-gateway/<providerName>/..." before dispatching so
// callers can build upstream-shaped requests and stay agnostic of the
// daemon's mount layout. The source is attached to the request context for
// downstream logging; routing does not depend on it.
func (f *transportFactory) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	if f.handler == nil {
		return nil, xerrors.New("aibridged handler not registered")
	}
	if providerName == "" {
		return nil, xerrors.New("provider name is required")
	}
	return &inMemoryRoundTripper{
		next:         xhttp.HandlerTransport(f.handler),
		providerName: providerName,
		source:       source,
	}, nil
}

// inMemoryRoundTripper adapts requests to the aibridged mount layout and
// serves them in process through next.
type inMemoryRoundTripper struct {
	next         http.RoundTripper
	providerName string
	source       aibridge.Source
}

func (t *inMemoryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// The in-process transport requires the caller to have placed the
	// delegated API key ID on the context. Without it, aibridged has no
	// identity to act under. Fail fast at the transport boundary so the
	// handler can assume the invariant.
	if _, ok := aibridge.DelegatedAPIKeyIDFromContext(req.Context()); !ok {
		return nil, xerrors.New("aibridged in-memory transport requires WithDelegatedAPIKeyID on the request context")
	}

	// Adapt the caller's upstream-shaped URL to the daemon's mount layout:
	// "/api/v2/ai-gateway/<providerName>/<original-path>". Done here so
	// callers do not need to encode the mount prefix or the provider
	// routing key into the requests they hand to the transport.
	newPath, err := url.JoinPath(aibridge.AIGatewayRootPath, t.providerName, req.URL.Path)
	if err != nil {
		return nil, xerrors.Errorf("rewrite request URL for provider %q: %w", t.providerName, err)
	}
	// The Source is attached to the served context so downstream handlers
	// can log the call site.
	req = req.Clone(aibridge.WithSource(req.Context(), t.source))
	req.URL.Path = newPath
	return t.next.RoundTrip(req)
}
