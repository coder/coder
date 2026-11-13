package agentmcp

import (
	"flag"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/net/idna"
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

// originOf converts a non-ASCII hostname to its IDNA ASCII form, the
// name net/http dials. When that conversion fails it returns an error
// along with the origin built from the lowercased hostname.
func originOf(u *url.URL) (urlOrigin, error) {
	host := u.Hostname()
	var hostErr error
	if !isASCII(host) {
		if ascii, err := idna.Lookup.ToASCII(host); err == nil {
			host = ascii
		} else {
			hostErr = xerrors.New("hostname is not a valid IDNA name")
		}
	}
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
	return urlOrigin{scheme: scheme, host: strings.ToLower(host), port: port}, hostErr
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func (o urlOrigin) String() string {
	return o.scheme + "://" + net.JoinHostPort(o.host, o.port)
}

// sameOriginHeaderClient returns a client that adds headers only to
// requests matching serverURL's scheme, host, and port. It never
// replaces a caller-set header or adds a header net/http manages. The
// first request to another origin, or with a hostname that is not valid
// IDNA, is logged at warn level and sent without the headers.
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
	origin, err := originOf(u)
	if err != nil {
		return nil, xerrors.New("invalid server url")
	}
	return &http.Client{Transport: &sameOriginHeaderRoundTripper{
		base:    base,
		logger:  logger,
		origin:  origin,
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
	if reqOrigin, err := originOf(req.URL); err != nil || reqOrigin != h.origin {
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
