package aiproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/elazarl/goproxy"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

type Server struct {
	ctx            context.Context
	logger         slog.Logger
	proxy          *goproxy.ProxyHttpServer
	httpServer     *http.Server
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

	// Load CA certificate for MITM
	if err := loadMitmCertificate(opts.CertFile, opts.KeyFile); err != nil {
		return nil, xerrors.Errorf("failed to load MITM certificate: %w", err)
	}

	srv := &Server{
		ctx:            ctx,
		logger:         logger,
		coderAccessURL: opts.CoderAccessURL,
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = false

	// Custom MITM handler that extracts auth and rejects unauthenticated requests.
	// The token is stored in ctx.UserData which goproxy propagates to subsequent
	// request contexts for decrypted requests within this MITM session.
	mitmWithAuth := goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		proxyAuth := ctx.Req.Header.Get("Proxy-Authorization")
		coderToken := extractCoderTokenFromProxyAuth(proxyAuth)

		srv.logger.Info(srv.ctx, "CONNECT request",
			slog.F("host", host),
			slog.F("has_coder_token", coderToken != ""),
		)

		// Reject unauthenticated requests - proxy is a protected service
		if coderToken == "" {
			srv.logger.Warn(srv.ctx, "rejecting unauthenticated CONNECT request",
				slog.F("host", host),
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
		Addr:    opts.ListenAddr,
		Handler: proxy,
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
	if !strings.HasPrefix(proxyAuth, "Basic ") {
		return ""
	}

	encoded := strings.TrimPrefix(proxyAuth, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}

	// Format: "username:password" - we use password as the Coder token
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return ""
	}

	return parts[1]
}

// providerFromHost maps the request host to the aibridge provider name.
// All requests through the proxy for known AI providers are routed through aibridge.
// Unknown hosts return empty string and are passed through directly without aibridge.
func providerFromHost(host string) string {
	switch {
	case strings.Contains(host, "api.anthropic.com"):
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
	coderToken, _ := ctx.UserData.(string)

	srv.logger.Info(srv.ctx, "received request",
		slog.F("url", req.URL.String()),
		slog.F("method", req.Method),
		slog.F("host", req.Host),
		slog.F("has_coder_token", coderToken != ""),
	)

	// Check if this request is for a supported AI provider.
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

	// Reject unauthenticated requests
	if coderToken == "" {
		srv.logger.Warn(srv.ctx, "rejecting unauthenticated request",
			slog.F("host", req.Host),
			slog.F("path", originalPath),
		)
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusProxyAuthRequired, "Proxy authentication required")
	}

	// Rewrite URL to point to aibridged
	newURL, err := url.Parse(srv.coderAccessURL)
	if err != nil {
		srv.logger.Error(srv.ctx, "failed to parse coder access URL", slog.Error(err))
		return req, goproxy.NewResponse(req, goproxy.ContentTypeText, http.StatusInternalServerError, "Internal proxy error")
	}

	newURL.Path = "/api/v2/aibridge" + aibridgePath
	newURL.RawQuery = req.URL.RawQuery

	req.URL = newURL
	req.Host = newURL.Host

	// Set Authorization header for coder's aibridge authentication
	req.Header.Set("Authorization", "Bearer "+coderToken)

	srv.logger.Info(srv.ctx, "rewriting request to aibridged",
		slog.F("new_url", newURL.String()),
		slog.F("provider", provider),
	)

	return req, nil
}

// responseHandler handles responses from upstream.
func (srv *Server) responseHandler(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	if resp == nil {
		return resp
	}

	req := ctx.Req
	contentType := resp.Header.Get("Content-Type")
	contentEncoding := resp.Header.Get("Content-Encoding")

	// Skip logging compressed responses
	if contentEncoding == "" && strings.Contains(contentType, "text") {
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
func (srv *Server) Close() error {
	if srv.httpServer != nil {
		return srv.httpServer.Shutdown(srv.ctx)
	}
	return nil
}
