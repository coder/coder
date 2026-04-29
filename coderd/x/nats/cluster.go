package nats

import (
	"context"
	"net/url"
	"strings"

	"golang.org/x/xerrors"
)

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

// PeerProvider returns the set of cluster peers to seed route discovery on
// startup. V1 only consults the provider once during New.
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
