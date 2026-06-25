package nats

import (
	"errors"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const defaultClusterTokenUsername = "coder"

// PeerFetcher fetches NATS peer route addresses.
type PeerFetcher interface {
	FetchNATSPeers() []string
	SetSelfNATSPort(port int32)
}

type NopPeerFetcher struct{}

func (NopPeerFetcher) SetSelfNATSPort(int32) {}

func (NopPeerFetcher) FetchNATSPeers() []string {
	return nil
}

// SetPeerFetcher replaces the peer fetcher used by RefreshPeers and triggers
// an immediate peer refresh. Passing nil disables peering.
func (p *Pubsub) SetPeerFetcher(fetcher PeerFetcher) {
	p.mu.Lock()
	if fetcher == nil {
		fetcher = NopPeerFetcher{}
	}
	p.peerFetcher = fetcher
	p.mu.Unlock()
	if ca := p.Server.ClusterAddr(); ca != nil {
		if ca.Port >= 1 && ca.Port <= 65535 {
			//nolint:gosec // range checked above so conversion is safe.
			fetcher.SetSelfNATSPort(int32(ca.Port))
		} else {
			p.logger.Warn(p.ctx, "unexpected NATS cluster port", slog.F("port", ca.Port))
		}
	}
	p.RefreshPeers()
}

// SetClusterTLSProvider installs the provider used to mint the cluster route
// TLS config on the next peer-route reload and triggers an immediate refresh.
// The provider is invoked once: the first reload that runs while it is set
// installs cluster mTLS, and later reloads reuse the already-applied config.
// Passing nil leaves routes plaintext.
func (p *Pubsub) SetClusterTLSProvider(provider ClusterTLSProvider) {
	p.clusterMu.Lock()
	p.clusterTLSProvider = provider
	p.clusterMu.Unlock()
	p.RefreshPeers()
}

// RefreshPeers signals the peer refresh worker to fetch and apply the latest
// peer route addresses. Multiple pending refreshes are coalesced.
func (p *Pubsub) RefreshPeers() {
	select {
	case p.peerRefresh <- struct{}{}:
	default:
	}
}

func (p *Pubsub) runPeerRefresh() {
	for {
		p.mu.Lock()
		fetcher := p.peerFetcher
		p.mu.Unlock()

		addrs := fetcher.FetchNATSPeers()
		if err := p.setPeerAddresses(addrs); err != nil {
			if errors.Is(err, errClosed) && p.ctx.Err() != nil {
				return
			}
			p.logger.Error(p.ctx, "refresh nats peers", slog.Error(err))
		}

		select {
		case <-p.ctx.Done():
			return
		case <-p.peerRefresh:
		}
	}
}

// setPeerAddresses replaces the configured NATS cluster peer routes.
func (p *Pubsub) setPeerAddresses(addresses []string) error {
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

	self := &url.URL{Scheme: "nats", Host: p.Server.ClusterAddr().String()}
	routes = filterSelfRoutes(routes, self)

	if p.opts.ClusterAuthToken != "" {
		routes = routesWithAuth(routes, p.opts.ClusterAuthToken)
	}

	routes = sortRouteURLs(routes)

	// Skip the reload when nothing changed, but never skip the first reload
	// that still needs to install cluster TLS: routes may already match (e.g.
	// after the provider is wired with the peer set unchanged) yet the TLS
	// config has not been applied yet.
	tlsPending := p.clusterTLSProvider != nil && !p.clusterTLSApplied
	if sortedURLsEqual(p.currentRoutes, routes) && !tlsPending {
		return nil
	}

	newOpts := p.serverOpts.Clone()
	newOpts.Routes = cloneRouteURLs(routes)
	// Install cluster mTLS once, before the routes in this reload handshake.
	// No plaintext window exists because the server booted with no routes, so
	// the first route ever established already negotiates over mTLS.
	if tlsPending {
		tlsConfig, err := p.clusterTLSProvider()
		if err != nil {
			return xerrors.Errorf("build cluster tls: %w", err)
		}
		if tlsConfig != nil {
			newOpts.Cluster.TLSConfig = tlsConfig
			newOpts.Cluster.TLSTimeout = clusterTLSTimeout.Seconds()
		}
	}
	if err := p.Server.ReloadOptions(newOpts); err != nil {
		return xerrors.Errorf("reload nats peer addresses: %w", err)
	}
	p.serverOpts = newOpts.Clone()
	p.currentRoutes = cloneRouteURLs(routes)
	if newOpts.Cluster.TLSConfig != nil {
		p.clusterTLSApplied = true
		p.logger.Debug(p.ctx, "installed nats cluster mTLS config via peer reload",
			slog.F("routes", len(routes)),
		)
	}
	return nil
}

func parsePeerAddresses(addresses []string) ([]*url.URL, error) {
	routesByAddress := make(map[string]*url.URL, len(addresses))
	for i, address := range addresses {
		trimmed := strings.TrimSpace(address)
		if trimmed == "" {
			return nil, xerrors.Errorf("peer address %d is empty", i)
		}

		host, port, err := normalizeHostPort(trimmed)
		if err != nil {
			return nil, err
		}

		hostPort := net.JoinHostPort(host, strconv.Itoa(port))
		routesByAddress[hostPort] = &url.URL{
			Scheme: "nats",
			Host:   hostPort,
		}
	}

	routes := make([]*url.URL, 0, len(routesByAddress))
	for _, route := range routesByAddress {
		routes = append(routes, route)
	}
	return routes, nil
}

func filterSelfRoutes(routes []*url.URL, self *url.URL) []*url.URL {
	filtered := make([]*url.URL, 0, len(routes))
	for _, route := range routes {
		if route.String() == self.String() {
			continue
		}
		filtered = append(filtered, route)
	}
	return filtered
}

func normalizeHostPort(address string) (string, int, error) {
	route, err := url.Parse(address)
	if err != nil {
		return "", 0, xerrors.Errorf("parse peer address %q: %w", address, err)
	}
	if route.User != nil {
		return "", 0, xerrors.Errorf("peer address %q must not include userinfo", address)
	}
	if route.Path != "" || route.RawQuery != "" || route.Fragment != "" {
		return "", 0, xerrors.Errorf("peer address %q must not include path, query, or fragment", address)
	}
	if route.Scheme != "nats" {
		return "", 0, xerrors.Errorf("peer address %q must use nats scheme", address)
	}

	host, port, err := net.SplitHostPort(route.Host)
	if err != nil {
		return "", 0, xerrors.Errorf("split %q host port: %w", address, err)
	}
	if host == "" || port == "" {
		return "", 0, xerrors.Errorf("%q must include host and port", address)
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, xerrors.Errorf("parse %q port: %w", address, err)
	}
	if portNumber <= 0 || portNumber > 65535 {
		return "", 0, xerrors.Errorf("peer address %q must include a valid port", address)
	}
	return host, portNumber, nil
}

func sortRouteURLs(routes []*url.URL) []*url.URL {
	slices.SortFunc(routes, func(a, b *url.URL) int {
		return strings.Compare(a.String(), b.String())
	})
	return routes
}

func routesWithAuth(routes []*url.URL, token string) []*url.URL {
	if token == "" {
		return routes
	}
	withAuth := make([]*url.URL, 0, len(routes))
	for _, route := range routes {
		if route == nil {
			withAuth = append(withAuth, nil)
			continue
		}
		clone := *route
		clone.User = url.UserPassword(defaultClusterTokenUsername, token)
		withAuth = append(withAuth, &clone)
	}
	return withAuth
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
