package aibridgeproxyd

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
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
	// HeaderAIBridgeRequestID is the header used to correlate requests
	// between aibridgeproxyd and aibridged.
	HeaderAIBridgeRequestID = "X-AI-Bridge-Request-Id"
	// ProxyAuthRealm is the realm used in Proxy-Authenticate challenges.
	// The realm helps clients identify which credentials to use.
	ProxyAuthRealm = `"Coder AI Bridge Proxy"`
)

// proxyAuthRequiredMsg is the response body for 407 responses.
var proxyAuthRequiredMsg = []byte(http.StatusText(http.StatusProxyAuthRequired))

// loadMitmOnce ensures the MITM certificate is loaded exactly once.
// goproxy.GoproxyCa is a package-level global variable shared across all
// goproxy.ProxyHttpServer instances in the process. In tests, multiple proxy
// servers run in parallel, and without this guard they would race on writing
// to GoproxyCa. In production, only one server runs, so this has no impact.
var loadMitmOnce sync.Once

// pipeListener is a net.Listener that creates in-memory pipe connections on demand.
// Used for serving HTTP handlers over in-memory pipes without network overhead.
type pipeListener struct {
	connCh chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		connCh: make(chan net.Conn),
		closed: make(chan struct{}),
	}
}

// Dial creates a new pipe connection and returns the client side.
// The server side is sent to Accept() to be handled by the HTTP server.
func (l *pipeListener) Dial() (net.Conn, error) {
	clientConn, serverConn := net.Pipe()

	select {
	case l.connCh <- serverConn:
		return clientConn, nil
	case <-l.closed:
		_ = clientConn.Close()
		_ = serverConn.Close()
		return nil, net.ErrClosed
	}
}

// Accept returns the next incoming connection (server side of pipe).
func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

// Close closes the listener.
func (l *pipeListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
	})
	return nil
}

// Addr returns a dummy address.
func (l *pipeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

// Server is the AI MITM (Man-in-the-Middle) proxy server.
// It is responsible for:
//   - intercepting HTTPS requests to AI providers
//   - decrypting requests using the configured CA certificate
//   - forwarding requests to aibridged for processing
type Server struct {
	ctx                      context.Context
	logger                   slog.Logger
	proxy                    *goproxy.ProxyHttpServer
	httpServer               *http.Server
	listener                 net.Listener
	aibridgeListener         *pipeListener
	aibridgeHTTPServer       *http.Server
	aibridgeProviderFromHost func(host string) string
	// caCert is the PEM-encoded CA certificate loaded during initialization.
	// This is served to clients who need to trust the proxy.
	caCert []byte
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
	// AIBridgeHandler is the HTTP handler for AI Bridge requests.
	// Requests are routed directly to this handler via in-memory HTTP calls.
	AIBridgeHandler http.Handler
	// CertFile is the path to the CA certificate file used for MITM.
	CertFile string
	// KeyFile is the path to the CA private key file used for MITM.
	KeyFile string
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
}

func New(ctx context.Context, logger slog.Logger, opts Options) (*Server, error) {
	logger.Info(ctx, "initializing aibridgeproxyd")

	if opts.ListenAddr == "" {
		return nil, xerrors.New("listen address is required")
	}

	if opts.AIBridgeHandler == nil {
		return nil, xerrors.New("AIBridgeHandler is required")
	}

	if opts.CertFile == "" || opts.KeyFile == "" {
		return nil, xerrors.New("cert file and key file are required")
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

	// Load CA certificate for MITM
	certPEM, err := loadMitmCertificate(opts.CertFile, opts.KeyFile)
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
	// This ensures secure TLS connections for HTTPS upstream proxy connections.
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, xerrors.Errorf("failed to load system certificate pool: %w", err)
	}

	// Create persistent in-memory server for aibridge requests.
	// This server handles all aibridge requests via in-memory pipes.
	aibridgeListener := newPipeListener()

	// Configure transport with custom DialContext for in-memory aibridge routing.
	// This transport is ONLY used for MITM'd requests to allowlisted domains.
	// Non-allowlisted domains are tunneled via ConnectDial and never use this transport.
	proxy.Tr = &http.Transport{
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			// Route requests to aibridge.internal through in-memory handler.
			if strings.Contains(addr, "aibridge.internal") {
				return aibridgeListener.Dial()
			}

			// If we reach here, something is wrong. Only MITM'd aibridge requests
			// should use this transport. Non-allowlisted domains should be tunneled
			// via ConnectDial (for HTTPS/CONNECT) and not reach this point.
			// Direct HTTP requests (non-CONNECT) are not supported by aibridgeproxyd.
			return nil, xerrors.Errorf("aibridgeproxyd does not support proxying requests to %s (only MITM'd AI provider requests are supported)", addr)
		},
	}

	// Configure upstream proxy for tunneled (non-allowlisted) requests.
	// This only affects CONNECT requests to domains not in the allowlist.
	// MITM'd requests (allowlisted domains) are handled by aibridge directly
	// via in-memory handler, not through the upstream proxy.
	if opts.UpstreamProxy != "" {
		upstreamURL, err := url.Parse(opts.UpstreamProxy)
		if err != nil {
			return nil, xerrors.Errorf("invalid upstream proxy URL %q: %w", opts.UpstreamProxy, err)
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

		// Configure tunneled CONNECT requests to go through upstream proxy.
		proxy.ConnectDial = proxy.NewConnectDialToProxy(opts.UpstreamProxy)
	} else {
		// No upstream proxy - use direct dialing for tunneled CONNECT requests.
		// This ensures tunneled requests don't go through proxy.Tr.
		proxy.ConnectDial = func(network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{}
			return dialer.Dial(network, addr)
		}
	}

	srv := &Server{
		ctx:                      ctx,
		logger:                   logger,
		proxy:                    proxy,
		aibridgeListener:         aibridgeListener,
		aibridgeProviderFromHost: aibridgeProviderFromHost,
		caCert:                   certPEM,
	}

	// Start persistent HTTP server for aibridge requests.
	// Wrap the handler with StripPrefix since it expects paths without /api/v2/aibridge.
	srv.aibridgeHTTPServer = &http.Server{
		Handler: http.StripPrefix("/api/v2/aibridge", opts.AIBridgeHandler),
	}
	go func() {
		logger.Debug(ctx, "starting in-memory aibridge HTTP server")
		if err := srv.aibridgeHTTPServer.Serve(aibridgeListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "aibridge HTTP server error", slog.Error(err))
		}
	}()

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

	// Handle decrypted requests: route to aibridged for known AI providers, or tunnel to original destination.
	proxy.OnRequest().DoFunc(srv.handleRequest)
	// Handle responses from aibridged.
	proxy.OnResponse().DoFunc(srv.handleResponse)

	// Create listener first so we can get the actual address.
	// This is useful in tests where port 0 is used to avoid conflicts.
	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to listen on %s: %w", opts.ListenAddr, err)
	}
	srv.listener = listener

	// Start HTTP server in background
	srv.httpServer = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info(ctx, "aibridgeproxyd configured",
		slog.F("listen_addr", listener.Addr().String()),
		slog.F("domain_allowlist", mitmHosts),
		slog.F("upstream_proxy", opts.UpstreamProxy),
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

// Close gracefully shuts down the proxy server.
func (s *Server) Close() error {
	s.logger.Info(s.ctx, "closing aibridgeproxyd server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Close aibridge listener first to stop accepting new connections.
	if s.aibridgeListener != nil {
		_ = s.aibridgeListener.Close()
	}

	// Shutdown aibridge HTTP server.
	if s.aibridgeHTTPServer != nil {
		if err := s.aibridgeHTTPServer.Shutdown(ctx); err != nil {
			s.logger.Warn(s.ctx, "error shutting down aibridge HTTP server", slog.Error(err))
		}
	}

	// Shutdown main proxy HTTP server.
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}

	return nil
}

// loadMitmCertificate loads the CA certificate and private key for MITM proxying.
// This function is safe to call concurrently - the certificate is only loaded once
// into the global goproxy.GoproxyCa variable.
// Returns the PEM-encoded certificate for serving to clients.
func loadMitmCertificate(certFile, keyFile string) ([]byte, error) {
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, xerrors.Errorf("load CA certificate: %w", err)
	}

	if len(tlsCert.Certificate) == 0 {
		return nil, xerrors.Errorf("no certificates found")
	}

	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, xerrors.Errorf("parse CA certificate: %w", err)
	}

	// Ensure that we only return the certificate and never any included private keys.
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: tlsCert.Certificate[0],
	})

	// Only protect the global assignment with sync.Once
	loadMitmOnce.Do(func() {
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

	return goproxy.MitmConnect, host
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
	default:
		return ""
	}
}

// handleRequest intercepts HTTP requests after MITM decryption.
//   - Requests to known AI providers are rewritten to aibridge paths with authentication.
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

	// Rewrite the request to point to aibridge via in-memory routing.
	// The custom DialContext will intercept connections to aibridge.internal
	// and route them through in-memory pipes to the handler.
	req.URL.Scheme = "http"
	req.URL.Host = "aibridge.internal"
	req.URL.Path = path.Join("/api/v2/aibridge", reqCtx.Provider, originalPath)
	// Preserve query parameters from the original request.
	// req.URL.RawQuery already contains the original query parameters.

	// Set X-Coder-Token header for aibridged authentication.
	// Using a separate header preserves the original request headers,
	// which are forwarded to upstream providers.
	req.Header.Set(agplaibridge.HeaderCoderAuth, reqCtx.CoderToken)

	// Set custom header for cross-service log correlation.
	// This allows correlating aibridgeproxyd logs with aibridged logs.
	req.Header.Set(HeaderAIBridgeRequestID, reqCtx.RequestID.String())

	logger.Info(s.ctx, "routing request to aibridge handler",
		slog.F("aibridge_path", req.URL.Path),
	)

	// Return nil response - let goproxy's transport make the call.
	// The response will be received in handleResponse.
	return req, nil
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

	s.logger.Debug(s.ctx, "received response from aibridged",
		slog.F("connect_id", connectSessionID.String()),
		slog.F("request_id", requestID.String()),
		slog.F("status", resp.StatusCode),
		slog.F("provider", provider),
	)

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
		http.Error(rw, "CA certificate not configured", http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "application/x-pem-file")
	rw.Header().Set("Content-Disposition", "attachment; filename=ca-cert.pem")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(s.caCert)
}
