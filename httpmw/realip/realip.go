// realip determines the originating client's IP address, considering
// proxies if applicable.
//
// This is similar in nature to other middlewares, such as:
//
// - https://github.com/go-chi/chi/blob/master/middleware/realip.go (MIT)
// - https://github.com/tomasen/realip/blob/master/realip.go (MIT)
//
// However, this middleware supports additional configuration options and
// filters out untrusted headers, which is important for proxied connections.
package realip

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"

	"golang.org/x/xerrors"
)

const (
	// Note: these should be canonicalized (see http.CanonicalHeaderKey)
	// or else things will not work correctly
	headerCFConnectingIP  string = "Cf-Connecting-Ip"
	headerTrueClientIP    string = "True-Client-Ip"
	headerXRealIP         string = "X-Real-Ip"
	headerXForwardedFor   string = "X-Forwarded-For"
	headerXForwardedProto string = "X-Forwarded-Proto"
)

var headersAll = []string{
	headerCFConnectingIP,
	headerTrueClientIP,
	headerXRealIP,
	headerXForwardedFor,
	headerXForwardedProto,
}

// Config configures the search order for the function, which controls
// which headers to consider trusted.
type Config struct {
	// TrustedOrigins is a list of networks that will be trusted. If
	// any non-trusted address supplies these headers, they will be
	// ignored.
	TrustedOrigins []*net.IPNet

	// CloudflareConnectingIP trusts the CF-Connecting-IP header.
	// https://support.cloudflare.com/hc/en-us/articles/206776727-Understanding-the-True-Client-IP-Header
	CloudflareConnectingIP bool

	// TrueClientIP trusts the True-Client-IP header.
	TrueClientIP bool

	// XRealIP trusts the X-Real-IP header.
	XRealIP bool

	// X-Forwarded-For trusts the X-Forwarded-For and X-Forwarded-Proto
	// headers.
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Forwarded-Proto
	XForwardedFor bool
}

type AddrInfo struct {
	// IP is the remote IP address.
	IP net.IP
	// TLS is true if the protocol was HTTPS; false otherwise.
	TLS bool
}

// Middleware is a middleware that uses headers from reverse proxies to
// propagate origin IP address information, when configured to do so.
func Middleware(config *Config) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Preserve the original TLS connection state and RemoteAddr
			req = req.WithContext(WithState(req.Context(), &State{
				Config:             config,
				OriginalTLS:        req.TLS,
				OriginalRemoteAddr: req.RemoteAddr,
			}))

			info, err := ExtractAddress(config, req)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, err.Error())
				return
			}

			if info.IP != nil {
				req.RemoteAddr = info.IP.String()
			}

			if info.TLS && req.TLS == nil {
				// If the client is connected over HTTPS, and the connection
				// between the reverse proxy and Coder is not secure, then
				// add an empty connection state.
				req.TLS = &tls.ConnectionState{}
			} else if !info.TLS && req.TLS != nil {
				// If the client is connected over HTTP, and the connection
				// between the reverse proxy and Coder is secure, then clear
				// the connection state.
				req.TLS = nil
			}

			next.ServeHTTP(w, req)
		})
	}
}

// ExtractAddress returns the original client address according to the
// configuration and headers. It does not mutate the original request.
func ExtractAddress(config *Config, req *http.Request) (AddrInfo, error) {
	if config == nil {
		config = &Config{}
	}

	info := AddrInfo{
		IP:  getRemoteAddress(req.RemoteAddr),
		TLS: req.TLS != nil,
	}

	cf := isContainedIn(config.TrustedOrigins, info.IP)
	if !cf {
		// Address is not valid or the origin is not trusted; use the
		// original address
		return info, nil
	}

	// Accept X-Forwarded-Proto for trusted origins, even if we do not trust
	// X-Forwarded-For. This header is not a security risk, so we can accept
	// it more liberally.
	switch req.Header.Get(headerXForwardedProto) {
	case "":
		// If the header is not set, use the original protocol
	case "https":
		// The reverse proxy indicates the client is connected over HTTPS
		info.TLS = true
	default:
		// The reverse proxy returned HTTP or garbage
		info.TLS = false
	}

	// We want to prefer (in order):
	// - CF-Connecting-IP
	// - True-Client-IP
	// - X-Real-IP
	// - X-Forwarded-For
	if config.CloudflareConnectingIP {
		addr := getRemoteAddress(req.Header.Get(headerCFConnectingIP))
		if addr != nil {
			info.IP = addr
			return info, nil
		}
	}

	if config.TrueClientIP {
		addr := getRemoteAddress(req.Header.Get(headerTrueClientIP))
		if addr != nil {
			info.IP = addr
			return info, nil
		}
	}

	if config.XRealIP {
		addr := getRemoteAddress(req.Header.Get(headerXRealIP))
		if addr != nil {
			info.IP = addr
			return info, nil
		}
	}

	if config.XForwardedFor {
		addr := getRemoteAddress(req.Header.Get(headerXForwardedFor))
		if addr != nil {
			info.IP = addr
			return info, nil
		}
	}

	return info, nil
}

// FilterUntrustedHeaders removes all known proxy headers from the
// request for untrusted origins, and ensures that only one copy
// of each proxy header is set.
func FilterUntrustedHeaders(config *Config, req *http.Request) error {
	if config == nil {
		config = &Config{}
	}

	cf := isContainedIn(config.TrustedOrigins, getRemoteAddress(req.RemoteAddr))
	if !cf {
		// Address is not valid or the origin is not trusted; clear
		// all known proxy headers and return
		for _, header := range headersAll {
			req.Header.Del(header)
		}
		return nil
	}

	if config.CloudflareConnectingIP {
		req.Header.Set(headerCFConnectingIP, req.Header.Get(headerCFConnectingIP))
	} else {
		req.Header.Del(headerCFConnectingIP)
	}

	if config.TrueClientIP {
		req.Header.Set(headerTrueClientIP, req.Header.Get(headerTrueClientIP))
	} else {
		req.Header.Del(headerTrueClientIP)
	}

	if config.XRealIP {
		req.Header.Set(headerXRealIP, req.Header.Get(headerXRealIP))
	} else {
		req.Header.Del(headerXRealIP)
	}

	if config.XForwardedFor {
		req.Header.Set(headerXForwardedFor, req.Header.Get(headerXForwardedFor))
		req.Header.Set(headerXForwardedProto, req.Header.Get(headerXForwardedProto))
	} else {
		req.Header.Del(headerXForwardedFor)
		req.Header.Del(headerXForwardedProto)
	}

	return nil
}

// EnsureXForwardedFor ensures that the request has an X-Forwarded-For
// header. It uses the following logic:
//
// 1. If we have a direct connection (remoteAddr == proxyAddr), then
//    set it to remoteAddr
// 2. If we have a proxied connection (remoteAddr != proxyAddr) and
//    X-Forwarded-For doesn't begin with remoteAddr, then overwrite
//    it with remoteAddr,proxyAddr
// 3. If we have a proxied connection (remoteAddr != proxyAddr) and
//    X-Forwarded-For begins with remoteAddr, then append proxyAddr
//    to the original X-Forwarded-For header
// 4. If X-Forwarded-Proto is not set, then it will be set to "https"
//    if req.TLS != nil, otherwise it will be set to "http"
func EnsureXForwardedFor(req *http.Request) error {
	state := FromContext(req.Context())
	if state == nil {
		return xerrors.New("request does not contain realip.State; was it processed by realip.Middleware?")
	}

	remoteAddr := getRemoteAddress(req.RemoteAddr)
	if remoteAddr == nil {
		return xerrors.Errorf("failed to parse remote address: %s", remoteAddr)
	}

	proxyAddr := getRemoteAddress(state.OriginalRemoteAddr)
	if proxyAddr == nil {
		return xerrors.Errorf("failed to parse original address: %s", remoteAddr)
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

// State is the original state prior to modification by this middleware,
// useful for getting information about the connecting client if needed.
type State struct {
	// Config is the configuration applied in the middleware. Consider
	// this read-only and do not modify.
	Config *Config

	// OriginalRemoteAddr is the original RemoteAddr for the request.
	OriginalRemoteAddr string

	// OriginalTLS is the original TLS state for the request. Consider
	// this read-only and do not modify.
	OriginalTLS *tls.ConnectionState
}

type ctxKey struct{}

// FromContext retrieves the state from the given context.Context.
func FromContext(ctx context.Context) *State {
	state, ok := ctx.Value(ctxKey{}).(*State)
	if !ok {
		return nil
	}
	return state
}

// WithState returns an updated context containing the given State.
func WithState(ctx context.Context, state *State) context.Context {
	return context.WithValue(ctx, ctxKey{}, state)
}
