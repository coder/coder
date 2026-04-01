package aibridgeproxyd

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/aibridge"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

// Known AI provider hosts.
const (
	HostAnthropic = "api.anthropic.com"
	HostOpenAI    = "api.openai.com"
	HostCopilot   = "api.individual.githubcopilot.com"
)

const (
	// ProxyAuthRealm is the realm used in Proxy-Authenticate challenges.
	// The realm helps clients identify which credentials to use.
	ProxyAuthRealm = `"Coder AI Bridge Proxy"`
)

// proxyAuthRequiredMsg is the response body for 407 responses.
var proxyAuthRequiredMsg = []byte(http.StatusText(http.StatusProxyAuthRequired))

// loadMITMOnce ensures the MITM certificate is loaded exactly once.
// goproxy.GoproxyCa is a package-level global variable shared across all
// goproxy.ProxyHttpServer instances in the process. In tests, multiple proxy
// servers run in parallel, and without this guard they would race on writing
// to GoproxyCa. In production, only one server runs, so this has no impact.
var loadMITMOnce sync.Once

// blockedIPError is returned by checkBlockedIP and checkBlockedIPAndDial when
// a connection is blocked because the destination resolves to a private or
// reserved IP range. ConnectionErrHandler uses this type to return 403
// Forbidden instead of the generic 502 Bad Gateway, since the block is a
// policy decision rather than an upstream failure.
type blockedIPError struct {
	host string
	ip   net.IP
}

func (e *blockedIPError) Error() string {
	return fmt.Sprintf("connection to %s (%s) blocked: destination is in a private/reserved IP range", e.host, e.ip)
}

// blockedIPRanges defines private, reserved, and special-purpose IP ranges
// that are blocked by default to prevent connections to internal networks.
// Operators can selectively allow specific ranges via AllowedPrivateCIDRs.
var blockedIPRanges = func() []net.IPNet {
	cidrs := []string{
		"0.0.0.0/8",      // RFC 1122: "This" network
		"10.0.0.0/8",     // RFC 1918: Private-Use
		"100.64.0.0/10",  // RFC 6598: Shared Address Space (CGNAT / Tailscale)
		"127.0.0.0/8",    // RFC 1122: Loopback
		"169.254.0.0/16", // RFC 3927: Link-Local (cloud IMDS: AWS, GCP, Azure)
		"172.16.0.0/12",  // RFC 1918: Private-Use
		"192.0.0.0/24",   // RFC 6890: IETF Protocol Assignments
		"192.168.0.0/16", // RFC 1918: Private-Use
		"198.18.0.0/15",  // RFC 2544: Benchmarking
		"240.0.0.0/4",    // RFC 1112: Reserved for Future Use
		"::1/128",        // RFC 4291: Loopback
		"64:ff9b::/96",   // RFC 6052: NAT64 well-known prefix
		"64:ff9b:1::/48", // RFC 8215: NAT64 local-use prefix
		"2002::/16",      // RFC 3056: 6to4
		"fc00::/7",       // RFC 4193: Unique-Local
		"fe80::/10",      // RFC 4291: Link-Local Unicast

		// Note: intentionally excluded because Go's net.IPNet.Contains matches
		// all IPv4 addresses against this range due to internal IPv4-to-IPv6 mapping.
		// See https://github.com/golang/go/issues/51906
		// "::ffff:0:0/96",  // RFC 4291: IPv4-mapped IPv6
	}

	ranges := make([]net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid blocked CIDR %q: %v", cidr, err))
		}
		ranges = append(ranges, *ipNet)
	}
	return ranges
}()

// Server is the AI MITM (Man-in-the-Middle) proxy server.
// It is responsible for:
//   - intercepting HTTPS requests to AI providers
//   - decrypting requests using the configured MITM CA certificate
//   - forwarding requests to aibridged for processing
type Server struct {
	ctx                      context.Context
	logger                   slog.Logger
	proxy                    *goproxy.ProxyHttpServer
	httpServer               *http.Server
	listener                 net.Listener
	tlsEnabled               bool
	coderAccessURL           *url.URL
	aibridgeProviderFromHost func(host string) string
	// caCert is the PEM-encoded MITM CA certificate loaded during initialization.
	// This is served to clients who need to trust the proxy's generated certificates.
	caCert []byte
	// allowedPrivateRanges are CIDR ranges exempt from the blocked IP denylist.
	allowedPrivateRanges []net.IPNet
	// Metrics is the Prometheus metrics for the proxy. If nil, metrics are disabled.
	metrics *Metrics
}

// requestContext holds metadata propagated through the proxy request/response chain.
// It is stored in goproxy's ProxyCtx.UserData and enriched as the request progresses
// through the proxy handlers.
type requestContext struct {
	// ConnectSessionID is a unique identifier for this CONNECT session.
	// Set in authMiddleware during the CONNECT handshake.
	// Used to correlate requests/responses with their originating CONNECT.
	ConnectSessionID uuid.UUID
	// CoderToken is the authentication token extracted from Proxy-Authorization.
	// Set in authMiddleware during the CONNECT handshake.
	CoderToken string
	// Provider is the aibridge provider name.
	// Set in authMiddleware during the CONNECT handshake.
	Provider string
	// RequestID is a unique identifier for this request.
	// Set in handleRequest for MITM'd requests.
	// Sent to aibridged via custom header for cross-service correlation.
	RequestID uuid.UUID
}

// Options configures the AI Bridge Proxy server.
type Options struct {
	// ListenAddr is the address the proxy server will listen on.
	ListenAddr string
	// TLSCertFile is the path to the TLS certificate file for the proxy listener.
	TLSCertFile string
	// TLSKeyFile is the path to the TLS private key file for the proxy listener.
	TLSKeyFile string
	// CoderAccessURL is the URL of the Coder deployment where aibridged is running.
	// Requests to supported AI providers are forwarded here.
	CoderAccessURL string
	// MITMCertFile is the path to the CA certificate file used for MITM.
	MITMCertFile string
	// MITMKeyFile is the path to the CA private key file used for MITM.
	MITMKeyFile string
	// AllowedPorts is the list of ports allowed for CONNECT requests.
	// Defaults to ["80", "443"] if empty.
	AllowedPorts []string
	// CertStore is an optional certificate cache for MITM. If nil, a default
	// cache is created. Exposed for testing.
	CertStore goproxy.CertStorage
	// DomainAllowlist is the list of domains to intercept and route through AI Bridge.
	// Only requests to these domains will be MITM'd and forwarded to aibridged.
	// Requests to other domains will be tunneled directly without decryption.
	DomainAllowlist []string
	// AIBridgeProviderFromHost maps a hostname to a known aibridge provider name.
	// If nil, the default provider mapping is used.
	AIBridgeProviderFromHost func(host string) string
	// UpstreamProxy is the URL of an upstream HTTP proxy to chain tunneled
	// (non-allowlisted) requests through. If empty, tunneled requests connect
	// directly to their destinations.
	// Format: http://[user:pass@]host:port or https://[user:pass@]host:port
	UpstreamProxy string
	// UpstreamProxyCA is the path to a PEM-encoded CA certificate file to trust
	// for the upstream proxy's TLS connection. Only needed for HTTPS upstream
	// proxies with certificates not trusted by the system. If empty, the system
	// certificate pool is used.
	UpstreamProxyCA string
	// AllowedPrivateCIDRs is a list of CIDR ranges that are permitted even
	// though they fall within blocked private/reserved IP ranges. This allows
	// access to specific internal networks while keeping all other private
	// ranges blocked. If empty, all private ranges are blocked.
	AllowedPrivateCIDRs []string
	// Metrics is the prometheus metrics instance for recording proxy metrics.
	// If nil, metrics will not be recorded.
	Metrics *Metrics
}

func New(ctx context.Context, logger slog.Logger, opts Options) (*Server, error) {
	logger.Info(ctx, "initializing aibridgeproxyd")

	if opts.ListenAddr == "" {
		return nil, xerrors.New("listen address is required")
	}

	// Listener TLS requires both cert and key files. When set, the proxy listener
	// is served over HTTPS, otherwise it defaults to HTTP.
	if (opts.TLSCertFile != "") != (opts.TLSKeyFile != "") {
		return nil, xerrors.New("tls cert file and tls key file must both be set")
	}

	if strings.TrimSpace(opts.CoderAccessURL) == "" {
		return nil, xerrors.New("coder access URL is required")
	}
	coderAccessURL, err := url.Parse(opts.CoderAccessURL)
	if err != nil {
		return nil, xerrors.Errorf("invalid coder access URL %q: %w", opts.CoderAccessURL, err)
	}
	// Resolve the default port when not explicitly specified in the URL.
	coderAccessPort := coderAccessURL.Port()
	if coderAccessPort == "" {
		switch coderAccessURL.Scheme {
		case "https":
			coderAccessPort = "443"
		default:
			coderAccessPort = "80"
		}
	}
	coderAccessURL.Host = net.JoinHostPort(coderAccessURL.Hostname(), coderAccessPort)

	// MITM cert and key are required to intercept and decrypt HTTPS traffic.
	if opts.MITMCertFile == "" || opts.MITMKeyFile == "" {
		return nil, xerrors.New("MITM CA cert file and key file are required")
	}

	allowedPorts := opts.AllowedPorts
	if len(allowedPorts) == 0 {
		allowedPorts = []string{"80", "443"}
	}

	if len(opts.DomainAllowlist) == 0 {
		return nil, xerrors.New("domain allow list is required")
	}
	mitmHosts, err := convertDomainsToHosts(opts.DomainAllowlist, allowedPorts)
	if err != nil {
		return nil, xerrors.Errorf("invalid domain allowlist: %w", err)
	}
	if len(mitmHosts) == 0 {
		return nil, xerrors.New("domain allowlist is empty, at least one domain is required")
	}

	// Use custom provider mapper if provided, otherwise use default.
	aibridgeProviderFromHost := opts.AIBridgeProviderFromHost
	if aibridgeProviderFromHost == nil {
		aibridgeProviderFromHost = defaultAIBridgeProvider
	}

	// Validate that all allowlisted domains have correct aibridge provider mappings.
	for _, domain := range opts.DomainAllowlist {
		if aibridgeProviderFromHost(domain) == "" {
			return nil, xerrors.Errorf("domain %q is in allowlist but has no provider mapping", domain)
		}
	}

	// Parse configured exceptions to the blocked IP ranges.
	allowedPrivateRanges := make([]net.IPNet, 0, len(opts.AllowedPrivateCIDRs))
	for _, cidr := range opts.AllowedPrivateCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, xerrors.Errorf("invalid allowed private CIDR %q: %w", cidr, err)
		}
		allowedPrivateRanges = append(allowedPrivateRanges, *ipNet)
	}

	// Load the CA certificate for MITM.
	certPEM, err := loadMITMCertificate(opts.MITMCertFile, opts.MITMKeyFile)
	if err != nil {
		return nil, xerrors.Errorf("failed to load MITM certificate: %w", err)
	}

	proxy := goproxy.NewProxyHttpServer()

	// Cache generated leaf certificates to avoid expensive RSA key generation
	// and signing on every request to the same hostname.
	if opts.CertStore != nil {
		proxy.CertStore = opts.CertStore
	} else {
		proxy.CertStore = NewCertCache()
	}

	// Always set secure TLS defaults, overriding goproxy's default.
	// This ensures secure TLS connections for:
	// - HTTPS upstream proxy connections
	// - MITM'd requests if aibridge uses HTTPS
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, xerrors.Errorf("failed to load system certificate pool: %w", err)
	}

	srv := &Server{
		ctx:                      ctx,
		logger:                   logger,
		proxy:                    proxy,
		tlsEnabled:               opts.TLSCertFile != "",
		coderAccessURL:           coderAccessURL,
		aibridgeProviderFromHost: aibridgeProviderFromHost,
		caCert:                   certPEM,
		allowedPrivateRanges:     allowedPrivateRanges,
		metrics:                  opts.Metrics,
	}

	// Configure upstream proxy for tunneled (non-allowlisted) CONNECT requests.
	// Allowlisted domains are MITM'd and forwarded to aibridge directly,
	// bypassing the upstream proxy.
	if opts.UpstreamProxy != "" {
		upstreamURL, err := url.Parse(opts.UpstreamProxy)
		if err != nil {
			return nil, xerrors.Errorf("invalid upstream proxy URL %q: %w", opts.UpstreamProxy, err)
		}

		// Extract and validate upstream proxy authentication if provided.
		// The credentials are parsed once at startup and reused for all
		// tunneled CONNECT requests through the upstream proxy.
		var connectReqHandler func(*http.Request)
		if upstreamURL.User != nil {
			proxyAuth := makeProxyAuthHeader(upstreamURL.User)
			if proxyAuth == "" {
				return nil, xerrors.Errorf("upstream proxy URL %q has invalid credentials: both username and password are empty", opts.UpstreamProxy)
			}
			connectReqHandler = func(req *http.Request) {
				req.Header.Set("Proxy-Authorization", proxyAuth)
			}
		}

		// Set transport without Proxy to ensure MITM'd requests go directly to aibridge,
		// not through any upstream proxy.
		proxy.Tr = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    rootCAs,
			},
		}

		// Add custom CA certificate if provided (for corporate proxies with private CAs).
		// If no CA certificate is provided, the system certificate pool is used.
		if opts.UpstreamProxyCA != "" {
			if upstreamURL.Scheme == "https" {
				caCert, err := os.ReadFile(opts.UpstreamProxyCA)
				if err != nil {
					return nil, xerrors.Errorf("failed to read upstream proxy CA certificate from %q: %w", opts.UpstreamProxyCA, err)
				}
				if !rootCAs.AppendCertsFromPEM(caCert) {
					return nil, xerrors.Errorf("failed to parse upstream proxy CA certificate")
				}
				logger.Info(ctx, "configured upstream proxy CA certificate")
			} else {
				logger.Warn(ctx, "upstream proxy CA certificate is only used for HTTPS upstream proxies, ignoring",
					slog.F("upstream_scheme", upstreamURL.Scheme),
				)
			}
		}

		connectDialer := proxy.NewConnectDialToProxyWithHandler(opts.UpstreamProxy, connectReqHandler)
		proxy.ConnectDial = func(network, addr string) (net.Conn, error) {
			// Block CONNECT tunnels to private/reserved IP ranges.
			// addr is the CONNECT target, not the upstream proxy address.
			if err := srv.checkBlockedIP(ctx, addr); err != nil {
				return nil, err
			}
			return connectDialer(network, addr)
		}
	}

	// No upstream proxy configured: check private/reserved IPs and dial to the destination.
	if proxy.ConnectDial == nil {
		proxy.ConnectDial = func(network, addr string) (net.Conn, error) {
			return srv.checkBlockedIPAndDial(srv.ctx, network, addr)
		}
	}

	// Override goproxy's default CONNECT error handler to avoid leaking
	// internal error details to clients. Errors are still logged by the caller.
	// Policy blocks (private/reserved IP ranges) return 403 Forbidden; all
	// other dial failures return 502 Bad Gateway.
	proxy.ConnectionErrHandler = func(w io.Writer, _ *goproxy.ProxyCtx, err error) {
		status := http.StatusBadGateway
		var blocked *blockedIPError
		if errors.As(err, &blocked) {
			status = http.StatusForbidden
		}
		statusText := http.StatusText(status)
		_, _ = fmt.Fprintf(w, "HTTP/1.1 %d %s\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", status, statusText, len(statusText), statusText)
	}

	// Reject CONNECT requests to non-standard ports.
	proxy.OnRequest().HandleConnectFunc(srv.portMiddleware(allowedPorts))

	// Apply MITM with authentication only to allowlisted hosts.
	proxy.OnRequest(
		// Only CONNECT requests to these hosts will be intercepted and decrypted.
		// All other requests will be tunneled directly to their destination.
		goproxy.ReqHostIs(mitmHosts...),
	).HandleConnectFunc(
		// Extract Coder token from proxy authentication to forward to aibridged.
		srv.authMiddleware,
	)

	// Tunnel CONNECT requests for non-allowlisted domains directly to their destination.
	// goproxy calls handlers in registration order: this must come after the MITM handler
	// so it only handles requests that weren't matched by the allowlist.
	proxy.OnRequest().HandleConnectFunc(srv.tunneledMiddleware)

	// Handle decrypted requests: route to aibridged for known AI providers, or tunnel to original destination.
	proxy.OnRequest().DoFunc(srv.handleRequest)
	// Handle responses from aibridged.
	proxy.OnResponse().DoFunc(srv.handleResponse)

	// Create a plain HTTP listener by default. Port 0 is accepted and resolves
	// to a random available port, which is useful in tests to avoid conflicts.
	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to listen on %s: %w", opts.ListenAddr, err)
	}

	// Upgrade to HTTPS by wrapping the listener in TLS. The plain listener is
	// closed explicitly on error to avoid leaking the bound socket.
	if opts.TLSCertFile != "" {
		tlsCert, err := tls.LoadX509KeyPair(opts.TLSCertFile, opts.TLSKeyFile)
		if err != nil {
			_ = listener.Close()
			return nil, xerrors.Errorf("load listener TLS certificate: %w", err)
		}
		listener = tls.NewListener(listener, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{tlsCert},
		})
	}

	srv.listener = listener

	// Start HTTP server in background
	srv.httpServer = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info(ctx, "aibridgeproxyd configured",
		slog.F("listen_addr", listener.Addr().String()),
		slog.F("tls_listener_enabled", srv.tlsEnabled),
		slog.F("coder_access_url", coderAccessURL.String()),
		slog.F("domain_allowlist", mitmHosts),
		slog.F("upstream_proxy", opts.UpstreamProxy),
		slog.F("allowed_private_cidrs", opts.AllowedPrivateCIDRs),
	)

	go func() {
		logger.Info(ctx, "starting aibridgeproxyd server", slog.F("addr", listener.Addr().String()))
		if err := srv.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "aibridgeproxyd server error", slog.Error(err))
		}
	}()

	return srv, nil
}

// Addr returns the address the server is listening on.
// This is useful when the server was started with port 0.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// IsTLSListener reports whether the proxy listener is serving TLS.
func (s *Server) IsTLSListener() bool {
	return s.tlsEnabled
}

// CoderAccessURL returns the parsed Coder access URL with a normalized port.
func (s *Server) CoderAccessURL() *url.URL {
	return s.coderAccessURL
}

// Close gracefully shuts down the proxy server.
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	s.logger.Info(s.ctx, "closing aibridgeproxyd server")

	// Unregister metrics to clean up Prometheus registry.
	if s.metrics != nil {
		s.metrics.Unregister()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// loadMITMCertificate loads the MITM CA certificate and private key for MITM proxying.
// This function is safe to call concurrently - the certificate is only loaded once
// into the global goproxy.GoproxyCa variable.
// Returns the PEM-encoded certificate for serving to clients.
func loadMITMCertificate(certFile, keyFile string) ([]byte, error) {
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, xerrors.Errorf("load MITM CA certificate: %w", err)
	}

	if len(tlsCert.Certificate) == 0 {
		return nil, xerrors.Errorf("no certificates found")
	}

	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, xerrors.Errorf("parse MITM CA certificate: %w", err)
	}

	// Ensure that we only return the certificate and never any included private keys.
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: tlsCert.Certificate[0],
	})

	// Only protect the global assignment with sync.Once
	loadMITMOnce.Do(func() {
		goproxy.GoproxyCa = tls.Certificate{
			Certificate: tlsCert.Certificate,
			PrivateKey:  tlsCert.PrivateKey,
			Leaf:        x509Cert,
		}
	})

	return certPEM, nil
}

// portMiddleware is a CONNECT middleware that rejects requests to non-standard ports.
// This prevents the proxy from being used to tunnel to arbitrary services (SSH, databases, etc.).
func (s *Server) portMiddleware(allowedPorts []string) func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	allowed := make(map[string]bool, len(allowedPorts))
	for _, p := range allowedPorts {
		allowed[p] = true
	}

	return func(host string, _ *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		logger := s.logger.With(
			slog.F("host", host),
		)

		_, port, err := net.SplitHostPort(host)
		if err != nil {
			logger.Warn(s.ctx, "rejecting CONNECT with invalid host format",
				slog.Error(err),
			)
			return goproxy.RejectConnect, host
		}
		if port == "" {
			logger.Warn(s.ctx, "rejecting CONNECT with empty port")
			return goproxy.RejectConnect, host
		}

		logger = logger.With(slog.F("port", port))

		if !allowed[port] {
			logger.Warn(s.ctx, "rejecting CONNECT to non-allowed port")
			return goproxy.RejectConnect, host
		}

		return nil, ""
	}
}

// convertDomainsToHosts converts a list of domain names to host:port combinations.
// Each domain is combined with each allowed port.
// Returns an error if a domain includes a port that's not in the allowed ports list.
// For example, ["api.anthropic.com"] with ports ["443"] becomes ["api.anthropic.com:443"].
func convertDomainsToHosts(domains []string, allowedPorts []string) ([]string, error) {
	var hosts []string
	for _, domain := range domains {
		domain = strings.TrimSpace(strings.ToLower(domain))
		if domain == "" {
			continue
		}

		// If domain already includes a port, validate it's in the allowed list.
		if strings.Contains(domain, ":") {
			host, port, err := net.SplitHostPort(domain)
			if err != nil {
				return nil, xerrors.Errorf("invalid domain %q: %w", domain, err)
			}
			if !slices.Contains(allowedPorts, port) {
				return nil, xerrors.Errorf("invalid port in domain %q: port %s is not in allowed ports %v", domain, port, allowedPorts)
			}
			hosts = append(hosts, host+":"+port)
		} else {
			// Otherwise, combine domain with all allowed ports.
			for _, port := range allowedPorts {
				hosts = append(hosts, domain+":"+port)
			}
		}
	}
	return hosts, nil
}

// authMiddleware is a CONNECT middleware that extracts the Coder token from
// the Proxy-Authorization header and stores it in a requestContext in ctx.UserData
// for use by downstream handlers.
// Requests without valid credentials receive a 407 Proxy Authentication
// Required response with a challenge header, allowing clients to retry with
// credentials.
//
// Clients provide credentials by setting their HTTP Proxy as:
//
//	HTTPS_PROXY=http://ignored:<coder-token>@host:port
//
// The token is extracted from the password field of basic auth.
func (s *Server) authMiddleware(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	// Generate a unique connect session ID for this CONNECT request.
	// A UUID is used instead of goproxy's ctx.Session because ctx.Session is an
	// incrementing int64 that resets on process restart and is not globally unique.
	connectSessionID := uuid.New()

	logger := s.logger.With(
		slog.F("connect_id", connectSessionID.String()),
		slog.F("host", host),
	)

	// Determine the provider from the request hostname.
	provider := s.aibridgeProviderFromHost(ctx.Req.URL.Hostname())
	// This should never happen: startup validation ensures all allowlisted
	// domains have known aibridge provider mappings.
	if provider == "" {
		logger.Error(s.ctx, "rejecting CONNECT request with no provider mapping")
		return goproxy.RejectConnect, host
	}

	logger = logger.With(
		slog.F("provider", provider),
	)

	proxyAuth := ctx.Req.Header.Get("Proxy-Authorization")
	coderToken := extractCoderTokenFromProxyAuth(proxyAuth)

	// Reject requests for both missing and invalid credentials
	if coderToken == "" {
		hasAuth := proxyAuth != ""
		logger.Warn(s.ctx, "rejecting CONNECT request",
			slog.F("reason", map[bool]string{true: "invalid_credentials", false: "missing_credentials"}[hasAuth]),
		)

		// Send 407 challenge to allow clients to retry with credentials.
		ctx.Resp = newProxyAuthRequiredResponse(ctx.Req) //nolint:bodyclose // Response body is written by goproxy to the client
		return goproxy.RejectConnect, host
	}

	// Store the request context in UserData for downstream handlers.
	// goproxy propagates UserData to subsequent request/response contexts
	// for decrypted requests within this MITM session.
	ctx.UserData = &requestContext{
		ConnectSessionID: connectSessionID,
		CoderToken:       coderToken,
		Provider:         provider,
	}

	logger.Debug(s.ctx, "request CONNECT authenticated")

	// Record successful MITM CONNECT session establishment.
	if s.metrics != nil {
		s.metrics.ConnectSessionsTotal.WithLabelValues(RequestTypeMITM).Inc()
	}

	return goproxy.MitmConnect, host
}

// makeProxyAuthHeader creates a Proxy-Authorization header value from URL user info.
//
// Valid formats:
//   - username:password -> Basic auth with both credentials
//   - username: or username -> Basic auth with username only (empty password)
//   - :password -> Basic auth with empty username (token-based proxies)
//
// Returns empty string when both username and password are empty.
func makeProxyAuthHeader(userInfo *url.Userinfo) string {
	if userInfo == nil {
		return ""
	}

	username := userInfo.Username()
	password, _ := userInfo.Password()

	// Reject only when both username and password are empty (no credentials).
	if username == "" && password == "" {
		return ""
	}

	return "Basic " + base64.StdEncoding.EncodeToString([]byte(userInfo.String()))
}

// extractCoderTokenFromProxyAuth extracts the Coder token from the
// Proxy-Authorization header. The token is expected to be in the password
// field of basic auth: "Basic base64(username:token)".
//
// Returns empty string if no valid token is found.
func extractCoderTokenFromProxyAuth(proxyAuth string) string {
	if proxyAuth == "" {
		return ""
	}

	// Expected format: "Basic base64(username:password)"
	// Auth scheme is case-insensitive per RFC 7235.
	parts := strings.Fields(proxyAuth)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return ""
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	// Format: "username:password", password is the Coder token.
	// Username is ignored and can be any value.
	credentials := strings.SplitN(string(decoded), ":", 2)
	if len(credentials) != 2 {
		return ""
	}

	return credentials[1]
}

// extractCoderTokenFromBearerAuth extracts the bearer token from an
// Authorization header. Returns empty string if the header is not a
// valid "Bearer <token>" value.
func extractCoderTokenFromBearerAuth(auth string) string {
	parts := strings.Fields(auth)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// newProxyAuthRequiredResponse creates a 407 Proxy Authentication Required
// response with the appropriate challenge header. This is used both during
// CONNECT handling and for decrypted requests missing authentication.
//
// Note: based on github.com/elazarl/goproxy/ext/auth.BasicUnauthorized, inlined
// here to avoid adding a dependency on the ext module.
func newProxyAuthRequiredResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusProxyAuthRequired,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Request:    req,
		Header: http.Header{
			"Proxy-Authenticate": []string{"Basic realm=" + ProxyAuthRealm},
			"Proxy-Connection":   []string{"close"},
		},
		Body:          io.NopCloser(bytes.NewBuffer(proxyAuthRequiredMsg)),
		ContentLength: int64(len(proxyAuthRequiredMsg)),
	}
}

// defaultAIBridgeProvider maps the request host to the aibridge provider name.
//   - Known AI providers return their provider name, used to route to the
//     corresponding aibridge endpoint.
//   - Unknown hosts return empty string and are passed through directly.
func defaultAIBridgeProvider(host string) string {
	switch strings.ToLower(host) {
	case HostAnthropic:
		return aibridge.ProviderAnthropic
	case HostOpenAI:
		return aibridge.ProviderOpenAI
	case HostCopilot:
		return aibridge.ProviderCopilot
	case agplaibridge.HostCopilotBusiness:
		return agplaibridge.ProviderCopilotBusiness
	case agplaibridge.HostCopilotEnterprise:
		return agplaibridge.ProviderCopilotEnterprise
	case agplaibridge.HostChatGPT:
		return agplaibridge.ProviderChatGPT
	default:
		return ""
	}
}

// tunneledMiddleware is a CONNECT middleware that handles tunneled (non-allowlisted)
// connections. These connections are not MITM'd and are tunneled directly to their
// destination. This middleware records metrics for tunneled CONNECT sessions.
func (s *Server) tunneledMiddleware(host string, _ *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	// Record tunneled CONNECT session establishment.
	if s.metrics != nil {
		s.metrics.ConnectSessionsTotal.WithLabelValues(RequestTypeTunneled).Inc()
	}

	// Return OkConnect to allow the tunnel to be established.
	// goproxy will create a tunnel between the client and the destination.
	return goproxy.OkConnect, host
}

// isBlockedIP reports whether the given IP is in a blocked private/reserved range
// and not exempted by AllowedPrivateCIDRs or the Coder access URL hostname.
func (s *Server) isBlockedIP(ip net.IP, hostname string, port string) bool {
	// Always allow the Coder access URL hostname+port so the proxy doesn't
	// block connections to its own deployment. Hostname-based (not IP-based)
	// to handle dynamic IPs (DNS changes, load balancers, k8s rescheduling).
	// The port is normalized at startup to handle URLs without explicit ports.
	if strings.EqualFold(hostname, s.coderAccessURL.Hostname()) && port == s.coderAccessURL.Port() {
		return false
	}

	for _, blocked := range blockedIPRanges {
		if blocked.Contains(ip) {
			for _, allowed := range s.allowedPrivateRanges {
				if allowed.Contains(ip) {
					return false
				}
			}
			return true
		}
	}
	return false
}

// checkBlockedIP resolves the destination address and returns an error if any
// resolved IP falls within a blocked range. Used in the upstream proxy path,
// where the actual dial is delegated to the upstream proxy dialer.
//
// Note: this only prevents DNS rebinding on aibridgeproxyd, not on upstream proxies.
// The upstream proxy performs its own DNS resolution when dialing, so there is
// a window between this check and the actual connection.
func (s *Server) checkBlockedIP(ctx context.Context, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return xerrors.Errorf("invalid address %q: %w", addr, err)
	}

	// DNS resolution relies on the OS resolver. We avoid application-level
	// caching to keep the implementation simple. DNS caching behavior depends
	// on the OS resolver.
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return xerrors.Errorf("failed to resolve %q: %w", host, err)
	}

	for _, ip := range ips {
		if s.isBlockedIP(ip.IP, host, port) {
			s.logger.Warn(ctx, "blocking connection to private/reserved IP",
				slog.F("hostname", host),
				slog.F("port", port),
				slog.F("resolved_ip", ip.IP.String()),
			)
			return &blockedIPError{host: host, ip: ip.IP}
		}
	}
	return nil
}

// checkBlockedIPAndDial dials the destination address, blocking connections to
// private/reserved IPs. Used for tunneled CONNECT requests when no upstream
// proxy is configured.
func (s *Server) checkBlockedIPAndDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, xerrors.Errorf("invalid address %q: %w", addr, err)
	}

	// DNS resolution is handled by Go's DialContext using the OS resolver.
	// We avoid application-level DNS caching to keep the implementation
	// simple. DNS caching behavior depends on the OS resolver.
	dialer := net.Dialer{
		// ControlContext fires after DNS resolution and before each TCP dial,
		// receiving the resolved IP:port. The resolved address is always an IP,
		// so there is no risk of DNS rebinding between validation and the dial.
		ControlContext: func(ctx context.Context, _, address string, _ syscall.RawConn) error {
			resolvedIP, _, err := net.SplitHostPort(address)
			if err != nil {
				return xerrors.Errorf("invalid resolved address %q: %w", address, err)
			}

			ip := net.ParseIP(resolvedIP)
			if ip == nil {
				return xerrors.Errorf("invalid resolved IP %q", resolvedIP)
			}

			if s.isBlockedIP(ip, host, port) {
				s.logger.Warn(ctx, "blocking connection to private/reserved IP",
					slog.F("hostname", host),
					slog.F("port", port),
					slog.F("resolved_ip", ip.String()),
				)
				return &blockedIPError{host: host, ip: ip}
			}
			return nil
		},
	}
	return dialer.DialContext(ctx, network, addr)
}

// handleRequest intercepts HTTP requests after MITM decryption.
//   - Requests to known AI providers are rewritten to point at aibridged.
//     In centralized mode the Coder token is already in the
//     Authorization header. For BYOK clients that cannot set custom
//     headers, the proxy injects the BYOK header.
//   - Unknown hosts are passed through to the original upstream.
func (s *Server) handleRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	originalPath := req.URL.Path

	// Get the request context stored during CONNECT.
	reqCtx, _ := ctx.UserData.(*requestContext)
	if reqCtx == nil {
		s.logger.Warn(s.ctx, "rejecting request with missing context",
			slog.F("host", req.Host),
			slog.F("method", req.Method),
			slog.F("path", originalPath),
		)

		resp := goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusProxyAuthRequired, "Proxy authentication required")
		resp.Header.Set("Proxy-Authenticate", `Basic realm="Coder AI Bridge Proxy"`)
		return req, resp
	}

	if reqCtx.Provider == "" {
		// This should never happen: startup validation ensures all allowlisted
		// domains have known aibridge provider mappings.
		// The request is MITM'd (decrypted) but since there is no mapping,
		// there is no known route to aibridge.
		// Log error and forward to the original destination as a fallback.
		s.logger.Error(s.ctx, "decrypted request has no provider mapping, passing through",
			slog.F("connect_id", reqCtx.ConnectSessionID.String()),
			slog.F("host", req.Host),
			slog.F("method", req.Method),
			slog.F("path", originalPath),
		)
		return req, nil
	}

	// Generate a unique request ID for this request.
	// This ID is sent to aibridged for cross-service log correlation.
	reqCtx.RequestID = uuid.New()

	logger := s.logger.With(
		slog.F("connect_id", reqCtx.ConnectSessionID.String()),
		slog.F("request_id", reqCtx.RequestID.String()),
		slog.F("host", req.Host),
		slog.F("method", req.Method),
		slog.F("path", originalPath),
		slog.F("provider", reqCtx.Provider),
	)

	// Reject unauthenticated requests to AI providers.
	if reqCtx.CoderToken == "" {
		logger.Warn(s.ctx, "rejecting unauthenticated request to AI provider")
		// Describe to the client how to authenticate with the proxy.
		return req, newProxyAuthRequiredResponse(req)
	}

	// Rewrite the request to point to aibridged.
	if s.coderAccessURL == nil || s.coderAccessURL.String() == "" {
		logger.Error(s.ctx, "coderAccessURL is not configured")
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, "Proxy misconfigured")
	}

	aiBridgeURL, err := url.JoinPath(s.coderAccessURL.String(), "api/v2/aibridge", reqCtx.Provider, originalPath)
	if err != nil {
		logger.Error(s.ctx, "failed to build aibridged URL", slog.Error(err))
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, "Failed to build AI Bridge URL")
	}

	aiBridgeParsedURL, err := url.Parse(aiBridgeURL)
	if err != nil {
		logger.Error(s.ctx, "failed to parse aibridged URL", slog.Error(err))
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, "Failed to parse AI Bridge URL")
	}

	// Preserve query parameters from the original request.
	// Both URL and Host must be set for the request to be properly routed.
	aiBridgeParsedURL.RawQuery = req.URL.RawQuery
	req.URL = aiBridgeParsedURL
	req.Host = aiBridgeParsedURL.Host

	injectBYOKHeaderIfNeeded(req.Header, reqCtx.CoderToken)

	// Set request ID header to correlate requests between aibridgeproxyd and aibridged.
	req.Header.Set(agplaibridge.HeaderCoderRequestID, reqCtx.RequestID.String())

	logger.Info(s.ctx, "routing MITM request to aibridged",
		slog.F("aibridged_url", aiBridgeParsedURL.String()),
	)

	// Record MITM request handling.
	if s.metrics != nil {
		s.metrics.MITMRequestsTotal.WithLabelValues(reqCtx.Provider).Inc()
		s.metrics.InflightMITMRequests.WithLabelValues(reqCtx.Provider).Inc()
	}

	return req, nil
}

// injectBYOKHeaderIfNeeded sets HeaderCoderToken when the
// Authorization header carries a bearer token that differs from the
// Coder token, indicating the client is using its own LLM
// credentials. Clients that can set custom headers
// do this themselves; this handles clients that cannot.
//
// In centralized mode, Authorization carries the Coder token
// itself, so aibridged discovers it via ExtractAuthToken
// without any extra header.
func injectBYOKHeaderIfNeeded(header http.Header, coderToken string) {
	// Don’t overwrite the header if it’s already set.
	if header.Get(agplaibridge.HeaderCoderToken) != "" {
		return
	}

	bearer := extractCoderTokenFromBearerAuth(header.Get("Authorization"))
	if bearer != "" && bearer != coderToken {
		header.Set(agplaibridge.HeaderCoderToken, coderToken)
	}
}

// handleResponse handles responses received from aibridged.
// This is only called for MITM'd requests (allowlisted domains routed through aibridged).
// Tunneled requests (non-allowlisted domains) bypass this handler entirely.
func (s *Server) handleResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	if resp == nil {
		return nil
	}

	reqCtx, _ := ctx.UserData.(*requestContext)
	connectSessionID := uuid.Nil
	requestID := uuid.Nil
	provider := ""
	if reqCtx != nil {
		connectSessionID = reqCtx.ConnectSessionID
		requestID = reqCtx.RequestID
		provider = reqCtx.Provider
	}

	logger := s.logger.With(
		slog.F("connect_id", connectSessionID.String()),
		slog.F("request_id", requestID.String()),
		slog.F("provider", provider),
		slog.F("status", resp.StatusCode),
	)

	switch {
	case resp.StatusCode >= http.StatusInternalServerError:
		logger.Error(s.ctx, "received error response from aibridged")
	case resp.StatusCode >= http.StatusBadRequest:
		logger.Warn(s.ctx, "received error response from aibridged")
	default:
		logger.Debug(s.ctx, "received response from aibridged")
	}

	if s.metrics != nil && provider != "" {
		// Decrement inflight requests gauge now that the request is complete.
		s.metrics.InflightMITMRequests.WithLabelValues(provider).Dec()

		// Record response by status code.
		s.metrics.MITMResponsesTotal.WithLabelValues(strconv.Itoa(resp.StatusCode), provider).Inc()
	}

	return resp
}

// Handler returns an HTTP handler for the AI Bridge Proxy's HTTP endpoints.
// This is separate from the proxy server itself and is used by coderd to
// serve endpoints like the CA certificate.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/ca-cert.pem", s.serveCACert)
	return r
}

// serveCACert is an HTTP handler that serves the CA certificate used for MITM
// proxying. Clients need this certificate to trust the proxy's intercepted
// connections. The certificate was validated during server initialization.
func (s *Server) serveCACert(rw http.ResponseWriter, _ *http.Request) {
	if len(s.caCert) == 0 {
		http.Error(rw, "MITM CA certificate not configured", http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "application/x-pem-file")
	rw.Header().Set("Content-Disposition", "attachment; filename=ca-cert.pem")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(s.caCert)
}
