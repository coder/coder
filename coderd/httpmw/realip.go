package httpmw

import (
	"fmt"
	"errors"
	"context"
	"net"
	"net/http"

	"strings"
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
		addr := getRemoteAddress(req.Header.Get(trustedHeader))
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
		req.Header.Set(header, req.Header.Get(header))
	}
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
		return errors.New("request does not contain realip.State; was it processed by httpmw.ExtractRealIP?")
	}
	remoteAddr := getRemoteAddress(req.RemoteAddr)
	if remoteAddr == nil {
		return fmt.Errorf("failed to parse remote address: %s", remoteAddr)
	}
	proxyAddr := getRemoteAddress(state.OriginalRemoteAddr)
	if proxyAddr == nil {
		return fmt.Errorf("failed to parse original address: %s", proxyAddr)
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
// getRemoteAddress extracts the IP address from the given string. If
// the string contains commas, it assumes that the first part is the
// original address.
func getRemoteAddress(address string) net.IP {
	// X-Forwarded-For may contain multiple addresses, in case the
	// proxies are chained; the first value is the client address
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
			return nil, fmt.Errorf("parse proxy origin %q: %w", origin, err)
		}
		config.TrustedOrigins = append(config.TrustedOrigins, network)
	}

	for index, header := range headers {
		headers[index] = http.CanonicalHeaderKey(header)
	}
	config.TrustedHeaders = headers

	return config, nil
}
