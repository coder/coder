package mcpclient

import (
	"flag"
	"net"
	"net/http"
	"time"
)

// dialTimeout bounds TCP connection establishment to an MCP
// server. Without it, a server that silently drops SYNs (for
// example an edge mitigation black-holing this deployment's
// egress IPs) blocks the dial until the kernel gives up
// retransmitting, roughly two minutes on Linux.
const dialTimeout = 5 * time.Second

// responseHeaderTimeout bounds how long a server may take to send
// response headers once a request is written. It matches
// toolCallTimeout rather than connectTimeout because the same
// client serves tool-call POSTs, and a JSON-response MCP server
// sends no headers until the tool finishes, so a lower value would
// kill legitimate slow tools that fit the tool-call budget.
// Long-lived SSE streams are unaffected; only their headers must
// arrive within this window. http.Client.Timeout is deliberately
// unset because it would cap the stream body too.
const responseHeaderTimeout = toolCallTimeout

// mcpSharedTransport is the transport for all production MCP
// connections. MCP traffic must not use http.DefaultTransport
// directly: the default has no dial or response-header bounds, so
// a black-holed server would hold connections for minutes.
var mcpSharedTransport = newMCPTransport()

// newMCPTransport clones http.DefaultTransport when possible,
// preserving proxy and connection-pool settings, and tightens its
// failure timeouts so an unresponsive MCP server fails in seconds.
func newMCPTransport() *http.Transport {
	tr, ok := http.DefaultTransport.(*http.Transport)
	if ok {
		tr = tr.Clone()
	} else {
		tr = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	tr.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	tr.ResponseHeaderTimeout = responseHeaderTimeout
	return tr
}

func httpClientWithHeaders(headers map[string]string) *http.Client {
	var base http.RoundTripper = mcpSharedTransport
	if isolated := mcpHTTPClient(); isolated != nil {
		base = isolated.Transport
	}
	if len(headers) == 0 {
		return &http.Client{Transport: base}
	}
	return &http.Client{Transport: &headerRoundTripper{
		base:    base,
		headers: headers,
	}}
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
// inside tests, or nil for production. During tests each client
// gets a fresh transport so closed httptest servers cannot leave
// stale pooled connections behind for later tests that reuse the
// same address.
func mcpHTTPClient() *http.Client {
	if flag.Lookup("test.v") == nil {
		return nil
	}
	return &http.Client{Transport: newMCPTransport()}
}
