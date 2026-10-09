package agentmcp

import (
	"flag"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/xerrors"
)

// requestOrigin is a lowercased URL scheme, hostname, and port, with the
// port defaulted from the scheme when absent.
type requestOrigin struct {
	scheme string
	host   string
	port   string
}

func originOf(u *url.URL) requestOrigin {
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return requestOrigin{scheme: scheme, host: strings.ToLower(u.Hostname()), port: port}
}

func (o requestOrigin) String() string {
	return o.scheme + "://" + net.JoinHostPort(o.host, o.port)
}

// httpClientWithHeaders returns a client that adds headers only to
// requests for the scheme, host, and port of serverURL, and never
// replaces a header the caller already set. Requests to any other
// origin, such as a redirect target or an SSE message endpoint, are sent
// without the headers.
func httpClientWithHeaders(serverURL string, headers map[string]string) (*http.Client, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		// The parse error quotes parts of the URL, which can carry
		// credentials.
		return nil, xerrors.New("invalid server url")
	}

	base := http.DefaultTransport
	if isolated := mcpHTTPClient(); isolated != nil {
		base = isolated.Transport
	}
	if len(headers) == 0 {
		return &http.Client{Transport: base}, nil
	}
	return &http.Client{Transport: &headerRoundTripper{
		base:    base,
		origin:  originOf(u),
		headers: headers,
	}}, nil
}

type headerRoundTripper struct {
	base    http.RoundTripper
	origin  requestOrigin
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if originOf(req.URL) != h.origin {
		return h.base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		if len(clone.Header.Values(k)) > 0 {
			continue
		}
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
