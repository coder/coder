package chattest

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
)

// RecordedRequest captures metadata from a single request that passed
// through the mock transport factory. Fields not already available on
// [http.Request] are included here; tests can access headers and path
// via [RecordedRequest.Request].
type RecordedRequest struct {
	// Request is a clone of the original [http.Request].
	Request *http.Request
	// ProviderName is the AI provider instance name passed to
	// [TransportFactory.TransportFor].
	ProviderName string
	// Source is the aibridge source passed to TransportFor.
	Source aibridge.Source
	// APIKeyID is the delegated API key ID attached to request ctx.
	APIKeyID string
}

// MockAIBridgeTransportOption configures a [MockAIBridgeTransport].
type MockAIBridgeTransportOption func(*MockAIBridgeTransport)

// WithPreservePath disables the default "/v1" path stripping so the
// target server receives the full original request path.
func WithPreservePath() MockAIBridgeTransportOption {
	return func(f *MockAIBridgeTransport) { f.preservePath = true }
}

// MockAIBridgeTransport is a test [aibridge.TransportFactory] that
// redirects requests to a target URL (typically a [chattest.NewOpenAI]
// or [chattest.NewAnthropic] server) and records each request for
// later inspection.
//
// By default it strips the leading "/v1" path segment before
// forwarding, matching how the real AI Gateway transport rewrites
// upstream-shaped requests. Pass [WithPreservePath] when the target
// server expects the full original path.
type MockAIBridgeTransport struct {
	target       *url.URL
	transport    http.RoundTripper
	preservePath bool
	mu           sync.Mutex
	requests     []RecordedRequest
}

// NewMockAIBridgeTransport creates a [MockAIBridgeTransport] that
// forwards to targetBaseURL.
func NewMockAIBridgeTransport(t testing.TB, targetBaseURL string, opts ...MockAIBridgeTransportOption) *MockAIBridgeTransport {
	t.Helper()
	target, err := url.Parse(targetBaseURL)
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}
	f := &MockAIBridgeTransport{target: target, transport: http.DefaultTransport}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// TransportFor implements [aibridge.TransportFactory].
func (f *MockAIBridgeTransport) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	if len(providerName) == 0 {
		return nil, xerrors.New("provider name is required")
	}
	return mockRoundTripper{factory: f, providerName: providerName, source: source}, nil
}

// RequestsSnapshot returns a copy of all recorded requests.
func (f *MockAIBridgeTransport) RequestsSnapshot() []RecordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

type mockRoundTripper struct {
	factory      *MockAIBridgeTransport
	providerName string
	source       aibridge.Source
}

func (rt mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Mirror the real aibridged transport: a delegated API key must be
	// on the context, otherwise aibridged has no identity to act under.
	apiKeyID, ok := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
	if !ok {
		return nil, xerrors.New("mock aibridged transport requires WithDelegatedAPIKeyID on the request context")
	}
	rt.factory.mu.Lock()
	rt.factory.requests = append(rt.factory.requests, RecordedRequest{
		Request:      req.Clone(req.Context()),
		ProviderName: rt.providerName,
		Source:       rt.source,
		APIKeyID:     apiKeyID,
	})
	rt.factory.mu.Unlock()

	targetURL := *rt.factory.target
	if rt.factory.preservePath {
		targetURL.Path = req.URL.Path
	} else {
		targetURL.Path = strings.TrimPrefix(req.URL.Path, "/v1")
		if targetURL.Path == "" {
			targetURL.Path = "/"
		}
	}
	targetURL.RawQuery = req.URL.RawQuery

	cloned := req.Clone(req.Context())
	cloned.URL = &targetURL
	cloned.Host = rt.factory.target.Host
	return rt.factory.transport.RoundTrip(cloned)
}
