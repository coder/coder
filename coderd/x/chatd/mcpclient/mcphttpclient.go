package mcpclient

import (
	"flag"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/xerrors"
)

type headerScope uint8

const (
	headerScopeAnyOrigin headerScope = iota
	headerScopeEndpointOrigin
)

const maxPrivateMCPHTTPResponseBytes = 1 << 20

var errPrivateMCPResponseTooLarge = xerrors.New("private MCP response body exceeds maximum size")

func privateMCPHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		panic("mcpclient: privateMCPHTTPClient called with nil base client")
	}
	client := *base
	client.Timeout = 0
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = &maxResponseBodyRoundTripper{
		base:     transport,
		maxBytes: maxPrivateMCPHTTPResponseBytes,
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
		return nil, errPrivateMCPResponseTooLarge
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
	maxRead := r.remaining + 1
	if int64(len(p)) > maxRead {
		p = p[:maxRead]
	}
	n, err := r.body.Read(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
		r.remaining = 0
		return n, errPrivateMCPResponseTooLarge
	}
	r.remaining -= int64(n)
	return n, err
}

func (r *maxResponseReadCloser) Close() error {
	return r.body.Close()
}

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
