package aiproxy

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
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
		OnConnect:  srv.connectHandler,
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

// connectHandler captures the Coder session token from CONNECT requests.
//
// Copilot CLI passes the token via proxy basic auth: HTTPS_PROXY=http://ignored:<token>@host:port
// The Proxy-Authorization header is only sent on CONNECT (not subsequent tunnel requests),
// so we store the token by base session ID for later retrieval in requestHandler.
//
// Note: requires gomitmproxy modification to preserve Proxy-Authorization before hop-by-hop stripping:
func (srv *Server) connectHandler(session *gomitmproxy.Session, proto string, addr string) net.Conn {
	// Only capture token for hosts that will be routed to aibridge
	if !strings.Contains(addr, "api.individual.githubcopilot.com") {
		return nil
	}

	req := session.Request()

	proxyAuth := req.Header.Get("Proxy-Authorization")
	coderToken := extractCoderTokenFromProxyAuth(proxyAuth)

	sessionID := session.ID()
	baseID := strings.Split(sessionID, "-")[0] // Just get "100011"

	fmt.Println("######################### req.Header: ", req.Header)
	fmt.Println("######################### proxyAuth: ", proxyAuth)
	fmt.Println("######################### coderToken: ", coderToken)

	srv.logger.Info(srv.ctx, "CONNECT request",
		slog.F("addr", addr),
		slog.F("session_id", sessionID),
		slog.F("base_id", baseID),
		slog.F("has_proxy_auth", proxyAuth != ""),
		slog.F("has_coder_token", coderToken != ""),
	)

	if coderToken != "" {
		srv.sessionTokensMu.Lock()
		srv.sessionTokens[baseID] = coderToken
		srv.sessionTokensMu.Unlock()
	}

	// Return nil to let gomitmproxy handle the connection normally
	return nil
}

// providerFromHost maps the request host to the aibridge provider name.
// All requests through the proxy for known AI providers are routed through aibridge.
// Unknown hosts return empty string and are passed through directly without aibridge
//	(TODO (ssncferreira): is this correct?)
func providerFromHost(host string) string {
	switch {
	case strings.Contains(host, "api.anthropic.com"):
		return "anthropic"
	case strings.Contains(host, "openai.com"):
		return "openai"
	case strings.Contains(host, "ampcode.com"):
		return "amp"
	case strings.Contains(host, "api.individual.githubcopilot.com"):
		return "copilot"
	default:
		return ""
	}
}

// mapPathForProvider converts the original request path to the aibridge path format.
// Returns empty string if the path should not be routed through aibridge.
func mapPathForProvider(provider, originalPath string) string {
	switch provider {
	case "anthropic":
		return "/anthropic" + originalPath
	case "openai":
		return "/openai" + originalPath
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
	case "copilot":
		// Only intercept AI provider routes
		// Original: /chat/completions
		// aibridge expects: /copilot/v1/chat/completions
		const copilotChatPath = "/chat/completions"
		if strings.HasPrefix(originalPath, copilotChatPath) {
			return "/copilot/v1" + originalPath
		}
		// Other Copilot routes should not go through aibridge
		return ""
	default:
		return "/" + provider + originalPath
	}
}

// requestHandler intercepts HTTP requests after the TLS tunnel is established.
//
// Requests to api.github.com (GitHub auth) are passed through directly.
// LLM requests to api.individual.githubcopilot.com/chat/completions are rewritten to aibridge,
// with the Coder session token (captured during CONNECT) injected as "Authorization: Bearer <token>".
//
// We look up the token using the base session ID (e.g., "100011" from "100011-1-1-4") because
// CONNECT and subsequent requests have different session IDs but share the same base prefix.
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

	provider := providerFromHost(req.Host)
	if provider == "" {
		srv.logger.Info(srv.ctx, "unknown provider, passthrough",
			slog.F("host", req.Host),
		)
		return req, nil
	}

	originalPath := req.URL.Path
	aibridgePath := mapPathForProvider(provider, originalPath)

	if aibridgePath == "" {
		srv.logger.Info(srv.ctx, "path not handled by aibridge, passthrough",
			slog.F("host", req.Host),
			slog.F("path", originalPath),
		)
		return req, nil
	}

	// Retrieve token stored during CONNECT using base session ID
	sessionID := session.ID()
	baseID := strings.Split(sessionID, "-")[0]

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

	srv.logger.Info(srv.ctx, "rewriting request to aibridged",
		slog.F("original_url", req.URL.String()),
		slog.F("new_url", newURL.String()),
		slog.F("provider", provider),
		slog.F("has_coder_token", coderToken != ""),
	)

	req.URL = newURL
	req.Host = newURL.Host

	// Set Authorization header for aibridge authentication
	if coderToken != "" {
		req.Header.Set("Authorization", "Bearer "+coderToken)
	}

	return req, nil
}

// responseHandler handles responses from upstream.
// For now, just passes through.
func (srv *Server) responseHandler(session *gomitmproxy.Session) *http.Response {
	req := session.Request()
	resp := session.Response()

	//contentType := resp.Header.Get("Content-Type")
	//contentEncoding := resp.Header.Get("Content-Encoding")
	//
	//// Skip logging compressed responses
	//if contentEncoding == "" && strings.Contains(contentType, "text") {
	//	// Read the response body
	//	var bodyBytes []byte
	//	if resp != nil && resp.Body != nil {
	//		bodyBytes, _ = io.ReadAll(resp.Body)
	//		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	//	}
	//
	//	if resp.StatusCode == 200 || resp.StatusCode == 202 {
	//		srv.logger.Info(srv.ctx, "received response",
	//			slog.F("url", req.URL.String()),
	//			slog.F("method", req.Method),
	//			slog.F("path", req.URL.Path),
	//			slog.F("response_status", resp.StatusCode),
	//			slog.F("response_body", string(bodyBytes)),
	//		)
	//	} else {
	//		srv.logger.Warn(srv.ctx, "received response",
	//			slog.F("url", req.URL.String()),
	//			slog.F("method", req.Method),
	//			slog.F("path", req.URL.Path),
	//			slog.F("response_status", resp.StatusCode),
	//			slog.F("response_body", string(bodyBytes)),
	//		)
	//	}
	//}

	// Without response body log
	if resp.StatusCode == 200 || resp.StatusCode == 202 {
		srv.logger.Info(srv.ctx, "received response",
			slog.F("url", req.URL.String()),
			slog.F("method", req.Method),
			slog.F("path", req.URL.Path),
			slog.F("response_status", resp.StatusCode),
		)
	} else {
		srv.logger.Warn(srv.ctx, "received response",
			slog.F("url", req.URL.String()),
			slog.F("method", req.Method),
			slog.F("path", req.URL.Path),
			slog.F("response_status", resp.StatusCode),
		)
	}

	return resp
}

// Close shuts down the proxy server.
func (srv *Server) Close() error {
	if srv.proxy != nil {
		srv.proxy.Close()
	}
	return nil
}
