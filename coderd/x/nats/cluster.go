package nats

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

// ErrNoEmbeddedServer is returned by RefreshPeers when the Pubsub was
// constructed via NewFromConn and therefore does not own an embedded
// NATS server whose route configuration can be reloaded.
var ErrNoEmbeddedServer = xerrors.New("nats pubsub has no embedded server")

// routeAuthUsername is the synthetic username carried in route CONNECT
// userinfo. The NATS route authenticator requires a nonempty username
// when Username/Password auth is configured; the actual secret is in
// the password field (ClusterToken).
const routeAuthUsername = "coder"

// Peer describes a single NATS cluster peer for startup route discovery.
type Peer struct {
	// Name is optional and used only for logs and metrics.
	Name string

	// RouteURL is the NATS route URL for this peer, without credentials.
	// Examples: nats://10.0.0.12:6222 or tls://nats-1.internal:6222.
	RouteURL string
}

// PeerProvider returns the set of cluster peers used to seed route
// discovery. New calls Peers once during startup. RefreshPeers may call
// Peers again from any goroutine; implementations must be safe for
// repeated calls and should return a fresh slice each time so callers
// can mutate it without affecting the provider's internal state.
type PeerProvider interface {
	Peers(ctx context.Context) ([]Peer, error)
}

// StaticPeerProvider is a PeerProvider backed by a fixed slice of peers.
type StaticPeerProvider []Peer

// Peers returns the static set of peers.
func (s StaticPeerProvider) Peers(context.Context) ([]Peer, error) {
	return []Peer(s), nil
}

// normalizePeers trims and validates RouteURL on each peer, preserving
// order and Name. It rejects empty URLs and any scheme other than
// "nats" or "tls".
func normalizePeers(peers []Peer) ([]Peer, error) {
	if len(peers) == 0 {
		return nil, nil
	}
	out := make([]Peer, 0, len(peers))
	for i, p := range peers {
		raw := strings.TrimSpace(p.RouteURL)
		if raw == "" {
			return nil, xerrors.Errorf("peer %d: empty RouteURL", i)
		}
		u, err := url.Parse(raw)
		if err != nil {
			return nil, xerrors.Errorf("peer %d: parse %q: %w", i, raw, err)
		}
		switch u.Scheme {
		case "nats", "tls":
		default:
			return nil, xerrors.Errorf("peer %d: unsupported scheme %q (want nats or tls)", i, u.Scheme)
		}
		out = append(out, Peer{Name: p.Name, RouteURL: raw})
	}
	return out, nil
}

// routeURLs converts already-normalized peers into *url.URL values and
// injects route auth userinfo. Route authentication is performed via
// CONNECT userinfo: NATS requires a nonempty username, so we always
// set the username to routeAuthUsername and place the shared cluster
// token in the password field when token is non-empty.
func routeURLs(peers []Peer, token string) ([]*url.URL, error) {
	if len(peers) == 0 {
		return nil, nil
	}
	out := make([]*url.URL, 0, len(peers))
	for i, p := range peers {
		u, err := url.Parse(p.RouteURL)
		if err != nil {
			return nil, xerrors.Errorf("peer %d: parse %q: %w", i, p.RouteURL, err)
		}
		if token != "" {
			u.User = url.UserPassword(routeAuthUsername, token)
		}
		out = append(out, u)
	}
	return out, nil
}

// sortRouteURLs returns a new slice containing the same *url.URL pointers
// as in, sorted by URL.String(). The input slice is not mutated.
func sortRouteURLs(in []*url.URL) []*url.URL {
	out := make([]*url.URL, len(in))
	copy(out, in)
	sort.Slice(out, func(i, j int) bool {
		var a, b string
		if out[i] != nil {
			a = out[i].String()
		}
		if out[j] != nil {
			b = out[j].String()
		}
		return a < b
	})
	return out
}

// routeURLsEqual reports whether a and b contain the same set of route
// URLs in the same order, comparing by URL.String().
func routeURLsEqual(a, b []*url.URL) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		var as, bs string
		if a[i] != nil {
			as = a[i].String()
		}
		if b[i] != nil {
			bs = b[i].String()
		}
		if as != bs {
			return false
		}
	}
	return true
}

// cloneRouteURLs returns a deep copy of in. Each *url.URL is shallow
// copied (URL has no pointer fields except User which is not mutated).
func cloneRouteURLs(in []*url.URL) []*url.URL {
	if in == nil {
		return nil
	}
	out := make([]*url.URL, len(in))
	for i, u := range in {
		if u == nil {
			continue
		}
		cp := *u
		out[i] = &cp
	}
	return out
}
