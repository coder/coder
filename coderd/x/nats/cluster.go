package nats

import (
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// SetPeerAddresses replaces the configured NATS cluster peer routes.
func (p *Pubsub) SetPeerAddresses(addresses []string) error {
	p.clusterMu.Lock()
	defer p.clusterMu.Unlock()

	if p.ctx.Err() != nil {
		return errClosed
	}
	if !p.clustered {
		return xerrors.New("nats pubsub was not started with clustering enabled")
	}

	routes, err := parsePeerAddresses(addresses)
	if err != nil {
		return err
	}
	selfRoutes := []*url.URL{{
		Scheme: "nats",
		Host:   net.JoinHostPort(p.serverOpts.Cluster.Host, strconv.Itoa(p.serverOpts.Cluster.Port)),
	}}
	if clusterAddr := p.ns.ClusterAddr(); clusterAddr != nil {
		selfRoutes = append(selfRoutes, &url.URL{Scheme: "nats", Host: clusterAddr.String()})
	}
	routes = filterSelfRoutes(routes, selfRoutes...)
	if routeURLsEqual(p.currentRoutes, routes) {
		return nil
	}

	newOpts := p.serverOpts.Clone()
	newOpts.Routes = cloneRouteURLs(routes)
	if err := p.ns.ReloadOptions(newOpts); err != nil {
		return xerrors.Errorf("reload nats peer addresses: %w", err)
	}
	p.serverOpts = newOpts.Clone()
	p.currentRoutes = cloneRouteURLs(routes)
	return nil
}

func clusterRequested(opts Options) bool {
	return opts.ClusterHost != "" ||
		opts.ClusterPort != 0 ||
		opts.RoutePoolSize != 0 ||
		len(opts.PeerAddresses) > 0
}

func defaultOptions(opts Options) Options {
	if !clusterRequested(opts) {
		opts.disableCluster = true
	}
	return opts
}

func parsePeerAddresses(addresses []string) ([]*url.URL, error) {
	routesByAddress := make(map[string]*url.URL, len(addresses))
	for i, address := range addresses {
		trimmed := strings.TrimSpace(address)
		if trimmed == "" {
			return nil, xerrors.Errorf("peer address %d is empty", i)
		}

		normalizedHost, err := normalizeHostPort(trimmed)
		if err != nil {
			return nil, err
		}

		routesByAddress[normalizedHost] = &url.URL{
			Scheme: "nats",
			Host:   normalizedHost,
		}
	}

	routes := make([]*url.URL, 0, len(routesByAddress))
	for _, route := range routesByAddress {
		routes = append(routes, route)
	}
	// Sort them so that we can easily check for equality on incoming SetPeerAddresses calls.
	slices.SortFunc(routes, func(a, b *url.URL) int {
		return strings.Compare(a.String(), b.String())
	})
	return routes, nil
}

func filterSelfRoutes(routes []*url.URL, selfRoutes ...*url.URL) []*url.URL {
	self := make(map[string]struct{}, len(selfRoutes))
	for _, route := range selfRoutes {
		if route == nil {
			continue
		}
		self[route.Host] = struct{}{}
	}
	if len(self) == 0 {
		return routes
	}

	filtered := routes[:0]
	for _, route := range routes {
		if route == nil {
			continue
		}
		if _, ok := self[route.Host]; ok {
			continue
		}
		filtered = append(filtered, route)
	}
	return filtered
}

func normalizeHostPort(address string) (string, error) {
	if strings.ContainsAny(address, "/?#") {
		return "", xerrors.Errorf("peer address %q must not include path, query, or fragment", address)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return "", xerrors.Errorf("peer address %q must include host and port", address)
	}
	if strings.Contains(host, "@") {
		return "", xerrors.Errorf("peer address %q must not include userinfo", address)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber <= 0 || portNumber > 65535 {
		return "", xerrors.Errorf("peer address %q must include a valid port", address)
	}
	return net.JoinHostPort(host, strconv.Itoa(portNumber)), nil
}

func routeURLsEqual(a, b []*url.URL) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			return false
		}
	}
	return true
}

func cloneRouteURLs(routes []*url.URL) []*url.URL {
	if routes == nil {
		return nil
	}
	clones := make([]*url.URL, len(routes))
	for i, route := range routes {
		if route == nil {
			continue
		}
		clone := *route
		clones[i] = &clone
	}
	return clones
}
