package mcpssrf

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// extraBlockedPrefixes covers special-use ranges that netip.Addr's built-in
// classifications do not recognize, preventing them from bypassing MCP SSRF protection.
var extraBlockedPrefixes = []netip.Prefix{
	// IPv4 special-use ranges.
	netip.MustParsePrefix("0.0.0.0/8"),       // RFC 1122 "this network".
	netip.MustParsePrefix("100.64.0.0/10"),   // RFC 6598 carrier-grade NAT.
	netip.MustParsePrefix("192.0.0.0/24"),    // RFC 6890 IETF protocol assignments.
	netip.MustParsePrefix("192.0.2.0/24"),    // RFC 5737 documentation.
	netip.MustParsePrefix("192.31.196.0/24"), // RFC 7535 AS112-v4.
	netip.MustParsePrefix("192.52.193.0/24"), // RFC 7450 AMT.
	netip.MustParsePrefix("192.88.99.0/24"),  // RFC 7526 deprecated 6to4 relay anycast.
	netip.MustParsePrefix("192.175.48.0/24"), // RFC 7534 direct delegation AS112 service.
	netip.MustParsePrefix("198.18.0.0/15"),   // RFC 2544 benchmarking.
	netip.MustParsePrefix("198.51.100.0/24"), // RFC 5737 documentation.
	netip.MustParsePrefix("203.0.113.0/24"),  // RFC 5737 documentation.
	netip.MustParsePrefix("240.0.0.0/4"),     // RFC 1112 reserved.

	// IPv6 special-use ranges not covered by stdlib.
	netip.MustParsePrefix("64:ff9b::/96"),      // RFC 6052 IPv4/IPv6 translation.
	netip.MustParsePrefix("64:ff9b:1::/48"),    // RFC 8215 IPv4/IPv6 translation.
	netip.MustParsePrefix("100::/64"),          // RFC 6666 discard-only.
	netip.MustParsePrefix("100:0:0:1::/64"),    // RFC 9780 dummy prefix.
	netip.MustParsePrefix("2001::/23"),         // RFC 2928 IETF protocol assignments.
	netip.MustParsePrefix("2001:db8::/32"),     // RFC 3849 documentation.
	netip.MustParsePrefix("2002::/16"),         // RFC 3056 6to4.
	netip.MustParsePrefix("2620:4f:8000::/48"), // RFC 7534 direct delegation AS112 service.
	netip.MustParsePrefix("3fff::/20"),         // RFC 9637 documentation.
	netip.MustParsePrefix("5f00::/16"),         // RFC 9602 segment routing SIDs.
	netip.MustParsePrefix("fec0::/10"),         // RFC 3879 deprecated site-local addresses.
}

// ParseAllowedPrefix parses an allowed CIDR and converts IPv4-mapped IPv6
// prefixes to equivalent IPv4 prefixes. Prefixes shorter than the 96-bit
// IPv4-mapped marker cannot be represented as IPv4 ranges.
func ParseAllowedPrefix(raw string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return netip.Prefix{}, err
	}
	if !prefix.Addr().Is4In6() {
		return prefix, nil
	}
	if prefix.Bits() < 96 {
		return netip.Prefix{}, xerrors.Errorf("IPv4-mapped IPv6 prefix length must be at least 96 bits")
	}
	return netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96), nil
}

// IPv4-mapped IPv6 addresses are normalized before checking so mapped private
// addresses cannot bypass IPv4 restrictions. Allowed prefixes take precedence.
func isBlockedAddr(addr netip.Addr, allowed []netip.Prefix) bool {
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
	for _, prefix := range extraBlockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// CheckSameOriginRedirect allows only method-preserving redirects within the
// original scheme, host, and port.
func CheckSameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return xerrors.New("stopped after 10 redirects")
	}
	origin := via[0]
	previous := via[len(via)-1]
	if req.Method != previous.Method {
		return xerrors.Errorf("redirect changed method from %s to %s", previous.Method, req.Method)
	}
	if req.URL.Scheme != origin.URL.Scheme ||
		!strings.EqualFold(req.URL.Hostname(), origin.URL.Hostname()) ||
		normalizedPort(req.URL) != normalizedPort(origin.URL) {
		return xerrors.Errorf("redirect must stay on origin %q", origin.URL.Scheme+"://"+origin.URL.Host)
	}
	return nil
}

func normalizedPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch u.Scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

// NewHTTPClient blocks private and special-use destinations unless allowed.
// It validates and dials resolved IPs directly to prevent DNS rebinding.
// Base client timeouts and non-routing transport settings are preserved.
func NewHTTPClient(base *http.Client, allowed []netip.Prefix) *http.Client {
	timeout := 30 * time.Second
	var transport *http.Transport
	if base != nil {
		timeout = base.Timeout
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
			return nil, xerrors.Errorf("network %q not permitted for MCP traffic", network)
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
			if isBlockedAddr(ip, allowed) {
				return nil, xerrors.Errorf(
					"connection to %q blocked: %s is in a private/reserved IP range not permitted for MCP traffic",
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
			if ip, err := netip.ParseAddr(req.URL.Hostname()); err == nil && isBlockedAddr(ip, allowed) {
				return xerrors.Errorf(
					"redirect to %q blocked: destination is in a private/reserved IP range not permitted for MCP traffic",
					req.URL.Host,
				)
			}
			return nil
		},
	}
}
