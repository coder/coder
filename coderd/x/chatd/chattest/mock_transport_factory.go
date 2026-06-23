package chattest

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/coder/coder/v2/coderd/aibridge"
)

// RecordedRequest captures metadata from a single request that passed
// through the mock transport factory. Tests use it to assert that AI
// Gateway routing attributed requests to the correct API key and
// provider.
type RecordedRequest struct {
	ProviderName  string
	Source        aibridge.Source
	APIKeyID      string
	Path          string
	Authorization string
	XAPIKey       string
	CoderToken    string
}

// MockTransportFactory is a test [aibridge.TransportFactory] that
// redirects requests to a target URL (typically a [chattest.NewOpenAI]
// or [chattest.NewAnthropic] server) and records each request for
// later inspection.
//
// By default it strips the leading "/v1" path segment before
// forwarding, matching how the real AI Gateway transport rewrites
// upstream-shaped requests. Use [NewMockTransportFactoryPreservePath]
// when the target server expects the full original path.
type MockTransportFactory struct {
	target       *url.URL
	transport    http.RoundTripper
	preservePath bool
	mu           sync.Mutex
	requests     []RecordedRequest
}

// NewMockTransportFactory creates a [MockTransportFactory] that
// forwards to targetBaseURL, stripping the leading "/v1" path segment.
func NewMockTransportFactory(t testing.TB, targetBaseURL string) *MockTransportFactory {
	t.Helper()
	target, err := url.Parse(targetBaseURL)
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}
	return &MockTransportFactory{target: target, transport: http.DefaultTransport}
}

// NewMockTransportFactoryPreservePath creates a [MockTransportFactory]
// that forwards to targetBaseURL without stripping the request path.
func NewMockTransportFactoryPreservePath(t testing.TB, targetBaseURL string) *MockTransportFactory {
	t.Helper()
	target, err := url.Parse(targetBaseURL)
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}
	return &MockTransportFactory{target: target, transport: http.DefaultTransport, preservePath: true}
}

// TransportFor implements [aibridge.TransportFactory].
func (f *MockTransportFactory) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	return mockRoundTripper{factory: f, providerName: providerName, source: source}, nil
}

// RequestsSnapshot returns a copy of all recorded requests.
func (f *MockTransportFactory) RequestsSnapshot() []RecordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RecordedRequest(nil), f.requests...)
}

type mockRoundTripper struct {
	factory      *MockTransportFactory
	providerName string
	source       aibridge.Source
}

func (rt mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	apiKeyID, _ := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
	rt.factory.mu.Lock()
	rt.factory.requests = append(rt.factory.requests, RecordedRequest{
		ProviderName:  rt.providerName,
		Source:        rt.source,
		APIKeyID:      apiKeyID,
		Path:          req.URL.Path,
		Authorization: req.Header.Get("Authorization"),
		XAPIKey:       req.Header.Get("X-Api-Key"),
		CoderToken:    req.Header.Get(aibridge.HeaderCoderToken),
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
