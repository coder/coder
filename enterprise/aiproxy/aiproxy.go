package aiproxy

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

	// Map of session ID -> Coder token (captured from CONNECT auth)
	sessionTokens   map[string]string
	sessionTokensMu sync.RWMutex
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
		sessionTokens:  make(map[string]string),
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
// Unknown hosts return empty string and are passed through directly without aibridge
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
		//   TODO(ssncferreira): aiproxy could automatically passthrough providers' routes
		return ""
	case "anthropic":
		return "/anthropic" + originalPath
	case "openai":
		return "/openai" + originalPath
	default:
		return "/" + provider + originalPath
	}
}

// requestHandler intercepts HTTP requests.
//
// For CONNECT requests: captures the Coder session token from Proxy-Authorization
// header. This must be done here (not in OnConnect) because gomitmproxy strips
// hop-by-hop headers (including Proxy-Authorization) before calling OnConnect.
//
// For other requests: LLM requests are rewritten to aibridge, with the Coder session
// token (captured during CONNECT) injected as "Authorization: Bearer <token>".
//
// Coder session token is passed via proxy basic auth: HTTPS_PROXY=http://ignored:<token>@host:port
// We store the token by base session ID for later retrieval because subsequent requests
// after CONNECT are child sessions that inherit the parent CONNECT session's base ID.
// For example, "100011-1-1-4" shares the base ID "100011" with its parent CONNECT session.
func (srv *Server) requestHandler(session *gomitmproxy.Session) (*http.Request, *http.Response) {
	req := session.Request()

	// Capture Proxy-Authorization on CONNECT before gomitmproxy strips hop-by-hop headers.
	if req.Method == http.MethodConnect {
		proxyAuth := req.Header.Get("Proxy-Authorization")
		coderToken := extractCoderTokenFromProxyAuth(proxyAuth)

		sessionID := session.ID()
		baseID := strings.Split(sessionID, "-")[0]

		srv.logger.Info(srv.ctx, "CONNECT request",
			slog.F("addr", req.URL.Host),
			slog.F("session_id", sessionID),
			slog.F("base_id", baseID),
			slog.F("proxyAuth", proxyAuth),
			slog.F("has_coder_token", coderToken != ""),
		)

		if coderToken != "" {
			srv.sessionTokensMu.Lock()
			srv.sessionTokens[baseID] = coderToken
			srv.sessionTokensMu.Unlock()
		}

		return nil, nil
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

	// Retrieve token stored during CONNECT using base session ID
	sessionID := session.ID()
	baseID := strings.Split(sessionID, "-")[0] // Get parent session ID

	srv.sessionTokensMu.RLock()
	coderToken := srv.sessionTokens[baseID]
	srv.sessionTokensMu.RUnlock()

	srv.logger.Info(srv.ctx, "looking up coder token",
		slog.F("session_id", sessionID),
		slog.F("base_id", baseID),
		slog.F("has_coder_token", coderToken != ""),
	)

	// Rewrite URL to point to aibridged
	newURL, err := url.Parse(srv.coderAccessURL)
	if err != nil {
		srv.logger.Error(srv.ctx, "failed to parse coder access URL", slog.Error(err))
		return req, nil
	}

	newURL.Path = "/api/v2/aibridge" + aibridgePath
	newURL.RawQuery = req.URL.RawQuery

	req.URL = newURL
	req.Host = newURL.Host

	// Set Authorization header for coder's aibridge authentication
	if coderToken != "" {
		req.Header.Set("Authorization", "Bearer "+coderToken)
	}

	srv.logger.Info(srv.ctx, "rewriting request to aibridged",
		slog.F("headers", req.Header),
		slog.F("original_url", req.URL.String()),
		slog.F("new_url", newURL.String()),
		slog.F("provider", provider),
		slog.F("has_coder_token", coderToken != ""),
	)

	return req, nil
}

// responseHandler handles responses from upstream.
// For now, just passes through.
func (srv *Server) responseHandler(session *gomitmproxy.Session) *http.Response {
	req := session.Request()
	resp := session.Response()

	contentType := resp.Header.Get("Content-Type")
	contentEncoding := resp.Header.Get("Content-Encoding")

	// Skip logging compressed responses
	if contentEncoding == "" && strings.Contains(contentType, "text") {
		// Read the response body
		var bodyBytes []byte
		if resp != nil && resp.Body != nil {
			bodyBytes, _ = io.ReadAll(resp.Body)
			resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		if resp.StatusCode == 200 || resp.StatusCode == 202 {
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

	// Without response body log
	//if resp.StatusCode == 200 || resp.StatusCode == 202 {
	//	srv.logger.Info(srv.ctx, "received response",
	//		slog.F("url", req.URL.String()),
	//		slog.F("method", req.Method),
	//		slog.F("path", req.URL.Path),
	//		slog.F("response_status", resp.StatusCode),
	//
	//	)
	//} else {
	//	srv.logger.Warn(srv.ctx, "received response",
	//		slog.F("url", req.URL.String()),
	//		slog.F("method", req.Method),
	//		slog.F("path", req.URL.Path),
	//		slog.F("response_status", resp.StatusCode),
	//
	//	)
	//}

	return resp
}

// Close shuts down the proxy server.
func (srv *Server) Close() error {
	if srv.proxy != nil {
		srv.proxy.Close()
	}
	return nil
}
