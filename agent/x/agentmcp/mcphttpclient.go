package agentmcp

import (
	"flag"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// httpClientWithHeaders returns a client that sets headers on every
// request, whatever its origin, replacing any value the caller set.
func httpClientWithHeaders(headers map[string]string) *http.Client {
	base := mcpBaseTransport()
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

// urlOrigin is a lowercased URL scheme, hostname, and port, with the
// port defaulted from the scheme when absent.
type urlOrigin struct {
	scheme string
	host   string
	port   string
}

func originOf(u *url.URL) urlOrigin {
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
	return urlOrigin{scheme: scheme, host: strings.ToLower(u.Hostname()), port: port}
}

func (o urlOrigin) String() string {
	return o.scheme + "://" + net.JoinHostPort(o.host, o.port)
}

// sameOriginHeaderClient returns a client that adds headers only to
// requests matching serverURL's scheme, host, and port. It never
// replaces a caller-set header or adds a header net/http manages. The
// first request to another origin is logged at warn level.
func sameOriginHeaderClient(logger slog.Logger, serverURL string, headers map[string]string) (*http.Client, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		// The parse error quotes parts of the URL, which can carry
		// credentials.
		return nil, xerrors.New("invalid server url")
	}

	base := mcpBaseTransport()
	if len(headers) == 0 {
		return &http.Client{Transport: base}, nil
	}
	return &http.Client{Transport: &sameOriginHeaderRoundTripper{
		base:    base,
		logger:  logger,
		origin:  originOf(u),
		headers: headers,
	}}, nil
}

type sameOriginHeaderRoundTripper struct {
	base     http.RoundTripper
	logger   slog.Logger
	origin   urlOrigin
	headers  map[string]string
	warnOnce sync.Once
}

func (h *sameOriginHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if reqOrigin := originOf(req.URL); reqOrigin != h.origin {
		h.warnOnce.Do(func() {
			// Only origins are logged; paths and queries can carry
			// credentials.
			h.logger.Warn(req.Context(), "withholding configured MCP headers from a request to another origin",
				slog.F("server_origin", h.origin.String()),
				slog.F("request_origin", reqOrigin.String()))
		})
		return h.base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		if transportManagedHeaders[http.CanonicalHeaderKey(k)] || len(clone.Header.Values(k)) > 0 {
			continue
		}
		clone.Header.Set(k, v)
	}
	return h.base.RoundTrip(clone)
}

// transportManagedHeaders are set by net/http itself. A configured
// Accept-Encoding, for example, stops the transport from decompressing
// gzip responses.
var transportManagedHeaders = map[string]bool{
	"Accept-Encoding":   true,
	"Connection":        true,
	"Keep-Alive":        true,
	"Proxy-Connection":  true,
	"Te":                true,
	"Trailer":           true,
	"Transfer-Encoding": true,
	"Upgrade":           true,
}

func mcpBaseTransport() http.RoundTripper {
	if isolated := mcpHTTPClient(); isolated != nil {
		return isolated.Transport
	}
	return http.DefaultTransport
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
