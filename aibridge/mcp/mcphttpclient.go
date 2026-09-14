package mcp

import (
	"flag"
	"net/http"
)

// withHeaders shallow-copies base so client-level settings such as
// Timeout and Jar survive the transport wrap.
func withHeaders(base *http.Client, headers map[string]string) *http.Client {
	client := &http.Client{}
	if base != nil {
		clone := *base
		client = &clone
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	if len(headers) > 0 {
		client.Transport = &headerRoundTripper{
			base:    client.Transport,
			headers: headers,
		}
	}
	return client
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
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
