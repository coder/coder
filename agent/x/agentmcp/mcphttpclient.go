package agentmcp

import (
	"flag"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/xerrors"
)

// maxRedirects mirrors the net/http default redirect limit.
const maxRedirects = 10

// requestOrigin is the scheme and host (including port) that configured
// headers are bound to.
type requestOrigin struct {
	scheme string
	host   string
}

func originOf(u *url.URL) requestOrigin {
	return requestOrigin{scheme: strings.ToLower(u.Scheme), host: strings.ToLower(u.Host)}
}

// httpClientWithHeaders builds the HTTP client for an MCP server at
// serverURL. Configured headers are sent only on requests to the same
// scheme and host as serverURL and never replace a header the caller
// already set. Redirects that leave that origin, including any change
// from https to http, are refused.
func httpClientWithHeaders(serverURL string, headers map[string]string) (*http.Client, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, xerrors.Errorf("parse server url %q: %w", serverURL, err)
	}
	origin := originOf(u)

	base := http.DefaultTransport
	if isolated := mcpHTTPClient(); isolated != nil {
		base = isolated.Transport
	}
	if len(headers) > 0 {
		base = &headerRoundTripper{
			base:    base,
			origin:  origin,
			headers: headers,
		}
	}
	return &http.Client{
		Transport:     base,
		CheckRedirect: sameOriginRedirectPolicy(origin),
	}, nil
}

// sameOriginRedirectPolicy returns a CheckRedirect that refuses any
// redirect whose target is not on origin, and stops after maxRedirects
// hops like the default policy.
func sameOriginRedirectPolicy(origin requestOrigin) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return xerrors.Errorf("stopped after %d redirects", maxRedirects)
		}
		if target := originOf(req.URL); target != origin {
			return xerrors.Errorf("refusing redirect from %s://%s to %s://%s",
				origin.scheme, origin.host, target.scheme, target.host)
		}
		return nil
	}
}

type headerRoundTripper struct {
	base    http.RoundTripper
	origin  requestOrigin
	headers map[string]string
}

// RoundTrip adds the configured headers when the request targets the
// configured origin. Headers already present on the request are left
// untouched.
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
