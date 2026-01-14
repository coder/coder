//nolint:revive,gocritic,errname,unconvert,noctx,errorlint,bodyclose
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" // G108: pprof is intentionally exposed for debugging
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/cli/boundary/audit"
	"github.com/coder/coder/v2/enterprise/cli/boundary/rulesengine"
)

// Server handles HTTP and HTTPS requests with rule-based filtering
type Server struct {
	ruleEngine rulesengine.Engine
	auditor    audit.Auditor
	logger     *slog.Logger
	tlsConfig  *tls.Config
	httpPort   int
	started    atomic.Bool

	listener     net.Listener
	pprofServer  *http.Server
	pprofEnabled bool
	pprofPort    int
}

// Config holds configuration for the proxy server
type Config struct {
	HTTPPort     int
	RuleEngine   rulesengine.Engine
	Auditor      audit.Auditor
	Logger       *slog.Logger
	TLSConfig    *tls.Config
	PprofEnabled bool
	PprofPort    int
}

// NewProxyServer creates a new proxy server instance
func NewProxyServer(config Config) *Server {
	return &Server{
		ruleEngine:   config.RuleEngine,
		auditor:      config.Auditor,
		logger:       config.Logger,
		tlsConfig:    config.TLSConfig,
		httpPort:     config.HTTPPort,
		pprofEnabled: config.PprofEnabled,
		pprofPort:    config.PprofPort,
	}
}

// Start starts the HTTP proxy server with TLS termination capability
func (p *Server) Start() error {
	if p.isStarted() {
		return nil
	}

	p.logger.Info("Starting HTTP proxy with TLS termination", "port", p.httpPort)

	// Start pprof server if enabled
	if p.pprofEnabled {
		p.pprofServer = &http.Server{ // G112: pprof server doesn't need ReadHeaderTimeout
			Addr:    fmt.Sprintf(":%d", p.pprofPort),
			Handler: http.DefaultServeMux,
		}

		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p.pprofPort))
		if err != nil {
			p.logger.Error("failed to listen on port for pprof server", "port", p.pprofPort, "error", err)
			return xerrors.Errorf("failed to listen on port %v for pprof server: %v", p.pprofPort, err)
		}

		go func() {
			p.logger.Info("Serving pprof on existing listener", "port", p.pprofPort)
			if err := p.pprofServer.Serve(ln); err != nil && errors.Is(err, http.ErrServerClosed) {
				p.logger.Error("pprof server error", "error", err)
			}
		}()

	}

	var err error
	p.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", p.httpPort))
	if err != nil {
		p.logger.Error("Failed to create HTTP listener", "error", err)
		return err
	}

	p.started.Store(true)

	// Start HTTP server with custom listener for TLS detection
	go func() {
		for {
			conn, err := p.listener.Accept()
			if err != nil && errors.Is(err, net.ErrClosed) && p.isStopped() {
				return
			}
			if err != nil {
				p.logger.Error("Failed to accept connection", "error", err)
				continue
			}

			// Handle connection with TLS detection
			go p.handleConnectionWithTLSDetection(conn)
		}
	}()

	return nil
}

// Stops proxy server
func (p *Server) Stop() error {
	if p.isStopped() {
		return nil
	}
	p.started.Store(false)

	if p.listener == nil {
		p.logger.Error("unexpected nil listener")
		return xerrors.New("unexpected nil listener")
	}

	err := p.listener.Close()
	if err != nil {
		p.logger.Error("Failed to close listener", "error", err)
		return err
	}

	// Close pprof server
	if p.pprofServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.pprofServer.Shutdown(ctx); err != nil {
			p.logger.Error("Failed to shutdown pprof server", "error", err)
		}
	}

	return nil
}

func (p *Server) isStarted() bool {
	return p.started.Load()
}

func (p *Server) isStopped() bool {
	return !p.started.Load()
}

func (p *Server) handleConnectionWithTLSDetection(conn net.Conn) {
	// Detect protocol using TLS handshake detection
	wrappedConn, isTLS, err := p.isTLSConnection(conn)
	if err != nil {
		p.logger.Error("Failed to check connection type", "error", err)

		err := conn.Close()
		if err != nil {
			p.logger.Error("Failed to close connection", "error", err)
		}
		return
	}
	if isTLS {
		p.logger.Debug("🔒 Detected TLS connection - handling as HTTPS")
		p.handleTLSConnection(wrappedConn)
	} else {
		p.logger.Debug("🌐 Detected HTTP connection")
		p.handleHTTPConnection(wrappedConn)
	}
}

func (p *Server) isTLSConnection(conn net.Conn) (net.Conn, bool, error) {
	// Read first byte to detect TLS
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return nil, false, xerrors.Errorf("failed to read first byte from connection: %v, read %v bytes", err, n)
	}

	connWrapper := &connectionWrapper{conn, buf, false}

	// TLS detection based on first byte:
	// 0x16 (22) = TLS Handshake
	// 0x17 (23) = TLS Application Data
	// 0x14 (20) = TLS Change Cipher Spec
	// 0x15 (21) = TLS Alert
	isTLS := buf[0] == 0x16 || buf[0] == 0x17 || buf[0] == 0x14 || buf[0] == 0x15

	if isTLS {
		p.logger.Debug("TLS detected", "first byte", buf[0])
	}

	return connWrapper, isTLS, nil
}

func (p *Server) handleHTTPConnection(conn net.Conn) {
	defer func() {
		err := conn.Close()
		if err != nil {
			p.logger.Error("Failed to close connection", "error", err)
		}
	}()

	// Read HTTP request
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		p.logger.Error("Failed to read HTTP request", "error", err)
		return
	}

	if req.Method == http.MethodConnect {
		p.handleCONNECT(conn, req)
		return
	}

	p.logger.Debug("🌐 HTTP Request", "method", req.Method, "url", req.URL.String())
	p.processHTTPRequest(conn, req, false)
}

func (p *Server) handleTLSConnection(conn net.Conn) {
	// Create TLS connection
	tlsConn := tls.Server(conn, p.tlsConfig)

	defer func() {
		err := tlsConn.Close()
		if err != nil {
			p.logger.Error("Failed to close TLS connection", "error", err)
		}
	}()

	// Perform TLS handshake
	if err := tlsConn.Handshake(); err != nil {
		p.logger.Error("TLS handshake failed", "error", err)
		return
	}

	p.logger.Debug("✅ TLS handshake successful")

	// Read HTTP request over TLS
	req, err := http.ReadRequest(bufio.NewReader(tlsConn))
	if err != nil {
		p.logger.Error("Failed to read HTTPS request", "error", err)
		return
	}

	p.logger.Debug("🔒 HTTPS Request", "method", req.Method, "url", req.URL.String())
	p.processHTTPRequest(tlsConn, req, true)
}

func (p *Server) processHTTPRequest(conn net.Conn, req *http.Request, https bool) {
	p.logger.Debug("   Host", "host", req.Host)
	p.logger.Debug("   User-Agent", "user-agent", req.Header.Get("User-Agent"))

	// Construct fully qualified URL for rule evaluation and auditing.
	// In boundary's normal transparent proxy operation, req.URL only contains
	// the path since clients don't know they're going through a proxy.
	// When clients explicitly configure a proxy, req.URL contains the full URL.
	fullURL := req.URL.String()
	if req.URL.Scheme == "" {
		scheme := "http"
		if https {
			scheme = "https"
		}
		fullURL = scheme + "://" + req.Host + fullURL
	}

	result := p.ruleEngine.Evaluate(req.Method, fullURL)

	p.auditor.AuditRequest(audit.Request{
		Method:  req.Method,
		URL:     fullURL,
		Host:    req.Host,
		Allowed: result.Allowed,
		Rule:    result.Rule,
	})

	if !result.Allowed {
		p.writeBlockedResponse(conn, req)
		return
	}

	// Forward request to destination
	p.forwardRequest(conn, req, https)
}

func (p *Server) forwardRequest(conn net.Conn, req *http.Request, https bool) {
	// Create HTTP client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	scheme := "http"
	if https {
		scheme = "https"
	}

	// Create a new request to the target server
	targetURL := &url.URL{
		Scheme:   scheme,
		Host:     req.Host,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
	}

	body := req.Body
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		body = nil
	}
	newReq, err := http.NewRequest(req.Method, targetURL.String(), body)
	if err != nil {
		p.logger.Error("can't create http request", "error", err)
		return
	}

	// Copy headers
	for name, values := range req.Header {
		// Skip connection-specific headers
		if strings.ToLower(name) == "connection" || strings.ToLower(name) == "proxy-connection" {
			continue
		}
		for _, value := range values {
			newReq.Header.Add(name, value)
		}
	}

	// Make request to destination
	resp, err := client.Do(newReq)
	if err != nil {
		p.logger.Error("Failed to forward HTTPS request", "error", err)
		return
	}

	p.logger.Debug("🔒 HTTPS Response", "status code", resp.StatusCode, "status", resp.Status)

	p.logger.Debug("Forwarded Request",
		"method", newReq.Method,
		"host", newReq.Host,
		"URL", newReq.URL,
	)

	// Read the body and explicitly set Content-Length header, otherwise client can hung up on the request.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Error("can't read response body", "error", err)
		return
	}
	resp.Header.Add("Content-Length", strconv.Itoa(len(bodyBytes)))
	resp.ContentLength = int64(len(bodyBytes))
	err = resp.Body.Close()
	if err != nil {
		p.logger.Error("Failed to close HTTP response body", "error", err)
		return
	}
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// The downstream client (Claude) always communicates over HTTP/1.1.
	// However, Go's default HTTP client may negotiate an HTTP/2 connection
	// with the upstream server via ALPN during TLS handshake.
	// This can cause the response's Proto field to be set to "HTTP/2.0",
	// which would produce an invalid response for an HTTP/1.1 client.
	// To prevent this mismatch, we explicitly normalize the response
	// to HTTP/1.1 before writing it back to the client.
	resp.Proto = "HTTP/1.1"
	resp.ProtoMajor = 1
	resp.ProtoMinor = 1

	// Copy response back to client
	err = resp.Write(conn)
	if err != nil {
		p.logger.Error("Failed to forward back HTTP response",
			"error", err,
			"host", req.Host,
			"method", req.Method,
			// "bodyBytes", string(bodyBytes),
		)
		return
	}

	p.logger.Debug("Successfully wrote to connection")
}

func (p *Server) writeBlockedResponse(conn net.Conn, req *http.Request) {
	// Create a response object
	resp := &http.Response{
		Status:        "403 Forbidden",
		StatusCode:    http.StatusForbidden,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          nil,
		ContentLength: 0,
	}

	// Set headers
	resp.Header.Set("Content-Type", "text/plain")

	// Create the response body
	host := req.URL.Host
	if host == "" {
		host = req.Host
	}

	body := fmt.Sprintf(`🚫 Request Blocked by Boundary

Request: %s %s
Host: %s

To allow this request, restart boundary with:
  --allow "domain=%s"                    # Allow all methods to this host
  --allow "method=%s domain=%s"          # Allow only %s requests to this host

For more help: https://github.com/coder/boundary
`,
		req.Method, req.URL.Path, host, host, req.Method, host, req.Method)

	resp.Body = io.NopCloser(strings.NewReader(body))
	resp.ContentLength = int64(len(body))

	// Copy response back to client
	err := resp.Write(conn)
	if err != nil {
		p.logger.Error("Failed to write blocker response", "error", err)
		return
	}

	p.logger.Debug("Successfully wrote to connection")
}

// connectionWrapper lets us "unread" the peeked byte
type connectionWrapper struct {
	net.Conn
	buf     []byte
	bufUsed bool
}

func (c *connectionWrapper) Read(p []byte) (int, error) {
	if !c.bufUsed && len(c.buf) > 0 {
		n := copy(p, c.buf)
		c.bufUsed = true
		return n, nil
	}
	return c.Conn.Read(p)
}
