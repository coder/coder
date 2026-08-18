package confine

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/xerrors"

	sandboxpolicy "github.com/coder/coder/coder-sandbox/policy"
	sandboxproxy "github.com/coder/coder/coder-sandbox/proxy"
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

const destinationTimeout = 10 * time.Second

var errDestinationDenied = xerrors.New("destination resolves only to denied addresses")

// deniedDestinationRanges prevent the host-side proxy from reaching local,
// private, link-local, unspecified, or multicast networks on a client's behalf.
var deniedDestinationRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// DestinationOptions controls how an egress listener resolves and validates
// policy-allowed destinations.
type DestinationOptions struct {
	// LookupNetIP resolves each hostname once. Nil uses net.DefaultResolver.
	LookupNetIP func(ctx context.Context, network, host string) ([]netip.Addr, error)
	// AllowPrivateHost exempts exactly one hostname from denied-range checks.
	// This supports a coderd endpoint on a private address without weakening
	// validation for any other policy destination. It does not bypass policy.
	AllowPrivateHost string
	// AlwaysAllowHost and AlwaysAllowPort identify one exact destination that
	// bypasses the template policy. When set, AlwaysAllowHost also becomes the
	// private-host exemption so the two control-channel checks cannot diverge.
	AlwaysAllowHost string
	AlwaysAllowPort int
}

type resolvedDestination struct {
	addresses []netip.Addr
	port      int
}

// Proxy serves CONNECT, absolute-form, and origin-form HTTP requests.
type Proxy struct {
	listener    net.Listener
	server      *http.Server
	policy      *PolicyEngine
	event       EventCallback
	destination DestinationOptions
	wg          sync.WaitGroup
}

// ListenProxy starts an egress proxy on address.
func ListenProxy(address string, policy *PolicyEngine, event EventCallback) (*Proxy, error) {
	return ListenProxyWithOptions(address, policy, event, DestinationOptions{})
}

// ListenProxyWithOptions starts an egress proxy with destination validation
// options.
func ListenProxyWithOptions(address string, policy *PolicyEngine, event EventCallback, options DestinationOptions) (*Proxy, error) {
	if options.LookupNetIP != nil {
		// The sandbox proxy does not expose resolver or dialer injection. Retain
		// the existing path when callers require deterministic resolution.
		return listenLegacyProxyWithOptions(address, policy, event, options)
	}
	return listenSandboxProxy(address, policy, event, options)
}

func listenSandboxProxy(address string, engine *PolicyEngine, event EventCallback, options DestinationOptions) (*Proxy, error) {
	ca, caPEM, err := ephemeralProxyCA()
	if err != nil {
		return nil, err
	}
	evaluator := newPolicyEvaluator(engine, options)
	recorder := newSandboxEventRecorder(event)
	server, err := sandboxproxy.New(ca, caPEM, recorder)
	if err != nil {
		return nil, xerrors.Errorf("create sandbox egress proxy: %w", err)
	}
	subject := &sandboxproxy.Subject{
		ID:     "agent-confine",
		Name:   "AI egress proxy",
		Policy: evaluator,
	}
	connContext, err := sandboxproxy.SubjectConnContext(subject)
	if err != nil {
		return nil, xerrors.Errorf("configure sandbox egress proxy subject: %w", err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, xerrors.Errorf("listen egress proxy: %w", err)
	}
	options.AllowPrivateHost = normalizeHost(options.AllowPrivateHost)
	proxy := &Proxy{
		listener:    listener,
		policy:      engine,
		event:       event,
		destination: options,
	}
	proxy.server = &http.Server{
		Handler: sandboxRequestHandler{
			next:      server.Handler(),
			evaluator: evaluator,
			event:     event,
			legacy:    proxy,
		},
		ReadHeaderTimeout: 10 * time.Second,
		ConnContext:       connContext,
	}
	proxy.start()
	return proxy, nil
}

func listenLegacyProxy(address string, policy *PolicyEngine, event EventCallback) (*Proxy, error) {
	return listenLegacyProxyWithOptions(address, policy, event, DestinationOptions{})
}

func listenLegacyProxyWithOptions(address string, policy *PolicyEngine, event EventCallback, options DestinationOptions) (*Proxy, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, xerrors.Errorf("listen egress proxy: %w", err)
	}
	options.AllowPrivateHost = normalizeHost(options.AllowPrivateHost)
	proxy := &Proxy{
		listener:    listener,
		policy:      policy,
		event:       event,
		destination: options,
	}
	proxy.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}
	proxy.start()
	return proxy, nil
}

func (p *Proxy) start() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		_ = p.server.Serve(p.listener)
	}()
}

// ephemeralProxyCA satisfies the sandbox proxy constructor without enabling
// interception. The evaluator always returns TLSPassthrough, so this CA never
// signs a leaf. Callers may keep it in memory or plant its public certificate.
func ephemeralProxyCA() (tls.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, xerrors.Errorf("generate ephemeral proxy CA key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, nil, xerrors.Errorf("generate ephemeral proxy CA serial: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Coder AI Egress Proxy Ephemeral CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, xerrors.Errorf("create ephemeral proxy CA: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, nil, xerrors.Errorf("marshal ephemeral proxy CA key: %w", err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	ca, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, xerrors.Errorf("load ephemeral proxy CA: %w", err)
	}
	ca.Leaf, err = x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, xerrors.Errorf("parse ephemeral proxy CA: %w", err)
	}
	return ca, certificatePEM, nil
}

type sandboxRequestHandler struct {
	next      http.Handler
	evaluator *policyEvaluator
	event     EventCallback
	legacy    *Proxy
}

func (h sandboxRequestHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect {
		host, port, err := authority(req.Host, defaultHTTPSPort)
		if err != nil {
			http.Error(rw, "invalid proxy destination", http.StatusBadRequest)
			return
		}
		if !h.allowed(req.Context(), agentsdk.AISandboxNetworkProtocolConnect, host, port) {
			deny(rw, host)
			return
		}
		h.next.ServeHTTP(rw, req)
		return
	}
	if req.URL == nil {
		http.Error(rw, "invalid proxy destination", http.StatusBadRequest)
		return
	}
	if req.URL.Scheme == "" && req.URL.Host == "" {
		// Origin-form traffic remains on the existing handler. This is the
		// request shape produced by transparent netns redirection, which the
		// sandbox proxy does not accept as an explicit proxy request.
		h.legacy.forwardHTTP(rw, req)
		return
	}
	if req.URL.Scheme != "http" || req.URL.Host == "" {
		http.Error(rw, "absolute-form http URL required", http.StatusBadRequest)
		return
	}
	host, port, err := authority(req.URL.Host, defaultHTTPPort)
	if err != nil {
		http.Error(rw, "invalid proxy destination", http.StatusBadRequest)
		return
	}
	if !h.allowed(req.Context(), agentsdk.AISandboxNetworkProtocolHTTP, host, port) {
		deny(rw, host)
		return
	}
	h.next.ServeHTTP(rw, req)
}

func (h sandboxRequestHandler) allowed(ctx context.Context, protocol agentsdk.AISandboxNetworkProtocol, host string, port int) bool {
	//nolint:gosec // authority validates the port is between 1 and 65535.
	decision, err := h.evaluator.EvaluateName(ctx, host, uint16(port))
	if err == nil && decision.Action == sandboxpolicy.ActionAllow {
		return true
	}
	generation := h.evaluator.Generation()
	if err == nil {
		generation = decision.Generation
	}
	if h.event != nil {
		h.event(NetworkEvent{
			Protocol:       protocol,
			Host:           normalizeHost(host),
			Port:           port,
			Action:         agentsdk.AISandboxNetworkEventActionDenied,
			PolicyRevision: generation,
		})
	}
	return false
}

type sandboxEventKey struct {
	session int64
	kind    sandboxproxy.Kind
}

type sandboxEventRecorder struct {
	mu      sync.Mutex
	event   EventCallback
	pending map[sandboxEventKey]sandboxproxy.Event
}

func newSandboxEventRecorder(event EventCallback) *sandboxEventRecorder {
	return &sandboxEventRecorder{
		event:   event,
		pending: make(map[sandboxEventKey]sandboxproxy.Event),
	}
}

func (r *sandboxEventRecorder) Record(event sandboxproxy.Event) {
	if r == nil || r.event == nil {
		return
	}
	kind, ok := sandboxDecisionKind(event)
	if !ok {
		return
	}
	key := sandboxEventKey{session: event.Session, kind: kind}

	var terminal sandboxproxy.Event
	emit := false
	r.mu.Lock()
	switch event.Kind {
	case sandboxproxy.KindConnect, sandboxproxy.KindRequest:
		if event.Action == "" {
			break
		}
		if event.Phase == "" && event.Action == sandboxpolicy.ActionAllow {
			r.pending[key] = event
			break
		}
		delete(r.pending, key)
		terminal = event
		emit = true
	case sandboxproxy.KindResolution:
		pending, found := r.pending[key]
		if !found {
			break
		}
		address, err := netip.ParseAddr(event.ResolvedIP)
		_, literalErr := netip.ParseAddr(strings.Trim(pending.Host, "[]"))
		if literalErr == nil || err != nil || !sandboxpolicy.RequiresResolvedIPEvaluation(address) {
			delete(r.pending, key)
			terminal = pending
			emit = true
		}
	case sandboxproxy.KindError:
		pending, found := r.pending[key]
		if found {
			delete(r.pending, key)
			terminal = pending
			if strings.Contains(event.Err, "policy changed") {
				terminal.Action = sandboxpolicy.ActionDeny
				terminal.Generation = event.Generation
			}
			emit = true
		}
	}
	r.mu.Unlock()
	if !emit {
		return
	}
	networkEvent, ok := networkEventFromSandbox(terminal)
	if ok {
		r.event(networkEvent)
	}
}

func sandboxDecisionKind(event sandboxproxy.Event) (sandboxproxy.Kind, bool) {
	switch event.Kind {
	case sandboxproxy.KindConnect, sandboxproxy.KindRequest:
		return event.Kind, true
	case sandboxproxy.KindResolution, sandboxproxy.KindError:
		if event.Method == "" {
			return sandboxproxy.KindConnect, true
		}
		return sandboxproxy.KindRequest, true
	default:
		return "", false
	}
}

func networkEventFromSandbox(event sandboxproxy.Event) (NetworkEvent, bool) {
	port, err := strconv.Atoi(event.Port)
	if err != nil || !validPort(port) {
		return NetworkEvent{}, false
	}
	protocol := agentsdk.AISandboxNetworkProtocolHTTP
	if event.Kind == sandboxproxy.KindConnect {
		protocol = agentsdk.AISandboxNetworkProtocolConnect
	}
	action := agentsdk.AISandboxNetworkEventActionDenied
	if event.Action == sandboxpolicy.ActionAllow {
		action = agentsdk.AISandboxNetworkEventActionAllowed
	}
	return NetworkEvent{
		Protocol:       protocol,
		Host:           normalizeHost(event.Host),
		Port:           port,
		Action:         action,
		PolicyRevision: event.Generation,
	}, true
}

var _ sandboxproxy.Recorder = (*sandboxEventRecorder)(nil)

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

// ServeHTTP handles CONNECT tunnels and HTTP forwarding.
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
	if !decision.Allowed {
		p.emit(agentsdk.AISandboxNetworkProtocolConnect, host, port, decision)
		deny(rw, host)
		return
	}

	destination, err := p.destination.resolve(req.Context(), host, port)
	if errors.Is(err, errDestinationDenied) {
		decision.Allowed = false
		p.emit(agentsdk.AISandboxNetworkProtocolConnect, host, port, decision)
		deny(rw, host)
		return
	}
	p.emit(agentsdk.AISandboxNetworkProtocolConnect, host, port, decision)
	if err != nil {
		http.Error(rw, "proxy upstream unavailable", http.StatusBadGateway)
		return
	}
	upstream, err := destination.dial(req.Context(), "tcp")
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
	if req.URL == nil {
		http.Error(rw, "invalid proxy destination", http.StatusBadRequest)
		return
	}

	out := req.Clone(req.Context())
	host, port, err := authority(req.URL.Host, defaultHTTPPort)
	if req.URL.Scheme == "" && req.URL.Host == "" {
		host, port, err = originAuthority(req.Host)
		if err == nil {
			out.URL.Scheme = "http"
			out.URL.Host = net.JoinHostPort(host, strconv.Itoa(port))
			out.Host = out.URL.Host
		}
	} else if req.URL.Scheme != "http" || req.URL.Host == "" {
		http.Error(rw, "absolute-form http URL required", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(rw, "invalid proxy destination", http.StatusBadRequest)
		return
	}

	decision := p.policy.Decide(host, port)
	if !decision.Allowed {
		p.emit(agentsdk.AISandboxNetworkProtocolHTTP, host, port, decision)
		deny(rw, host)
		return
	}

	destination, err := p.destination.resolve(req.Context(), host, port)
	if errors.Is(err, errDestinationDenied) {
		decision.Allowed = false
		p.emit(agentsdk.AISandboxNetworkProtocolHTTP, host, port, decision)
		deny(rw, host)
		return
	}
	p.emit(agentsdk.AISandboxNetworkProtocolHTTP, host, port, decision)
	if err != nil {
		http.Error(rw, "proxy upstream unavailable", http.StatusBadGateway)
		return
	}

	out.RequestURI = ""
	out.Header = req.Header.Clone()
	out.Header.Del("Proxy-Connection")
	transport := &http.Transport{
		Proxy:       nil,
		DialContext: destination.dialContext,
	}
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

func (o DestinationOptions) resolve(ctx context.Context, host string, port int) (resolvedDestination, error) {
	host = normalizeHost(host)
	addresses := make([]netip.Addr, 0, 1)
	if address, err := netip.ParseAddr(host); err == nil {
		addresses = append(addresses, address)
	} else {
		lookup := o.LookupNetIP
		if lookup == nil {
			lookup = net.DefaultResolver.LookupNetIP
		}
		lookupCtx, cancel := context.WithTimeout(ctx, destinationTimeout)
		defer cancel()
		resolved, err := lookup(lookupCtx, "ip", host)
		if err != nil {
			return resolvedDestination{}, xerrors.Errorf("resolve proxy destination: %w", err)
		}
		addresses = append(addresses, resolved...)
	}

	allowPrivate := host != "" && host == normalizeHost(o.AllowPrivateHost)
	permitted := make([]netip.Addr, 0, len(addresses))
	validAddresses := 0
	for _, address := range addresses {
		if !address.IsValid() {
			continue
		}
		validAddresses++
		if allowPrivate || !deniedDestination(address) {
			permitted = append(permitted, address)
		}
	}
	if len(permitted) == 0 {
		if validAddresses > 0 {
			return resolvedDestination{}, errDestinationDenied
		}
		return resolvedDestination{}, xerrors.New("proxy destination has no addresses")
	}
	return resolvedDestination{addresses: permitted, port: port}, nil
}

func deniedDestination(address netip.Addr) bool {
	address = address.Unmap().WithZone("")
	for _, denied := range deniedDestinationRanges {
		if denied.Contains(address) {
			return true
		}
	}
	return false
}

func (d resolvedDestination) dial(ctx context.Context, network string) (net.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, destinationTimeout)
	defer cancel()

	errs := make([]error, 0, len(d.addresses))
	for _, address := range d.addresses {
		conn, err := (&net.Dialer{}).DialContext(dialCtx, network, net.JoinHostPort(address.String(), strconv.Itoa(d.port)))
		if err == nil {
			return conn, nil
		}
		errs = append(errs, err)
		if dialCtx.Err() != nil {
			break
		}
	}
	return nil, xerrors.Errorf("dial vetted proxy destination: %w", errors.Join(errs...))
}

func (d resolvedDestination) dialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	return d.dial(ctx, network)
}

func originAuthority(value string) (string, int, error) {
	if value == "" || strings.ContainsAny(value, "/\\") || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", 0, xerrors.New("invalid origin-form host")
	}
	return authority(value, defaultHTTPPort)
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
