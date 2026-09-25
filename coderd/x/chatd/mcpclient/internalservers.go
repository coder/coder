package mcpclient

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/xhttp"
)

// InternalScheme is the URL scheme of MCP servers that coderd serves in
// process. Only ConnectInline can reach them. The public API accepts only
// http and https URLs and no X-Coder-* headers on inline MCP servers, so
// only coderd code can attach an internal server, and the X-Coder-*
// headers it receives come from chatd.
const InternalScheme = "coder-internal"

var internalHostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// InternalServers maps hosts to in-process MCP handlers. It is fixed when
// it is built, so it is safe for concurrent use. The zero value has no
// servers.
type InternalServers struct {
	transports map[string]http.RoundTripper
}

// NewInternalServers serves each handler at InternalURL(host). It panics
// when a host is not a lowercase DNS label, as http.ServeMux.Handle panics
// on an invalid pattern.
func NewInternalServers(handlers map[string]http.Handler) InternalServers {
	transports := make(map[string]http.RoundTripper, len(handlers))
	for host, h := range handlers {
		if !internalHostPattern.MatchString(host) {
			panic(fmt.Sprintf("mcpclient: invalid internal MCP server host %q", host))
		}
		transports[host] = xhttp.HandlerTransport(h)
	}
	return InternalServers{transports: transports}
}

// InternalURL returns the URL at which chats reach the internal MCP
// server for host.
func InternalURL(host string) string {
	return InternalScheme + "://" + host
}

// Empty reports whether s has no servers.
func (s InternalServers) Empty() bool {
	return len(s.transports) == 0
}

// IsInternalURL reports whether rawURL uses InternalScheme.
func IsInternalURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && u.Scheme == InternalScheme
}

// internalRouter sends coder-internal requests to the registered handlers
// and all other requests to next.
type internalRouter struct {
	servers InternalServers
	next    http.RoundTripper
}

func (r *internalRouter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != InternalScheme {
		return r.next.RoundTrip(req)
	}
	rt, ok := r.servers.transports[req.URL.Host]
	if !ok {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, xerrors.Errorf("no internal MCP server registered for host %q", req.URL.Host)
	}
	return rt.RoundTrip(req)
}
