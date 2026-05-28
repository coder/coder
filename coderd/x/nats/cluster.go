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

	if p.serverOpts == nil || p.ns == nil {
		return errClosed
	}

	selfAddresses := []string{effectiveClusterAddress(p.serverOpts.Cluster.Host, p.serverOpts.Cluster.Port)}
	if clusterAddr := p.ns.ClusterAddr(); clusterAddr != nil {
		selfAddresses = append(selfAddresses, clusterAddr.String())
	}
	routes, err := parsePeerAddresses(addresses, selfAddresses...)
	if err != nil {
		return err
	}
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

func parsePeerAddresses(addresses []string, selfAddresses ...string) ([]*url.URL, error) {
	self := normalizedAddresses(selfAddresses...)
	routesByAddress := make(map[string]*url.URL, len(addresses))
	for i, address := range addresses {
		trimmed := strings.TrimSpace(address)
		if trimmed == "" {
			return nil, xerrors.Errorf("peer address %d is empty", i)
		}

		route, err := url.Parse(trimmed)
		if err != nil {
			return nil, xerrors.Errorf("parse peer address %q: %w", address, err)
		}
		if route.Scheme != "nats" {
			return nil, xerrors.Errorf("peer address %q must use nats scheme", address)
		}
		if route.User != nil {
			return nil, xerrors.Errorf("peer address %q must not include userinfo", address)
		}
		if route.Path != "" || route.RawQuery != "" || route.Fragment != "" {
			return nil, xerrors.Errorf("peer address %q must not include path, query, or fragment", address)
		}

		host, port, err := net.SplitHostPort(route.Host)
		if err != nil || host == "" || port == "" {
			return nil, xerrors.Errorf("peer address %q must include host and port", address)
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber <= 0 || portNumber > 65535 {
			return nil, xerrors.Errorf("peer address %q must include a valid port", address)
		}

		normalizedHost := net.JoinHostPort(host, strconv.Itoa(portNumber))
		if _, ok := self[normalizedHost]; ok {
			continue
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
	slices.SortFunc(routes, func(a, b *url.URL) int {
		return strings.Compare(routeURLString(a), routeURLString(b))
	})
	return routes, nil
}

func effectiveClusterAddress(host string, port int) string {
	if host == "" || port <= 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func normalizedAddresses(addresses ...string) map[string]struct{} {
	if len(addresses) == 0 {
		return nil
	}

	normalized := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		if address == "" {
			continue
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil || host == "" || port == "" {
			continue
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber <= 0 || portNumber > 65535 {
			continue
		}
		normalized[net.JoinHostPort(host, strconv.Itoa(portNumber))] = struct{}{}
	}
	return normalized
}

func routeURLsEqual(a, b []*url.URL) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if routeURLString(a[i]) != routeURLString(b[i]) {
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

func routeURLString(route *url.URL) string {
	if route == nil {
		return ""
	}
	return route.String()
}
