package confine

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// NetworkEvent records one proxy policy decision.
type NetworkEvent struct {
	Protocol       agentsdk.AISandboxNetworkProtocol
	Host           string
	Port           int
	Action         agentsdk.AISandboxNetworkEventAction
	PolicyRevision int64
}

// EventCallback receives every allow or deny decision.
type EventCallback func(NetworkEvent)

// Proxy serves CONNECT and absolute-form plain HTTP proxy requests.
type Proxy struct {
	listener net.Listener
	server   *http.Server
	policy   *PolicyEngine
	event    EventCallback
	wg       sync.WaitGroup
}

// ListenProxy starts an egress proxy on address.
func ListenProxy(address string, policy *PolicyEngine, event EventCallback) (*Proxy, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, xerrors.Errorf("listen egress proxy: %w", err)
	}
	proxy := &Proxy{
		listener: listener,
		policy:   policy,
		event:    event,
	}
	proxy.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}
	proxy.wg.Add(1)
	go func() {
		defer proxy.wg.Done()
		_ = proxy.server.Serve(listener)
	}()
	return proxy, nil
}

// Addr returns the proxy listener address.
func (p *Proxy) Addr() net.Addr {
	return p.listener.Addr()
}

// Close stops the proxy and waits for its serve loop.
func (p *Proxy) Close() error {
	err := p.server.Close()
	p.wg.Wait()
	return err
}

// ServeHTTP handles CONNECT tunnels and absolute-form HTTP forwarding.
func (p *Proxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect {
		p.serveConnect(rw, req)
		return
	}
	p.forwardHTTP(rw, req)
}

func (p *Proxy) serveConnect(rw http.ResponseWriter, req *http.Request) {
	host, port, err := authority(req.Host, defaultHTTPSPort)
	if err != nil {
		http.Error(rw, "invalid proxy destination", http.StatusBadRequest)
		return
	}
	decision := p.policy.Decide(host, port)
	p.emit(agentsdk.AISandboxNetworkProtocolConnect, host, port, decision)
	if !decision.Allowed {
		deny(rw, host)
		return
	}

	upstream, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 10*time.Second)
	if err != nil {
		http.Error(rw, "proxy upstream unavailable", http.StatusBadGateway)
		return
	}

	hijacker, ok := rw.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(rw, "proxy tunnel unavailable", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	if err := buffered.Flush(); err != nil {
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	splice(&prefixedConn{Conn: client, reader: buffered.Reader}, upstream)
}

func (p *Proxy) forwardHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.URL == nil || req.URL.Scheme != "http" || req.URL.Host == "" {
		http.Error(rw, "absolute-form http URL required", http.StatusBadRequest)
		return
	}
	host, port, err := authority(req.URL.Host, defaultHTTPPort)
	if err != nil {
		http.Error(rw, "invalid proxy destination", http.StatusBadRequest)
		return
	}
	decision := p.policy.Decide(host, port)
	p.emit(agentsdk.AISandboxNetworkProtocolHTTP, host, port, decision)
	if !decision.Allowed {
		deny(rw, host)
		return
	}

	out := req.Clone(req.Context())
	out.RequestURI = ""
	out.Header = req.Header.Clone()
	out.Header.Del("Proxy-Connection")
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	res, err := transport.RoundTrip(out)
	if err != nil {
		http.Error(rw, "proxy upstream unavailable", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	copyHeader(rw.Header(), res.Header)
	rw.WriteHeader(res.StatusCode)
	_, _ = io.Copy(rw, res.Body)
}

func (p *Proxy) emit(protocol agentsdk.AISandboxNetworkProtocol, host string, port int, decision Decision) {
	if p.event == nil {
		return
	}
	action := agentsdk.AISandboxNetworkEventActionDenied
	if decision.Allowed {
		action = agentsdk.AISandboxNetworkEventActionAllowed
	}
	p.event(NetworkEvent{
		Protocol:       protocol,
		Host:           normalizeHost(host),
		Port:           port,
		Action:         action,
		PolicyRevision: decision.Revision,
	})
}

func authority(value string, defaultPort int) (string, int, error) {
	host, portString, err := net.SplitHostPort(value)
	if err == nil {
		port, err := strconv.Atoi(portString)
		if err != nil || !validPort(port) {
			return "", 0, xerrors.New("invalid port")
		}
		return normalizeHost(host), port, nil
	}
	if strings.Contains(err.Error(), "missing port in address") {
		host = normalizeHost(value)
		if host == "" {
			return "", 0, xerrors.New("empty host")
		}
		return host, defaultPort, nil
	}
	return "", 0, xerrors.Errorf("split host and port: %w", err)
}

func deny(rw http.ResponseWriter, host string) {
	rw.WriteHeader(http.StatusForbidden)
	_, _ = fmt.Fprintf(rw, "egress denied: %s\n", normalizeHost(host))
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func splice(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(left, right)
		closeWrite(left)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(right, left)
		closeWrite(right)
	}()
	wg.Wait()
	_ = left.Close()
	_ = right.Close()
}

func closeWrite(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
		return
	}
	_ = conn.Close()
}

var _ http.Handler = (*Proxy)(nil)
