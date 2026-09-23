package mcpclient

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sync"

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

// InternalServers maps hosts to in-process MCP handlers. It is safe for
// concurrent use. A nil *InternalServers has no servers.
type InternalServers struct {
	mu      sync.RWMutex
	servers map[string]http.RoundTripper
}

// NewInternalServers returns an empty registry.
func NewInternalServers() *InternalServers {
	return &InternalServers{servers: make(map[string]http.RoundTripper)}
}

// Register serves h at coder-internal://<host> and returns that URL. It
// panics when host is not a lowercase DNS label or is already registered,
// as http.ServeMux.Handle does.
func (s *InternalServers) Register(host string, h http.Handler) string {
	if !internalHostPattern.MatchString(host) {
		panic(fmt.Sprintf("mcpclient: invalid internal MCP server host %q", host))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.servers[host]; ok {
		panic(fmt.Sprintf("mcpclient: internal MCP server host %q is already registered", host))
	}
	s.servers[host] = xhttp.HandlerTransport(h)
	return InternalScheme + "://" + host
}

// Empty reports whether no server is registered.
func (s *InternalServers) Empty() bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.servers) == 0
}

func (s *InternalServers) transport(host string) (http.RoundTripper, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rt, ok := s.servers[host]
	return rt, ok
}

// IsInternalURL reports whether rawURL uses InternalScheme.
func IsInternalURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && u.Scheme == InternalScheme
}

// internalRouter sends coder-internal requests to the registered handlers
// and all other requests to next.
type internalRouter struct {
	servers *InternalServers
	next    http.RoundTripper
}

func (r *internalRouter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != InternalScheme {
		return r.next.RoundTrip(req)
	}
	rt, ok := r.servers.transport(req.URL.Host)
	if !ok {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, xerrors.Errorf("no internal MCP server registered for host %q", req.URL.Host)
	}
	return rt.RoundTrip(req)
}
