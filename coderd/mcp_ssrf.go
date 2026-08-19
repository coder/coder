package coderd

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"time"

	"golang.org/x/xerrors"
)

// mcpDiscoveryExtraBlockedPrefixes lists special-use CIDR ranges that
// the stdlib classification methods (IsLoopback, IsPrivate, etc.) do
// not cover. Blocking these prevents SSRF against carrier-grade NAT,
// benchmarking, documentation, discard-only, and the all-zeros "this
// network" ranges.
//
// IPv6 ranges already handled by stdlib:
//   - ::1/128        (IsLoopback)
//   - fc00::/7       (IsPrivate, ULA)
//   - fe80::/10      (IsLinkLocalUnicast)
//   - ff00::/8       (IsMulticast)
//   - ::/128         (IsUnspecified)
var mcpDiscoveryExtraBlockedPrefixes = []netip.Prefix{
	// IPv4 special-use ranges.
	netip.MustParsePrefix("0.0.0.0/8"),     // RFC 1122 "this network".
	netip.MustParsePrefix("100.64.0.0/10"), // RFC 6598 carrier-grade NAT.
	netip.MustParsePrefix("198.18.0.0/15"), // RFC 2544 benchmarking.

	// IPv6 special-use ranges not covered by stdlib.
	netip.MustParsePrefix("64:ff9b:1::/48"), // RFC 8215 IPv4/IPv6 translation.
	netip.MustParsePrefix("100::/64"),       // RFC 6666 discard-only.
	netip.MustParsePrefix("2001:2::/48"),    // RFC 5180 benchmarking.
	netip.MustParsePrefix("2001:db8::/32"),  // RFC 3849 documentation.
}

// isBlockedMCPDiscoveryAddr reports whether addr must not be reached
// during MCP OAuth2 discovery because it is in a private, loopback,
// link-local, multicast, unspecified, or other special-use range.
// IPv4-mapped IPv6 addresses are unmapped first so a literal like
// ::ffff:169.254.169.254 cannot bypass the IPv4 ranges. Prefixes in
// allowed exempt their range from blocking.
func isBlockedMCPDiscoveryAddr(addr netip.Addr, allowed []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, prefix := range allowed {
		if prefix.Contains(addr) {
			return false
		}
	}
	if addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() ||
		addr.IsInterfaceLocalMulticast() {
		return true
	}
	for _, prefix := range mcpDiscoveryExtraBlockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// newMCPSSRFHTTPClient returns an HTTP client for attacker-influenced MCP
// requests that refuses to connect to private or internal addresses. It is used
// for OAuth2 discovery, Dynamic Client Registration, and private chat-scoped
// MCP server calls. Without this guard a hostile server can pivot coderd into
// internal infrastructure such as cloud metadata services (CDM-02-002).
//
// The guard validates the resolved IPs at dial time and dials a
// validated IP directly, so DNS rebinding cannot swap in a private
// address between validation and connect, and 3xx redirects to
// internal targets are blocked when the redirected connection is
// dialed. Requests never use a proxy: through a proxy the destination
// IP is invisible to the dialer and the guard would be ineffective.
//
// base contributes its timeout and (when its transport is an
// *http.Transport) TLS configuration; its dialing behavior is always
// replaced with the guarded dialer.
func newMCPSSRFHTTPClient(base *http.Client, allowed []netip.Prefix) *http.Client {
	timeout := 30 * time.Second
	var transport *http.Transport
	if base != nil {
		if base.Timeout > 0 {
			timeout = base.Timeout
		}
		if t, ok := base.Transport.(*http.Transport); ok && t != nil {
			transport = t.Clone()
		}
	}
	if transport == nil {
		if t, ok := http.DefaultTransport.(*http.Transport); ok {
			transport = t.Clone()
		} else {
			transport = &http.Transport{}
		}
	}

	// Force every connection through the guarded dialer: no proxies
	// and no alternate dial paths that would bypass it.
	transport.Proxy = nil
	//nolint:staticcheck // Deprecated fields are cleared so the guarded DialContext is authoritative.
	transport.Dial = nil
	//nolint:staticcheck // Deprecated fields are cleared so the guarded DialContext is authoritative.
	transport.DialTLS = nil
	transport.DialTLSContext = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		lookupNetwork := "ip"
		switch network {
		case "tcp":
		case "tcp4":
			lookupNetwork = "ip4"
		case "tcp6":
			lookupNetwork = "ip6"
		default:
			return nil, xerrors.Errorf("network %q not permitted for MCP requests", network)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, xerrors.Errorf("split host/port %q: %w", addr, err)
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, lookupNetwork, host)
		if err != nil {
			return nil, xerrors.Errorf("resolve %q: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, xerrors.Errorf("no addresses for %q", host)
		}
		// Reject when ANY resolved address is blocked so a single
		// tainted DNS answer short-circuits the dial rather than
		// racing it.
		for _, ip := range ips {
			if isBlockedMCPDiscoveryAddr(ip, allowed) {
				return nil, xerrors.Errorf(
					"connection to %q blocked: %s is in a private/reserved IP range not permitted for MCP requests",
					host, ip.Unmap(),
				)
			}
		}
		// Dial a validated IP directly. Dialing by hostname would
		// re-resolve, letting a hostile resolver swap in a private
		// IP after validation (DNS rebinding). TLS verification
		// still uses the URL hostname via the transport's TLS
		// config.
		var dialer net.Dialer
		var firstErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if dialErr == nil {
				return conn, nil
			}
			if firstErr == nil {
				firstErr = dialErr
			}
		}
		return nil, firstErr
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Mirror the default client's redirect cap.
			if len(via) >= 10 {
				return xerrors.New("stopped after 10 redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return xerrors.Errorf("redirect to non-HTTP scheme %q blocked", req.URL.Scheme)
			}
			// Defense in depth: reject redirects to blocked IP
			// literals before the request is attempted. Hostnames
			// are validated post-resolution by the guarded dialer.
			if ip, err := netip.ParseAddr(req.URL.Hostname()); err == nil && isBlockedMCPDiscoveryAddr(ip, allowed) {
				return xerrors.Errorf(
					"redirect to %q blocked: destination is in a private/reserved IP range not permitted for MCP requests",
					req.URL.Host,
				)
			}
			return nil
		},
	}
}
