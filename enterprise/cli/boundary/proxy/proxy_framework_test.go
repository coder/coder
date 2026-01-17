//nolint:paralleltest,testpackage,revive,gocritic,noctx,bodyclose
package proxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/enterprise/cli/boundary/audit"
	"github.com/coder/coder/v2/enterprise/cli/boundary/rulesengine"
	boundary_tls "github.com/coder/coder/v2/enterprise/cli/boundary/tls"
)

// mockAuditor is a simple mock auditor for testing
type mockAuditor struct{}

func (m *mockAuditor) AuditRequest(req audit.Request) {
	// No-op for testing
}

// ProxyTest is a high-level test framework for proxy tests
type ProxyTest struct {
	t              *testing.T
	server         *Server
	client         *http.Client
	proxyClient    *http.Client
	port           int
	useCertManager bool
	configDir      string
	startupDelay   time.Duration
	allowedRules   []string
	auditor        audit.Auditor
}

// ProxyTestOption is a function that configures ProxyTest
type ProxyTestOption func(*ProxyTest)

// NewProxyTest creates a new ProxyTest instance
func NewProxyTest(t *testing.T, opts ...ProxyTestOption) *ProxyTest {
	pt := &ProxyTest{
		t:              t,
		port:           8080,
		useCertManager: false,
		configDir:      "/tmp/boundary",
		startupDelay:   100 * time.Millisecond,
		allowedRules:   []string{}, // Default: deny all (no rules = deny by default)
	}

	// Apply options
	for _, opt := range opts {
		opt(pt)
	}

	return pt
}

// WithProxyPort sets the proxy server port
func WithProxyPort(port int) ProxyTestOption {
	return func(pt *ProxyTest) {
		pt.port = port
	}
}

// WithCertManager enables TLS certificate manager
func WithCertManager(configDir string) ProxyTestOption {
	return func(pt *ProxyTest) {
		pt.useCertManager = true
		pt.configDir = configDir
	}
}

// WithStartupDelay sets how long to wait after starting server before making requests
func WithStartupDelay(delay time.Duration) ProxyTestOption {
	return func(pt *ProxyTest) {
		pt.startupDelay = delay
	}
}

// WithAllowedDomain adds an allowed domain rule
func WithAllowedDomain(domain string) ProxyTestOption {
	return func(pt *ProxyTest) {
		pt.allowedRules = append(pt.allowedRules, fmt.Sprintf("domain=%s", domain))
	}
}

// WithAllowedRule adds a full allow rule (e.g., "method=GET domain=example.com path=/api/*")
func WithAllowedRule(rule string) ProxyTestOption {
	return func(pt *ProxyTest) {
		pt.allowedRules = append(pt.allowedRules, rule)
	}
}

// WithAuditor sets a custom auditor for capturing audit requests
func WithAuditor(auditor audit.Auditor) ProxyTestOption {
	return func(pt *ProxyTest) {
		pt.auditor = auditor
	}
}

// Start starts the proxy server
func (pt *ProxyTest) Start() *ProxyTest {
	pt.t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	testRules, err := rulesengine.ParseAllowSpecs(pt.allowedRules)
	require.NoError(pt.t, err, "Failed to parse test rules")

	ruleEngine := rulesengine.NewRuleEngine(testRules, logger)

	// Use custom auditor if provided, otherwise use no-op mock
	auditor := pt.auditor
	if auditor == nil {
		auditor = &mockAuditor{}
	}

	var tlsConfig *tls.Config
	if pt.useCertManager {
		currentUser, err := user.Current()
		require.NoError(pt.t, err, "Failed to get current user")

		uid, _ := strconv.Atoi(currentUser.Uid)
		gid, _ := strconv.Atoi(currentUser.Gid)

		certManager, err := boundary_tls.NewCertificateManager(boundary_tls.Config{
			Logger:    logger,
			ConfigDir: pt.configDir,
			Uid:       uid,
			Gid:       gid,
		})
		require.NoError(pt.t, err, "Failed to create certificate manager")

		tlsConfig, err = certManager.SetupTLSAndWriteCACert()
		require.NoError(pt.t, err, "Failed to setup TLS")
	} else {
		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	pt.server = NewProxyServer(Config{
		HTTPPort:   pt.port,
		RuleEngine: ruleEngine,
		Auditor:    auditor,
		Logger:     logger,
		TLSConfig:  tlsConfig,
	})

	err = pt.server.Start()
	require.NoError(pt.t, err, "Failed to start server")

	// Give server time to start
	time.Sleep(pt.startupDelay)

	// Create HTTP client for direct proxy requests
	pt.client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // G402: Skip cert verification for testing
			},
		},
		Timeout: 5 * time.Second,
	}

	// Create HTTP client for proxy transport (implicit CONNECT)
	proxyURL, err := url.Parse("http://localhost:" + strconv.Itoa(pt.port))
	require.NoError(pt.t, err, "Failed to parse proxy URL")

	pt.proxyClient = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // G402: Skip cert verification for testing
			},
		},
		Timeout: 10 * time.Second,
	}

	return pt
}

// Stop gracefully stops the proxy server
func (pt *ProxyTest) Stop() {
	if pt.server != nil {
		err := pt.server.Stop()
		if err != nil {
			pt.t.Logf("Failed to stop proxy server: %v", err)
		}
	}
}

// ExpectAllowed makes a request through the proxy and expects it to be allowed with the given response body
func (pt *ProxyTest) ExpectAllowed(proxyURL, hostHeader, expectedBody string) {
	pt.t.Helper()

	req, err := http.NewRequest("GET", proxyURL, nil)
	require.NoError(pt.t, err, "Failed to create request")
	req.Host = hostHeader

	resp, err := pt.client.Do(req)
	require.NoError(pt.t, err, "Failed to make request")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(pt.t, err, "Failed to read response body")

	require.Equal(pt.t, expectedBody, string(body), "Expected response body does not match")
}

// ExpectAllowedContains makes a request through the proxy and expects it to be allowed, checking that response contains the given text
func (pt *ProxyTest) ExpectAllowedContains(proxyURL, hostHeader, containsText string) {
	pt.t.Helper()

	req, err := http.NewRequest("GET", proxyURL, nil)
	require.NoError(pt.t, err, "Failed to create request")
	req.Host = hostHeader

	resp, err := pt.client.Do(req)
	require.NoError(pt.t, err, "Failed to make request")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(pt.t, err, "Failed to read response body")

	require.Contains(pt.t, string(body), containsText, "Response does not contain expected text")
}

// ExpectDeny makes a request through the proxy and expects it to be denied
func (pt *ProxyTest) ExpectDeny(proxyURL, hostHeader string) {
	pt.t.Helper()

	req, err := http.NewRequest("GET", proxyURL, nil)
	require.NoError(pt.t, err, "Failed to create request")
	req.Host = hostHeader

	resp, err := pt.client.Do(req)
	require.NoError(pt.t, err, "Failed to make request")
	defer resp.Body.Close()

	require.Equal(pt.t, http.StatusForbidden, resp.StatusCode, "Expected 403 Forbidden status")

	body, err := io.ReadAll(resp.Body)
	require.NoError(pt.t, err, "Failed to read response body")

	require.Contains(pt.t, string(body), "Request Blocked by Boundary", "Expected request to be blocked")
}

// ExpectDenyViaProxy makes a request through the proxy using proxy transport (implicit CONNECT for HTTPS)
// and expects it to be denied
func (pt *ProxyTest) ExpectDenyViaProxy(targetURL string) {
	pt.t.Helper()

	resp, err := pt.proxyClient.Get(targetURL)
	require.NoError(pt.t, err, "Failed to make request via proxy")
	defer resp.Body.Close()

	require.Equal(pt.t, http.StatusForbidden, resp.StatusCode, "Expected 403 Forbidden status")

	body, err := io.ReadAll(resp.Body)
	require.NoError(pt.t, err, "Failed to read response body")

	require.Contains(pt.t, string(body), "Request Blocked by Boundary", "Expected request to be blocked")
}

// ExpectAllowedViaProxy makes a request through the proxy using proxy transport (implicit CONNECT for HTTPS)
// and expects it to be allowed with the given response body
func (pt *ProxyTest) ExpectAllowedViaProxy(targetURL, expectedBody string) {
	pt.t.Helper()

	resp, err := pt.proxyClient.Get(targetURL)
	require.NoError(pt.t, err, "Failed to make request via proxy")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(pt.t, err, "Failed to read response body")

	require.Equal(pt.t, expectedBody, string(body), "Expected response body does not match")
}

// ExpectAllowedContainsViaProxy makes a request through the proxy using proxy transport (implicit CONNECT for HTTPS)
// and expects it to be allowed, checking that response contains the given text
func (pt *ProxyTest) ExpectAllowedContainsViaProxy(targetURL, containsText string) {
	pt.t.Helper()

	resp, err := pt.proxyClient.Get(targetURL)
	require.NoError(pt.t, err, "Failed to make request via proxy")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(pt.t, err, "Failed to read response body")

	require.Contains(pt.t, string(body), containsText, "Response does not contain expected text")
}

// explicitCONNECTTunnel represents an established CONNECT tunnel
type explicitCONNECTTunnel struct {
	tlsConn *tls.Conn
	reader  *bufio.Reader
}

// establishExplicitCONNECT establishes a CONNECT tunnel and returns a tunnel object
// targetHost should be in format "hostname:port" (e.g., "dev.coder.com:443")
func (pt *ProxyTest) establishExplicitCONNECT(targetHost string) (*explicitCONNECTTunnel, error) {
	pt.t.Helper()

	// Extract hostname for TLS ServerName (remove port if present)
	hostParts := strings.Split(targetHost, ":")
	serverName := hostParts[0]

	// Connect to proxy
	conn, err := net.Dial("tcp", "localhost:"+strconv.Itoa(pt.port))
	if err != nil {
		return nil, err
	}

	// Send explicit CONNECT request
	connectReq := "CONNECT " + targetHost + " HTTP/1.1\r\n" +
		"Host: " + targetHost + "\r\n" +
		"\r\n"
	_, err = conn.Write([]byte(connectReq))
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Read CONNECT response
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != 200 {
		conn.Close()
		return nil, xerrors.Errorf("CONNECT failed with status: %d", resp.StatusCode)
	}

	// Wrap connection with TLS client
	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: true, // G402: Skip cert verification for testing
		ServerName:         serverName,
	})

	// Perform TLS handshake
	err = tlsConn.Handshake()
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &explicitCONNECTTunnel{
		tlsConn: tlsConn,
		reader:  bufio.NewReader(tlsConn),
	}, nil
}

// sendRequest sends an HTTP request over the tunnel and returns the response body
func (tunnel *explicitCONNECTTunnel) sendRequest(targetHost, path string) ([]byte, error) {
	// Send HTTP request over the tunnel
	httpReq := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + targetHost + "\r\n" +
		"Connection: keep-alive\r\n" +
		"\r\n"
	_, err := tunnel.tlsConn.Write([]byte(httpReq))
	if err != nil {
		return nil, err
	}

	// Read HTTP response
	httpResp, err := http.ReadResponse(tunnel.reader, nil)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// sendRequestAndExpectDeny sends an HTTP request over the tunnel and expects it to be denied
func (tunnel *explicitCONNECTTunnel) sendRequestAndExpectDeny(targetHost, path string) error {
	// Send HTTP request over the tunnel
	httpReq := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + targetHost + "\r\n" +
		"Connection: keep-alive\r\n" +
		"\r\n"
	_, err := tunnel.tlsConn.Write([]byte(httpReq))
	if err != nil {
		return err
	}

	// Read HTTP response
	httpResp, err := http.ReadResponse(tunnel.reader, nil)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusForbidden {
		return xerrors.Errorf("expected 403 Forbidden, got %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}

	if !strings.Contains(string(body), "Request Blocked by Boundary") {
		return xerrors.Errorf("expected blocked response, got: %s", string(body))
	}

	return nil
}

// close closes the tunnel connection
func (tunnel *explicitCONNECTTunnel) close() error {
	return tunnel.tlsConn.Close()
}
