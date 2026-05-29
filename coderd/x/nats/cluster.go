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

	self := &url.URL{Scheme: "nats", Host: p.ns.ClusterAddr().String()}
	routes = filterSelfRoutes(routes, self)
	routes = sortRouteURLs(routes)

	if sortedURLsEqual(p.currentRoutes, routes) {
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
	return routes, nil
}

func filterSelfRoutes(routes []*url.URL, self *url.URL) []*url.URL {
	filtered := routes[:0]
	for _, route := range routes {
		if route.String() == self.String() {
			continue
		}
		filtered = append(filtered, route)
	}
	return filtered
}

func normalizeHostPort(address string) (string, error) {
	route, err := url.Parse(address)
	if err != nil {
		return "", xerrors.Errorf("parse peer address %q: %w", address, err)
	}
	if route.User != nil {
		return "", xerrors.Errorf("peer address %q must not include userinfo", address)
	}
	if route.Path != "" || route.RawQuery != "" || route.Fragment != "" {
		return "", xerrors.Errorf("peer address %q must not include path, query, or fragment", address)
	}

	host, port, err := net.SplitHostPort(route.Host)
	if err != nil {
		return "", xerrors.Errorf("split %q host port: %w", address, err)
	}
	if host == "" || port == "" {
		return "", xerrors.Errorf("%q must include host and port", address)
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return "", xerrors.Errorf("parse %q port: %w", address, err)
	}
	if portNumber <= 0 || portNumber > 65535 {
		return "", xerrors.Errorf("peer address %q must include a valid port", address)
	}
	return net.JoinHostPort(host, strconv.Itoa(portNumber)), nil
}

func sortRouteURLs(routes []*url.URL) []*url.URL {
	slices.SortFunc(routes, func(a, b *url.URL) int {
		return strings.Compare(a.String(), b.String())
	})
	return routes
}

// sortedURLsEqual assumes sorted slices.
func sortedURLsEqual(a, b []*url.URL) bool {
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
