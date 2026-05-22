package nats

import (
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

func clusterEnabled(opts Options) bool {
	return opts.ClusterName != "" ||
		opts.ClusterHost != "" ||
		opts.ClusterPort != 0 ||
		opts.ClusterAdvertise != "" ||
		opts.RoutePoolSize != 0 ||
		len(opts.PeerAddresses) > 0
}

func parsePeerAddresses(addresses []string) ([]*url.URL, error) {
	routes := make([]*url.URL, 0, len(addresses))
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

		normalized := &url.URL{
			Scheme: "nats",
			Host:   net.JoinHostPort(host, strconv.Itoa(portNumber)),
		}
		routes = append(routes, normalized)
	}
	return routes, nil
}

func sortRouteURLs(routes []*url.URL) {
	slices.SortFunc(routes, func(a, b *url.URL) int {
		return strings.Compare(routeURLString(a), routeURLString(b))
	})
}

func dedupeRouteURLs(routes []*url.URL) []*url.URL {
	if len(routes) < 2 {
		return cloneRouteURLs(routes)
	}
	deduped := make([]*url.URL, 0, len(routes))
	var previous string
	for _, route := range routes {
		current := routeURLString(route)
		if current == previous {
			continue
		}
		previous = current
		if route == nil {
			deduped = append(deduped, nil)
			continue
		}
		clone := *route
		deduped = append(deduped, &clone)
	}
	return deduped
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

func filterSelfRoutes(routes []*url.URL, selfAddresses ...string) []*url.URL {
	if len(routes) == 0 || len(selfAddresses) == 0 {
		return cloneRouteURLs(routes)
	}

	self := make(map[string]struct{}, len(selfAddresses))
	for _, address := range selfAddresses {
		if address == "" {
			continue
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil || host == "" || port == "" {
			continue
		}
		self[net.JoinHostPort(host, port)] = struct{}{}
	}
	if len(self) == 0 {
		return cloneRouteURLs(routes)
	}

	filtered := make([]*url.URL, 0, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		if _, ok := self[route.Host]; ok {
			continue
		}
		clone := *route
		filtered = append(filtered, &clone)
	}
	return filtered
}

// SetPeerAddresses replaces the configured NATS cluster peer routes.
func (p *Pubsub) SetPeerAddresses(addresses []string) error {
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
	selfAddresses := []string{p.opts.ClusterAdvertise}
	if p.ns != nil {
		if clusterAddr := p.ns.ClusterAddr(); clusterAddr != nil {
			selfAddresses = append(selfAddresses, clusterAddr.String())
		}
	}
	routes = filterSelfRoutes(routes, selfAddresses...)
	sortRouteURLs(routes)
	routes = dedupeRouteURLs(routes)

	p.clusterMu.Lock()
	defer p.clusterMu.Unlock()

	if p.ctx.Err() != nil {
		return errClosed
	}
	if routeURLsEqual(p.currentRoutes, routes) {
		return nil
	}
	if p.serverOpts == nil || p.ns == nil {
		return errClosed
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
