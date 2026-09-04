package mcpclient

import (
	"net/http"

	"github.com/coder/safedial"
)

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

// NewHTTPClient builds the SSRF-guarded, timeout-bounded HTTP
// client for MCP traffic. MCP destinations are configured by org
// admins rather than the deployment operator, so every connection
// goes through safedial's address policy. The guarded transport
// keeps the base client's settings but always bounds response
// headers: without a bound, a server that accepts a request and
// never answers holds the connection until the enclosing context
// expires. Dial establishment is bounded by the guarded dialer.
func NewHTTPClient(base *http.Client, opts ...safedial.Option) *http.Client {
	if base == nil {
		// Not safedial's nil default: that carries a whole-client
		// timeout, which would cap SSE stream bodies that must stay
		// open for the life of a session.
		base = &http.Client{}
	}
	client := safedial.NewHTTPClient(base, opts...)
	if tr, ok := client.Transport.(*http.Transport); ok {
		// The guarded transport is a private clone, so setting the
		// bound here cannot race with or mutate the caller's base.
		tr.ResponseHeaderTimeout = responseHeaderTimeout
	}
	return client
}

func httpClientWithHeaders(base *http.Client, headers map[string]string) *http.Client {
	if base == nil || base.Transport == nil {
		base = NewHTTPClient(base)
	}
	client := *base
	client.CheckRedirect = safedial.CheckSameOriginRedirect
	if len(headers) == 0 {
		return &client
	}
	transport := base.Transport
	client.Transport = &headerRoundTripper{
		base:    transport,
		headers: headers,
	}
	return &client
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
