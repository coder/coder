package aibridgeproxyd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/elazarl/goproxy"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// loadMitmOnce ensures the MITM certificate is loaded exactly once.
// goproxy.GoproxyCa is a package-level global variable shared across all
// goproxy.ProxyHttpServer instances in the process. In tests, multiple proxy
// servers run in parallel, and without this guard they would race on writing
// to GoproxyCa. In production, only one server runs, so this has no impact.
var loadMitmOnce sync.Once

// Server is the AI MITM (Man-in-the-Middle) proxy server.
// It is responsible for:
//   - intercepting HTTPS requests to AI providers
//   - decrypting requests using the configured CA certificate
//   - forwarding requests to aibridged for processing
type Server struct {
	ctx        context.Context
	logger     slog.Logger
	proxy      *goproxy.ProxyHttpServer
	httpServer *http.Server
	listener   net.Listener
}

// Options configures the AI Bridge Proxy server.
type Options struct {
	// ListenAddr is the address the proxy server will listen on.
	ListenAddr string
	// CertFile is the path to the CA certificate file used for MITM.
	CertFile string
	// KeyFile is the path to the CA private key file used for MITM.
	KeyFile string
	// AllowedPorts is the list of ports allowed for CONNECT requests.
	// Defaults to ["80", "443"] if empty.
	AllowedPorts []string
}

func New(ctx context.Context, logger slog.Logger, opts Options) (*Server, error) {
	logger.Info(ctx, "initializing AI Bridge Proxy server")

	if opts.ListenAddr == "" {
		return nil, xerrors.New("listen address is required")
	}

	if opts.CertFile == "" || opts.KeyFile == "" {
		return nil, xerrors.New("cert file and key file are required")
	}

	// Load CA certificate for MITM
	if err := loadMitmCertificate(opts.CertFile, opts.KeyFile); err != nil {
		return nil, xerrors.Errorf("failed to load MITM certificate: %w", err)
	}

	proxy := goproxy.NewProxyHttpServer()

	srv := &Server{
		ctx:    ctx,
		logger: logger,
		proxy:  proxy,
	}

	// Reject CONNECT requests to non-standard ports.
	allowedPorts := opts.AllowedPorts
	if len(allowedPorts) == 0 {
		allowedPorts = []string{"80", "443"}
	}
	proxy.OnRequest().HandleConnectFunc(srv.portMiddleware(allowedPorts))

	// Extract Coder session token from proxy authentication to forward to aibridged.
	// Decrypt all HTTPS requests via MITM. Requests are forwarded to
	// the original destination without modification for now.
	// TODO(ssncferreira): Route requests to aibridged will be implemented upstack.
	//   Related to https://github.com/coder/internal/issues/1181
	proxy.OnRequest().HandleConnectFunc(srv.authMiddleware)

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

	go func() {
		logger.Info(ctx, "starting AI Bridge Proxy", slog.F("addr", listener.Addr().String()))
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
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// loadMitmCertificate loads the CA certificate and private key for MITM proxying.
// This function is safe to call concurrently - the certificate is only loaded once
// into the global goproxy.GoproxyCa variable.
func loadMitmCertificate(certFile, keyFile string) error {
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return xerrors.Errorf("load CA certificate: %w", err)
	}

	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return xerrors.Errorf("parse CA certificate: %w", err)
	}

	// Only protect the global assignment with sync.Once
	loadMitmOnce.Do(func() {
		goproxy.GoproxyCa = tls.Certificate{
			Certificate: tlsCert.Certificate,
			PrivateKey:  tlsCert.PrivateKey,
			Leaf:        x509Cert,
		}
	})

	return nil
}

// portMiddleware is a CONNECT middleware that rejects requests to non-standard ports.
// This prevents the proxy from being used to tunnel to arbitrary services (SSH, databases, etc.).
func (s *Server) portMiddleware(allowedPorts []string) func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	allowed := make(map[string]bool, len(allowedPorts))
	for _, p := range allowedPorts {
		allowed[p] = true
	}

	return func(host string, _ *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		_, port, err := net.SplitHostPort(host)
		if err != nil {
			s.logger.Warn(s.ctx, "rejecting CONNECT with missing port",
				slog.F("host", host),
			)
			return goproxy.RejectConnect, host
		}

		if !allowed[port] {
			s.logger.Warn(s.ctx, "rejecting CONNECT to non-allowed port",
				slog.F("host", host),
				slog.F("port", port),
			)
			return goproxy.RejectConnect, host
		}

		return nil, ""
	}
}

// authMiddleware is a CONNECT middleware that extracts the Coder session token
// from the Proxy-Authorization header and stores it in ctx.UserData for use by
// downstream request handlers.
// Requests without valid credentials are rejected.
//
// Clients provide credentials by setting their HTTP Proxy as:
//
//	HTTPS_PROXY=http://ignored:<coder-token>@host:port
//
// The token is extracted from the password field of basic auth.
func (s *Server) authMiddleware(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	proxyAuth := ctx.Req.Header.Get("Proxy-Authorization")
	coderToken := extractCoderTokenFromProxyAuth(proxyAuth)

	// Reject requests without valid credentials.
	if coderToken == "" {
		hasAuth := proxyAuth != ""
		s.logger.Warn(s.ctx, "rejecting CONNECT request",
			slog.F("host", host),
			slog.F("reason", map[bool]string{true: "invalid_credentials", false: "missing_credentials"}[hasAuth]),
		)
		return goproxy.RejectConnect, host
	}

	// Store the token in UserData for downstream handlers.
	// goproxy propagates UserData to subsequent request contexts
	// for decrypted requests within this MITM session.
	ctx.UserData = coderToken

	return goproxy.MitmConnect, host
}

// extractCoderTokenFromProxyAuth extracts the Coder session token from the
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
