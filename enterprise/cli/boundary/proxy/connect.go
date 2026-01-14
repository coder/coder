// Package proxy implements HTTP CONNECT method for tunneling HTTPS traffic through a proxy.
//
// # HTTP CONNECT Method Overview
//
// The HTTP CONNECT method is used to establish a tunnel through a proxy server.
// This is essential for HTTPS proxying because HTTPS requires end-to-end encryption
// that cannot be inspected or modified by intermediaries.
//
// How HTTP_PROXY Works
//
// When a client is configured to use an HTTP proxy (via HTTP_PROXY environment variable
// or proxy settings), it behaves differently for HTTP vs HTTPS requests:
//
//   - HTTP requests: The client sends the full request to the proxy, including the
//     complete URL. The proxy forwards it to the destination server.
//
//   - HTTPS requests: The client cannot send the encrypted request directly because
//     the proxy needs to know where to connect. Instead, the client uses CONNECT
//     to establish a tunnel, then performs the TLS handshake and sends HTTPS
//     requests through that tunnel.
//
// # Non-Transparent Proxy
//
// This proxy is "non-transparent" because:
//   - Clients must be explicitly configured to use it (via HTTP_PROXY)
//   - Clients send CONNECT requests for HTTPS traffic
//   - The proxy terminates TLS, inspects requests, and re-encrypts to the destination
//   - Each HTTP request inside the tunnel is processed separately with rule evaluation
//
// # CONNECT Request Flow
//
// The following diagram illustrates how CONNECT works:
//
//	Client                    Proxy (HTTP/1.1 Server)              Real Server
//	  |                              |                                    |
//	  |-- CONNECT example.com:443 -->|                                    |
//	  |                              |                                    |
//	  |<-- 200 Connection Established|                                    |
//	  |                              |                                    |
//	  |-- TLS Handshake ------------->|                                    |
//	  |                              |                                    |
//	  |<-- TLS Handshake -------------|                                    |
//	  |                              |                                    |
//	  |-- Request #1: GET /page1 --->| (decrypts)                         |
//	  |                              |-- GET /page1 --------------------->|
//	  |                              |<-- Response #1 --------------------|
//	  |<-- Response #1 --------------| (encrypts)                         |
//	  |                              |                                    |
//	  |-- Request #2: GET /page2 --->| (decrypts)                         |
//	  |                              |-- GET /page2 --------------------->|
//	  |                              |<-- Response #2 --------------------|
//	  |<-- Response #2 --------------| (encrypts)                         |
//	  |                              |                                    |
//	  |-- Request #3: GET /api ----->| (decrypts)                         |
//	  |                              |-- GET /api ----------------------->|
//	  |                              |<-- Response #3 --------------------|
//	  |<-- Response #3 --------------| (encrypts)                         |
//	  |                              |                                    |
//	  | (connection stays open...)   |                                    |
//	  |                              |                                    |
//	  |-- [closes connection] ------->|                                    |
//
// Key Points:
//
//  1. CONNECT establishes the tunnel endpoint (e.g., "example.com:443")
//  2. The actual destination for each request is determined by the Host header
//     in the HTTP request inside the tunnel, not the CONNECT target
//  3. The proxy acts as a TLS server to decrypt traffic from the client
//  4. Each HTTP request inside the tunnel is evaluated against rules separately
//  5. The connection remains open for multiple requests (HTTP/1.1 keep-alive)
//
// Implementation Details:
//
//   - handleCONNECT: Receives the CONNECT request, sends "200 Connection Established"
//   - handleCONNECTTunnel: Wraps the connection with TLS, processes requests in a loop
//   - Each request uses req.Host to determine the actual destination, not the CONNECT target
//
//nolint:revive,gocritic,errname,unconvert,noctx,errorlint,bodyclose
package proxy

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"
)

// handleCONNECT handles HTTP CONNECT requests for tunneling.
//
// When a client wants to make an HTTPS request through the proxy, it first sends
// a CONNECT request with the target hostname:port (e.g., "example.com:443").
// The proxy responds with "200 Connection Established" and then the client
// performs a TLS handshake over the same connection.
//
// After the tunnel is established, handleCONNECTTunnel processes the encrypted
// traffic and handles each HTTP request inside the tunnel separately.
func (p *Server) handleCONNECT(conn net.Conn, req *http.Request) {
	p.logger.Debug("🔌 CONNECT request", "target", req.Host)

	// Send 200 Connection established response
	response := "HTTP/1.1 200 Connection established\r\n\r\n"
	_, err := conn.Write([]byte(response))
	if err != nil {
		p.logger.Error("Failed to send CONNECT response", "error", err)
		return
	}

	p.logger.Debug("CONNECT tunnel established", "target", req.Host)

	// Handle the tunnel - decrypt TLS and process each HTTP request
	p.handleCONNECTTunnel(conn)
}

// handleCONNECTTunnel handles the tunnel after CONNECT is established.
//
// This function:
//  1. Wraps the connection with TLS.Server to decrypt traffic from the client
//  2. Performs the TLS handshake
//  3. Reads HTTP requests from the tunnel in a loop
//  4. Processes each request separately (rule evaluation, forwarding)
//
// Important: The actual destination for each request is determined by the Host
// header in the HTTP request, not the CONNECT target. This allows multiple
// domains to be accessed over the same tunnel.
//
// The connection lifecycle is managed by handleHTTPConnection's defer, which
// closes the connection when this function returns.
func (p *Server) handleCONNECTTunnel(conn net.Conn) {
	// Wrap connection with TLS server to decrypt traffic
	tlsConn := tls.Server(conn, p.tlsConfig)

	// Perform TLS handshake
	if err := tlsConn.Handshake(); err != nil {
		p.logger.Error("TLS handshake failed in CONNECT tunnel", "error", err)
		return
	}

	p.logger.Debug("✅ TLS handshake successful in CONNECT tunnel")

	// Process HTTP requests in a loop
	reader := bufio.NewReader(tlsConn)
	for {
		// Read HTTP request from tunnel
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err == io.EOF {
				p.logger.Debug("CONNECT tunnel closed by client")
				break
			}
			p.logger.Error("Failed to read HTTP request from CONNECT tunnel", "error", err)
			break
		}

		p.logger.Debug("🔒 HTTP Request in CONNECT tunnel", "method", req.Method, "url", req.URL.String(), "target", req.Host)

		// Process this request - check if allowed and forward to target
		p.processHTTPRequest(tlsConn, req, true)
	}
}
