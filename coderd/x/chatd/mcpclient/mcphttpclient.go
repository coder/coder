package mcpclient

import (
	"flag"
	"net/http"
	"net/url"
	"strings"
)

type headerScope uint8

const (
	headerScopeAnyOrigin headerScope = iota
	headerScopeEndpointOrigin
)

func httpClientWithHeaders(
	baseClient *http.Client,
	headers map[string]string,
	endpoint string,
	scope headerScope,
) *http.Client {
	if baseClient == nil {
		baseClient = mcpHTTPClient()
	}
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	client := *baseClient
	baseTransport := client.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	if len(headers) == 0 {
		client.Transport = baseTransport
		return &client
	}
	var endpointURL *url.URL
	if scope == headerScopeEndpointOrigin {
		endpointURL, _ = url.Parse(endpoint)
	}
	client.Transport = &headerRoundTripper{
		base:        baseTransport,
		headers:     headers,
		endpointURL: endpointURL,
	}
	return &client
}

type headerRoundTripper struct {
	base        http.RoundTripper
	headers     map[string]string
	endpointURL *url.URL
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if h.endpointURL != nil &&
		(!strings.EqualFold(req.URL.Scheme, h.endpointURL.Scheme) ||
			!strings.EqualFold(req.URL.Host, h.endpointURL.Host)) {
		return h.base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	return h.base.RoundTrip(clone)
}

// mcpHTTPClient returns an isolated *http.Client when running
// inside tests, or nil for production. During tests,
// httptest.Server.Close() calls
// http.DefaultTransport.CloseIdleConnections(), which disrupts
// any MCP client sharing that transport. When DefaultTransport
// is a *http.Transport it is cloned; otherwise a minimal
// transport with ProxyFromEnvironment is created as a fallback.
func mcpHTTPClient() *http.Client {
	if flag.Lookup("test.v") == nil {
		return nil
	}
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		return &http.Client{Transport: dt.Clone()}
	}
	return &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}}
}
