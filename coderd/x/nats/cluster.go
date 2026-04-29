package nats

import "context"

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
	// TODO: validate and normalize peers when implementation lands.
	return []Peer(s), nil
}
