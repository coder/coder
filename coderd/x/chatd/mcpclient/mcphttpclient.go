package mcpclient

import (
	"io"
	"net/http"

	"golang.org/x/xerrors"

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

// maxChatAttachedHTTPResponseBytes caps one HTTP response body from a
// chat-attached MCP server. Chat owners, not org admins, choose these
// servers, so a hostile server must not be able to exhaust memory with
// an unbounded tool list or tool result.
const maxChatAttachedHTTPResponseBytes = 1 << 20

var errChatAttachedResponseTooLarge = xerrors.New("chat-attached MCP response body exceeds maximum size")

// chatAttachedHTTPClient wraps base so every response body is capped at
// maxChatAttachedHTTPResponseBytes. A nil or transport-less base falls
// back to the default guarded client, matching httpClientWithHeaders.
func chatAttachedHTTPClient(base *http.Client) *http.Client {
	if base == nil || base.Transport == nil {
		base = NewHTTPClient(base)
	}
	client := *base
	client.Transport = &maxResponseBodyRoundTripper{
		base:     base.Transport,
		maxBytes: maxChatAttachedHTTPResponseBytes,
	}
	return &client
}

type maxResponseBodyRoundTripper struct {
	base     http.RoundTripper
	maxBytes int64
}

func (t *maxResponseBodyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.Body == nil {
		return resp, nil
	}
	if resp.ContentLength > t.maxBytes {
		_ = resp.Body.Close()
		return nil, errChatAttachedResponseTooLarge
	}
	resp.Body = &maxResponseReadCloser{
		body:      resp.Body,
		remaining: t.maxBytes,
	}
	return resp, nil
}

type maxResponseReadCloser struct {
	body      io.ReadCloser
	remaining int64
}

func (r *maxResponseReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return r.body.Read(p)
	}
	// Read one byte past the cap so an exactly-at-cap body is not
	// rejected while an over-cap body is detected on this read.
	maxRead := r.remaining + 1
	if int64(len(p)) > maxRead {
		p = p[:maxRead]
	}
	n, err := r.body.Read(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
		r.remaining = 0
		return n, errChatAttachedResponseTooLarge
	}
	r.remaining -= int64(n)
	return n, err
}

func (r *maxResponseReadCloser) Close() error {
	return r.body.Close()
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
