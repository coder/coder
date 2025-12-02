package aiproxy

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cdr.dev/slog"
	"golang.org/x/xerrors"

	"github.com/AdguardTeam/gomitmproxy"
	"github.com/AdguardTeam/gomitmproxy/mitm"
)

type Server struct {
	ctx            context.Context
	logger         slog.Logger
	proxy          *gomitmproxy.Proxy
	coderAccessURL string
}

type Options struct {
	ListenAddr     string
	CertFile       string
	KeyFile        string
	CoderAccessURL string
}

func New(ctx context.Context, logger slog.Logger, opts Options) (*Server, error) {
	logger.Info(ctx, "initializing AI proxy server")

	mitmConfig, err := createMitmConfig(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, xerrors.Errorf("failed to create TLS proxy config: %w", err)
	}

	addr, err := net.ResolveTCPAddr("tcp", opts.ListenAddr)
	if err != nil {
		return nil, xerrors.Errorf("listen-address invalid: %w", err)
	}

	srv := &Server{
		ctx:            ctx,
		logger:         logger,
		coderAccessURL: opts.CoderAccessURL,
	}

	logger.Info(ctx, "starting AI proxy", slog.F("addr", addr.String()))
	proxy := gomitmproxy.NewProxy(gomitmproxy.Config{
		ListenAddr: addr,
		OnRequest:  srv.requestHandler,
		OnResponse: srv.responseHandler,
		MITMConfig: mitmConfig,
	})

	if err := proxy.Start(); err != nil {
		return nil, xerrors.Errorf("failed to start proxy: %w", err)
	}

	srv.proxy = proxy
	return srv, nil
}

// createMitmConfig creates the TLS MITM configuration.
func createMitmConfig(certFile, keyFile string) (*mitm.Config, error) {
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, xerrors.Errorf("failed to read x509 keypair: %w", err)
	}
	privateKey := tlsCert.PrivateKey.(*rsa.PrivateKey)

	x509c, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, xerrors.Errorf("failed to parse cert: %w", err)
	}

	mitmConfig, err := mitm.NewConfig(x509c, privateKey, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to create MITM config: %w", err)
	}

	mitmConfig.SetValidity(time.Hour * 24 * 365) // 1 year validity
	mitmConfig.SetOrganization("coder aiproxy")
	return mitmConfig, nil
}

// providerFromHost maps the request host to the aibridge provider name.
// All requests through the proxy for known AI providers are routed through aibridge.
// Unknown hosts return empty string and are passed through directly without aibridge
//	(TODO (ssncferreira): is this correct?)
func providerFromHost(host string) string {
	switch {
	case strings.Contains(host, "anthropic.com"):
		return "anthropic"
	case strings.Contains(host, "openai.com"):
		return "openai"
	case strings.Contains(host, "ampcode.com"):
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
		//   TODO(ssncferreira): should these routes be added to the provider's PassthroughRoutes?
		return ""
	case "anthropic":
		return "/anthropic" + originalPath
	case "openai":
		return "/openai" + originalPath
	default:
		return "/" + provider + originalPath
	}
}

// requestHandler handles incoming requests.
func (srv *Server) requestHandler(session *gomitmproxy.Session) (*http.Request, *http.Response) {
	req := session.Request()

	if req.Method == http.MethodConnect {
		return req, nil
	}

	srv.logger.Info(srv.ctx, "received request",
		slog.F("url", req.URL.String()),
		slog.F("method", req.Method),
		slog.F("host", req.Host),
	)

	// Check if this request is for a supported AI provider.
	// All requests through the proxy for known AI providers are routed through aibridge.
	// Unknown hosts return empty string and are passed through directly without aibridge.
	provider := providerFromHost(req.Host)
	if provider == "" {
		srv.logger.Info(srv.ctx, "unknown provider, passthrough",
			slog.F("host", req.Host),
		)
		return req, nil
	}

	// Map the original path to the aibridge path
	originalPath := req.URL.Path
	aibridgePath := mapPathForProvider(provider, originalPath)

	// If path doesn't map to an aibridge route, pass through directly
	if aibridgePath == "" {
		srv.logger.Info(srv.ctx, "path not handled by aibridge, passthrough",
			slog.F("host", req.Host),
			slog.F("path", originalPath),
		)
		return req, nil
	}

	// Rewrite URL to point to aibridged
	newURL, err := url.Parse(srv.coderAccessURL)
	if err != nil {
		srv.logger.Error(srv.ctx, "failed to parse coder access URL", slog.Error(err))
		return req, nil
	}

	newURL.Path = "/api/v2/aibridge" + aibridgePath
	newURL.RawQuery = req.URL.RawQuery

	srv.logger.Info(srv.ctx, "rewriting request to aibridged",
		slog.F("original_url", req.URL.String()),
		slog.F("new_url", newURL.String()),
		slog.F("provider", provider),
	)

	req.URL = newURL
	req.Host = newURL.Host

	return req, nil
}

// responseHandler handles responses from upstream.
// For now, just passes through.
func (srv *Server) responseHandler(session *gomitmproxy.Session) *http.Response {
	req := session.Request()
	srv.logger.Info(srv.ctx, "received response",
		slog.F("url", req.URL.String()),
		slog.F("method", req.Method),
	)

	return session.Response()
}

// Close shuts down the proxy server.
func (srv *Server) Close() error {
	if srv.proxy != nil {
		srv.proxy.Close()
	}
	return nil
}
