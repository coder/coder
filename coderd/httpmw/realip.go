package httpmw

import (
	"context"
	"net"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
)

const (
	headerXForwardedFor   string = "X-Forwarded-For"
	headerXForwardedProto string = "X-Forwarded-Proto"
)

// RealIPConfig configures the search order for the function, which controls
// which headers to consider trusted.
type RealIPConfig struct {
	// TrustedOrigins is a list of networks that will be trusted. If
	// any non-trusted address supplies these headers, they will be
	// ignored.
	TrustedOrigins []*net.IPNet

	// TrustedHeaders lists headers that are trusted for forwarding
	// IP addresses. e.g. "CF-Connecting-IP", "True-Client-IP", etc.
	TrustedHeaders []string
}

// ExtractRealIP is a middleware that uses headers from reverse proxies to
// propagate origin IP address information, when configured to do so.
func ExtractRealIP(config *RealIPConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Preserve the original TLS connection state and RemoteAddr
			req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, &RealIPState{
				Config:             config,
				OriginalRemoteAddr: req.RemoteAddr,
			}))

			info, err := ExtractRealIPAddress(config, req)
			if err != nil {
				httpapi.InternalServerError(w, err)
				return
			}
			req.RemoteAddr = info.String()

			next.ServeHTTP(w, req)
		})
	}
}

// ExtractRealIPAddress returns the original client address according to the
// configuration and headers. It does not mutate the original request.
func ExtractRealIPAddress(config *RealIPConfig, req *http.Request) (net.IP, error) {
	if config == nil {
		config = &RealIPConfig{
			TrustedOrigins: nil,
			TrustedHeaders: nil,
		}
	}

	cf := isContainedIn(config.TrustedOrigins, getRemoteAddress(req.RemoteAddr))
	if !cf {
		// Address is not valid or the origin is not trusted; use the
		// original address
		return getRemoteAddress(req.RemoteAddr), nil
	}

	for _, trustedHeader := range config.TrustedHeaders {
		// X-Forwarded-For is a list-valued header. Per RFC 7230, multiple
		// field lines with the same name are equivalent to a single
		// comma-separated value. Join them so a client cannot hide a spoofed
		// address in the first field line, which Header.Get would return on
		// its own. Other forwarding headers carry a single edge-proxy value,
		// so use Header.Get to preserve their first-value semantics.
		value := req.Header.Get(trustedHeader)
		if http.CanonicalHeaderKey(trustedHeader) == headerXForwardedFor {
			value = strings.Join(req.Header.Values(trustedHeader), ",")
		}
		addr := extractForwardedAddress(config, value)
		if addr != nil {
			return addr, nil
		}
	}

	return getRemoteAddress(req.RemoteAddr), nil
}

// FilterUntrustedOriginHeaders removes all known proxy headers from the
// request for untrusted origins, and ensures that only one copy
// of each proxy header is set.
func FilterUntrustedOriginHeaders(config *RealIPConfig, req *http.Request) {
	if config == nil {
		config = &RealIPConfig{
			TrustedOrigins: nil,
			TrustedHeaders: nil,
		}
	}

	cf := isContainedIn(config.TrustedOrigins, getRemoteAddress(req.RemoteAddr))
	if !cf {
		// Address is not valid or the origin is not trusted; clear
		// all known proxy headers and return
		for _, header := range config.TrustedHeaders {
			req.Header.Del(header)
		}
		return
	}

	for _, header := range config.TrustedHeaders {
		// X-Forwarded-For is a list-valued header whose field lines are
		// equivalent to a single comma-separated value (RFC 7230 section
		// 3.2.2). Join them so later hops are not dropped when collapsing to a
		// single line. Other forwarding headers carry a single value.
		if http.CanonicalHeaderKey(header) == headerXForwardedFor {
			req.Header.Set(header, strings.Join(req.Header.Values(header), ","))
			continue
		}
		req.Header.Set(header, req.Header.Get(header))
	}
}

// EffectiveHost returns the host Coder should trust for request handling.
// It uses X-Forwarded-Host only when the immediate peer is a configured
// trusted proxy. Otherwise it uses the received Host header.
func EffectiveHost(config *RealIPConfig, r *http.Request) string {
	if config == nil {
		config = &RealIPConfig{
			TrustedOrigins: nil,
			TrustedHeaders: nil,
		}
	}

	// When ExtractRealIP has run, r.RemoteAddr may hold the forwarded
	// client IP, and we should use the original socket peer for proxy
	// trust decisions.
	remoteAddr := r.RemoteAddr
	state := RealIP(r.Context())
	if state != nil && state.OriginalRemoteAddr != "" {
		remoteAddr = state.OriginalRemoteAddr
	}

	if isContainedIn(config.TrustedOrigins, getRemoteAddress(remoteAddr)) {
		if host := r.Header.Get(httpapi.XForwardedHostHeader); host != "" {
			return host
		}
	}

	return r.Host
}

// EnsureXForwardedForHeader ensures that the request has an X-Forwarded-For
// header. It uses the following logic:
//
//  1. If we have a direct connection (remoteAddr == proxyAddr), then
//     set it to remoteAddr
//  2. If we have a proxied connection (remoteAddr != proxyAddr) and
//     X-Forwarded-For doesn't begin with remoteAddr, then overwrite
//     it with remoteAddr,proxyAddr
//  3. If we have a proxied connection (remoteAddr != proxyAddr) and
//     X-Forwarded-For begins with remoteAddr, then append proxyAddr
//     to the original X-Forwarded-For header
//  4. If X-Forwarded-Proto is not set, then it will be set to "https"
//     if req.TLS != nil, otherwise it will be set to "http"
func EnsureXForwardedForHeader(req *http.Request) error {
	state := RealIP(req.Context())
	if state == nil {
		return xerrors.New("request does not contain realip.State; was it processed by httpmw.ExtractRealIP?")
	}

	remoteAddr := getRemoteAddress(req.RemoteAddr)
	if remoteAddr == nil {
		return xerrors.Errorf("failed to parse remote address: %s", remoteAddr)
	}

	proxyAddr := getRemoteAddress(state.OriginalRemoteAddr)
	if proxyAddr == nil {
		return xerrors.Errorf("failed to parse original address: %s", proxyAddr)
	}

	if remoteAddr.Equal(proxyAddr) {
		req.Header.Set(headerXForwardedFor, remoteAddr.String())
	} else {
		forwarded := req.Header.Get(headerXForwardedFor)
		if forwarded == "" || !remoteAddr.Equal(getRemoteAddress(forwarded)) {
			req.Header.Set(headerXForwardedFor, remoteAddr.String()+","+proxyAddr.String())
		} else {
			req.Header.Set(headerXForwardedFor, forwarded+","+proxyAddr.String())
		}
	}

	if req.Header.Get(headerXForwardedProto) == "" {
		if req.TLS != nil {
			req.Header.Set(headerXForwardedProto, "https")
		} else {
			req.Header.Set(headerXForwardedProto, "http")
		}
	}

	return nil
}

// getRemoteAddress extracts a single IP address from the given string,
// stripping a port if present. If the string contains commas, only the
// portion before the first comma is parsed. This helper does not select the
// real client from a multi-hop X-Forwarded-For chain; use
// extractForwardedAddress for that, which accounts for client-supplied values.
func getRemoteAddress(address string) net.IP {
	// A value may contain a port and, for a raw X-Forwarded-For value, more
	// than one comma-separated address. Parse only the part before the first
	// comma.
	i := strings.IndexByte(address, ',')
	if i == -1 {
		i = len(address)
	}

	// If the address contains a port, remove it
	firstAddress := address[:i]
	host, _, err := net.SplitHostPort(firstAddress)
	if err != nil {
		// This will error if there is no port, so try to parse the address
		return net.ParseIP(firstAddress)
	}
	return net.ParseIP(host)
}

// extractForwardedAddress parses a comma-separated forwarding header value and
// returns the rightmost address that is not a trusted origin. Reverse proxies
// append the peer that connected to them, so when every trusted proxy hop is
// listed in TrustedOrigins, the rightmost untrusted address is the real client;
// any values a client prepends to spoof its address sit to the left of the
// addresses inserted by trusted proxies and are ignored. If every parsed address
// is a trusted origin, the leftmost address is returned. It returns nil when no
// address can be parsed.
func extractForwardedAddress(config *RealIPConfig, value string) net.IP {
	parts := strings.Split(value, ",")
	var leftmost net.IP
	for i := len(parts) - 1; i >= 0; i-- {
		ip := getRemoteAddress(strings.TrimSpace(parts[i]))
		if ip == nil {
			continue
		}
		// Iterating right-to-left, so the last assignment is the leftmost
		// valid address, used as the fallback when all hops are trusted.
		leftmost = ip
		if !isContainedIn(config.TrustedOrigins, ip) {
			return ip
		}
	}
	return leftmost
}

// isContainedIn checks that the given address is contained in the given
// network.
func isContainedIn(networks []*net.IPNet, address net.IP) bool {
	for _, network := range networks {
		if network.Contains(address) {
			return true
		}
	}

	return false
}

// RealIPState is the original state prior to modification by this middleware,
// useful for getting information about the connecting client if needed.
type RealIPState struct {
	// Config is the configuration applied in the middleware. Consider
	// this read-only and do not modify.
	Config *RealIPConfig

	// OriginalRemoteAddr is the original RemoteAddr for the request.
	OriginalRemoteAddr string
}

type ctxKey struct{}

// FromContext retrieves the state from the given context.Context.
func RealIP(ctx context.Context) *RealIPState {
	state, ok := ctx.Value(ctxKey{}).(*RealIPState)
	if !ok {
		return nil
	}
	return state
}

// ParseRealIPConfig takes a raw string array of headers and origins
// to produce a config.
func ParseRealIPConfig(headers, origins []string) (*RealIPConfig, error) {
	config := &RealIPConfig{
		TrustedOrigins: []*net.IPNet{},
		TrustedHeaders: []string{},
	}
	for _, origin := range origins {
		_, network, err := net.ParseCIDR(origin)
		if err != nil {
			return nil, xerrors.Errorf("parse proxy origin %q: %w", origin, err)
		}
		config.TrustedOrigins = append(config.TrustedOrigins, network)
	}
	for index, header := range headers {
		headers[index] = http.CanonicalHeaderKey(header)
	}
	config.TrustedHeaders = headers

	return config, nil
}
