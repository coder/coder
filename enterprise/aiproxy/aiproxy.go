package aiproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/elazarl/goproxy"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// goproxyLogger adapts slog.Logger to goproxy's Logger interface.
type goproxyLogger struct {
	ctx    context.Context
	logger slog.Logger
}

func (l *goproxyLogger) Printf(format string, v ...any) {
	// goproxy's format includes "[%03d] " session prefix and trailing newline.
	// We strip the newline since slog adds its own.
	msg := fmt.Sprintf(format, v...)
	msg = strings.TrimSuffix(msg, "\n")
	l.logger.Debug(l.ctx, msg)
}

// certCache implements goproxy.CertStorage to cache generated leaf certificates.
type certCache struct {
	mu    sync.RWMutex
	certs map[string]*tls.Certificate
}

func (c *certCache) Fetch(hostname string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	c.mu.RLock()
	cert, ok := c.certs[hostname]
	c.mu.RUnlock()
	if ok {
		return cert, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if cert, ok := c.certs[hostname]; ok {
		return cert, nil
	}

	cert, err := gen()
	if err != nil {
		return nil, err
	}
	c.certs[hostname] = cert
	return cert, nil
}

type Server struct {
	ctx            context.Context
	logger         slog.Logger
	proxy          *goproxy.ProxyHttpServer
	httpServer     *http.Server
	coderAccessURL *url.URL
}

type Options struct {
	ListenAddr     string
	CertFile       string
	KeyFile        string
	CoderAccessURL string
	// UpstreamProxy is the URL of an upstream HTTP proxy to chain requests through.
	// If empty, requests are made directly to targets.
	// Format: http://[user:pass@]host:port or https://[user:pass@]host:port
	UpstreamProxy string
	// UpstreamProxyCACert is the PEM-encoded CA certificate to trust for the upstream
	// proxy's TLS interception. Required when chaining through an SSL-bumping proxy.
	UpstreamProxyCACert []byte
}

func New(ctx context.Context, logger slog.Logger, opts Options) (*Server, error) {
	logger.Info(ctx, "initializing AI proxy server")

	// Load CA certificate for MITM
	if err := loadMitmCertificate(opts.CertFile, opts.KeyFile); err != nil {
		return nil, xerrors.Errorf("failed to load MITM certificate: %w", err)
	}

	// Parse coderAccessURL once at startup - invalid URL is a fatal config error
	coderAccessURL, err := url.Parse(opts.CoderAccessURL)
	if err != nil {
		return nil, xerrors.Errorf("invalid CoderAccessURL %q: %w", opts.CoderAccessURL, err)
	}

	srv := &Server{
		ctx:            ctx,
		logger:         logger,
		coderAccessURL: coderAccessURL,
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true
	// proxy.Logger = &goproxyLogger{ctx: ctx, logger: logger.Named("goproxy")}
	proxy.CertStore = &certCache{certs: make(map[string]*tls.Certificate)}

	// Configure upstream proxy for chaining if specified
	if opts.UpstreamProxy != "" {
		upstreamURL, err := url.Parse(opts.UpstreamProxy)
		if err != nil {
			return nil, xerrors.Errorf("invalid UpstreamProxy URL %q: %w", opts.UpstreamProxy, err)
		}
		logger.Info(ctx, "configuring upstream proxy", slog.F("upstream", upstreamURL.Host))

		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		// Add upstream proxy CA to trusted roots if provided
		if len(opts.UpstreamProxyCACert) > 0 {
			rootCAs, err := x509.SystemCertPool()
			if err != nil {
				rootCAs = x509.NewCertPool()
			}
			if !rootCAs.AppendCertsFromPEM(opts.UpstreamProxyCACert) {
				return nil, xerrors.Errorf("failed to parse upstream proxy CA certificate")
			}
			tlsConfig.RootCAs = rootCAs
			logger.Info(ctx, "configured upstream proxy CA certificate")
		}

		// Configure HTTP transport to use upstream proxy
		proxy.Tr = &http.Transport{
			Proxy:           http.ProxyURL(upstreamURL),
			TLSClientConfig: tlsConfig,
		}

		// Configure CONNECT requests to go through upstream proxy
		proxy.ConnectDial = proxy.NewConnectDialToProxy(opts.UpstreamProxy)
	}

	// Custom MITM handler that extracts auth and rejects unauthenticated requests.
	// The token is stored in ctx.UserData which goproxy propagates to subsequent
	// request contexts for decrypted requests within this MITM session.
	mitmWithAuth := goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		proxyAuth := ctx.Req.Header.Get("Proxy-Authorization")
		coderToken := extractCoderTokenFromProxyAuth(proxyAuth)

		// Reject unauthenticated or invalid auth requests - proxy is a protected service
		if coderToken == "" {
			hasAuth := proxyAuth != ""
			srv.logger.Warn(srv.ctx, "rejecting connect request",
				slog.F("host", host),
				slog.F("reason", map[bool]string{true: "invalid_auth", false: "missing_auth"}[hasAuth]),
			)
			return goproxy.RejectConnect, host
		}

		// Store token in UserData - goproxy copies this to subsequent request contexts
		ctx.UserData = coderToken

		return goproxy.MitmConnect, host
	})

	// Apply MITM only to allowlisted AI provider hosts
	proxy.OnRequest(goproxy.ReqHostIs(
		"api.anthropic.com:443",
		"api.openai.com:443",
		"ampcode.com:443",
	)).HandleConnect(mitmWithAuth)

	// Request handler for decrypted HTTPS traffic
	proxy.OnRequest().DoFunc(srv.requestHandler)

	// Response handler
	proxy.OnResponse().DoFunc(srv.responseHandler)

	srv.proxy = proxy

	// Start HTTP server in background
	srv.httpServer = &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info(ctx, "starting AI proxy", slog.F("addr", opts.ListenAddr))
		if err := srv.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "proxy server error", slog.Error(err))
		}
	}()

	return srv, nil
}

// loadMitmCertificate loads the CA certificate for MITM into goproxy.
func loadMitmCertificate(certFile, keyFile string) error {
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return xerrors.Errorf("failed to load x509 keypair: %w", err)
	}

	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return xerrors.Errorf("failed to parse certificate: %w", err)
	}

	goproxy.GoproxyCa = tls.Certificate{
		Certificate: tlsCert.Certificate,
		PrivateKey:  tlsCert.PrivateKey,
		Leaf:        x509Cert,
	}

	return nil
}

// extractCoderTokenFromProxyAuth extracts the Coder session token from the
// Proxy-Authorization header. The token is expected to be in the password
// field of basic auth: "Basic base64(ignored:token)"
// Returns empty string if no valid token is found.
func extractCoderTokenFromProxyAuth(proxyAuth string) string {
	if proxyAuth == "" {
		return ""
	}

	// Expected format: "Basic base64(username:password)"
	// Auth scheme is case-insensitive per HTTP spec
	parts := strings.Fields(proxyAuth)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Basic") {
		return ""
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	// Format: "username:password" - we use password as the Coder token
	credentials := strings.SplitN(string(decoded), ":", 2)
	if len(credentials) != 2 {
		return ""
	}

	return credentials[1]
}

// canonicalHost strips the port from a host:port string and lowercases it.
func canonicalHost(h string) string {
	if i := strings.IndexByte(h, ':'); i != -1 {
		h = h[:i]
	}
	return strings.ToLower(h)
}

// providerFromHost maps the request host to the aibridge provider name.
// All requests through the proxy for known AI providers are routed through aibridge.
// Unknown hosts return empty string and are passed through directly without aibridge.
// Uses exact host matching consistent with the MITM allowlist.
func providerFromHost(host string) string {
	h := canonicalHost(host)
	switch h {
	case "api.anthropic.com":
		return "anthropic"
	case "api.openai.com":
		return "openai"
	case "ampcode.com":
		return "amp"
	default:
		return ""
	}
}

// mapPathForProvider converts the original request path to the aibridge path format.
// Returns empty string if the path should not be routed through aibridge.
func mapPathForProvider(provider, originalPath string) string {
	switch provider {
	case "amp":
		// Only intercept AI provider routes
		// Original: /api/provider/anthropic/v1/messages
		// aibridge expects: /amp/v1/messages
		const ampPrefix = "/api/provider/anthropic"
		if strings.HasPrefix(originalPath, ampPrefix) {
			return "/amp" + strings.TrimPrefix(originalPath, ampPrefix)
		}
		// Other Amp routes (e.g., /api/internal) should not go through aibridge
		return ""
	case "anthropic":
		return "/anthropic" + originalPath
	case "openai":
		return "/openai" + originalPath
	default:
		return "/" + provider + originalPath
	}
}

// requestHandler intercepts HTTP requests after MITM decryption.
// LLM requests are rewritten to aibridge, with the Coder session token
// (from ctx.UserData, set during CONNECT) injected as "Authorization: Bearer <token>".
func (srv *Server) requestHandler(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	// Get token from UserData (set during CONNECT, propagated by goproxy)
	// Presence of token indicates request was decrypted via MITM
	coderToken, _ := ctx.UserData.(string)
	decrypted := coderToken != ""

	// Check if this request is for a supported AI provider.
	provider := providerFromHost(req.Host)
	if provider == "" {
		srv.logger.Debug(srv.ctx, "passthrough request to unknown host",
			slog.F("host", req.Host),
			slog.F("method", req.Method),
			slog.F("path", req.URL.Path),
			slog.F("decrypted", decrypted),
		)
		return req, nil
	}

	// Map the original path to the aibridge path
	originalPath := req.URL.Path
	aibridgePath := mapPathForProvider(provider, originalPath)

	// If path doesn't map to an aibridge route, pass through directly
	if aibridgePath == "" {
		srv.logger.Debug(srv.ctx, "passthrough request to non-aibridge path",
			slog.F("host", req.Host),
			slog.F("method", req.Method),
			slog.F("path", originalPath),
			slog.F("provider", provider),
			slog.F("decrypted", decrypted),
		)
		return req, nil
	}

	// Reject unauthenticated requests
	if coderToken == "" {
		srv.logger.Warn(srv.ctx, "rejecting unauthenticated request",
			slog.F("host", req.Host),
			slog.F("path", originalPath),
			slog.F("decrypted", decrypted),
		)
		resp := goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusProxyAuthRequired, "Proxy authentication required")
		resp.Header.Set("Proxy-Authenticate", `Basic realm="Coder AI proxy"`)
		return req, resp
	}

	// Rewrite URL to point to aibridged (shallow copy of pre-parsed URL)
	newURL := *srv.coderAccessURL
	newURL.Path = "/api/v2/aibridge" + aibridgePath
	newURL.RawQuery = req.URL.RawQuery

	req.URL = &newURL
	req.Host = newURL.Host

	// Set Authorization header for coder's aibridge authentication
	req.Header.Set("Authorization", "Bearer "+coderToken)

	srv.logger.Info(srv.ctx, "proxying decrypted request to aibridge",
		slog.F("method", req.Method),
		slog.F("provider", provider),
		slog.F("path", originalPath),
	)

	return req, nil
}

// responseHandler handles responses from upstream.
func (srv *Server) responseHandler(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	// Check for proxy errors (connection failures, TLS errors, etc.)
	if ctx.Error != nil {
		srv.logger.Error(srv.ctx, "upstream request failed",
			slog.F("error", ctx.Error.Error()),
			slog.F("url", ctx.Req.URL.String()),
			slog.F("method", ctx.Req.Method),
		)
		return resp
	}

	req := ctx.Req
	contentType := resp.Header.Get("Content-Type")
	contentEncoding := resp.Header.Get("Content-Encoding")

	// Skip logging for compressed or streaming responses to preserve streaming semantics
	if contentEncoding == "" && strings.Contains(contentType, "text") &&
		!strings.HasPrefix(contentType, "text/event-stream") {
		// Read the response body
		var bodyBytes []byte
		if resp.Body != nil {
			bodyBytes, _ = io.ReadAll(resp.Body)
			resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
			srv.logger.Info(srv.ctx, "received response",
				slog.F("url", req.URL.String()),
				slog.F("method", req.Method),
				slog.F("path", req.URL.Path),
				slog.F("response_status", resp.StatusCode),
				slog.F("response_body", string(bodyBytes)),
			)
		} else {
			srv.logger.Warn(srv.ctx, "received response",
				slog.F("url", req.URL.String()),
				slog.F("method", req.Method),
				slog.F("path", req.URL.Path),
				slog.F("response_status", resp.StatusCode),
				slog.F("response_body", string(bodyBytes)),
			)
		}
	}

	return resp
}

// Close shuts down the proxy server.
// Note: existing MITM'd connections may persist briefly after shutdown due to
// goproxy's hijack-based design - Shutdown only manages connections that net/http
// is aware of.
func (srv *Server) Close() error {
	if srv.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.httpServer.Shutdown(ctx)
}
