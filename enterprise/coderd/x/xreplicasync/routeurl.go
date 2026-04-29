// Package xreplicasync adapts a replicasync.Manager-like replica source
// into a coderd/x/nats.PeerProvider, and drives RefreshPeers on the
// associated Pubsub whenever the replica set changes.
package xreplicasync

import (
	"fmt"
	"net/url"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// RouteURLFunc derives a NATS route URL for a single replica. Implementations
// must return an error when the replica does not carry the information
// required to build a route URL (for example, an empty hostname or relay
// address).
type RouteURLFunc func(database.Replica) (string, error)

// validateRouteURLConfig enforces the shared scheme and port rules used by
// every RouteURLFunc constructor in this package. Only the "nats" and "tls"
// schemes are accepted, matching the schemes that coderd/x/nats normalizes.
func validateRouteURLConfig(scheme string, port int) error {
	if scheme != "nats" && scheme != "tls" {
		return xerrors.Errorf("xreplicasync: invalid route url scheme %q: must be \"nats\" or \"tls\"", scheme)
	}
	if port <= 0 {
		return xerrors.Errorf("xreplicasync: invalid route url port %d: must be positive", port)
	}
	return nil
}

// RouteURLFromReplicaHostname returns a RouteURLFunc that builds the route
// URL using the replica's Hostname field, ignoring RelayAddress entirely.
// This is appropriate when replicas advertise a routable DNS name distinct
// from the HTTP relay address.
func RouteURLFromReplicaHostname(scheme string, port int) (RouteURLFunc, error) {
	if err := validateRouteURLConfig(scheme, port); err != nil {
		return nil, err
	}
	return func(replica database.Replica) (string, error) {
		if replica.Hostname == "" {
			return "", xerrors.Errorf("xreplicasync: replica %s has empty hostname", replica.ID)
		}
		return fmt.Sprintf("%s://%s:%d", scheme, replica.Hostname, port), nil
	}, nil
}

// RouteURLFromRelayAddress returns a RouteURLFunc that extracts the host
// portion of the replica's RelayAddress and combines it with the configured
// scheme and port. The relay's own scheme and port are ignored: the relay is
// an HTTP endpoint while the route URL is for the NATS cluster port.
func RouteURLFromRelayAddress(scheme string, port int) (RouteURLFunc, error) {
	if err := validateRouteURLConfig(scheme, port); err != nil {
		return nil, err
	}
	return func(replica database.Replica) (string, error) {
		if replica.RelayAddress == "" {
			return "", xerrors.Errorf("xreplicasync: replica %s has empty relay address", replica.ID)
		}
		u, err := url.Parse(replica.RelayAddress)
		if err != nil {
			return "", xerrors.Errorf("xreplicasync: parse relay address %q for replica %s: %w", replica.RelayAddress, replica.ID, err)
		}
		host := u.Hostname()
		if host == "" {
			return "", xerrors.Errorf("xreplicasync: relay address %q for replica %s has empty host", replica.RelayAddress, replica.ID)
		}
		return fmt.Sprintf("%s://%s:%d", scheme, host, port), nil
	}, nil
}
