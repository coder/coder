package confine

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/xerrors"
)

const (
	fallbackDNSUpstream             = "1.1.1.1:53"
	resolvConfPath                  = "/etc/resolv.conf"
	defaultDNSQueriesPerSecond      = 100
	defaultDNSQueryBurst            = 200
	defaultDNSMaxConcurrentUpstream = 64
	defaultDNSTransportTimeout      = 5 * time.Second
)

// DNSQueryDecision describes the result of handling a DNS query.
type DNSQueryDecision string

const (
	// DNSQueryDecisionAllowed indicates that the query passed policy and was
	// answered by the upstream resolver.
	DNSQueryDecisionAllowed DNSQueryDecision = "allowed"
	// DNSQueryDecisionDenied indicates that the relay refused the query.
	DNSQueryDecisionDenied DNSQueryDecision = "denied"
	// DNSQueryDecisionError indicates that an allowed query could not be
	// answered by the upstream resolver.
	DNSQueryDecisionError DNSQueryDecision = "error"
)

// DNSQueryCallback receives every DNS query decision.
type DNSQueryCallback func(DNSQueryEvent)

// DNSQueryEvent records one DNS query decision.
type DNSQueryEvent struct {
	QName          string
	QType          uint16
	Decision       DNSQueryDecision
	PolicyRevision int64
}

// Relay enforces DNS policy and forwards allowed queries to an upstream
// resolver over UDP and TCP.
type Relay struct {
	udpServer     *dns.Server
	tcpServer     *dns.Server
	addr          net.Addr
	policy        *PolicyEngine
	onQuery       DNSQueryCallback
	upstream      string
	limiter       *dnsTokenBucket
	upstreamSlots chan struct{}

	wg       sync.WaitGroup
	close    sync.Once
	closeErr error
}

// ListenRelay starts a policy-enforcing DNS relay on address.
func ListenRelay(address string, policy *PolicyEngine, onQuery DNSQueryCallback) (*Relay, error) {
	return newRelay(address, policy, onQuery, resolverAddress(resolvConfPath))
}

func newRelay(address string, policy *PolicyEngine, onQuery DNSQueryCallback, upstream string) (*Relay, error) {
	if policy == nil {
		return nil, xerrors.New("DNS relay policy is required")
	}

	udpConn, err := net.ListenPacket("udp", address)
	if err != nil {
		return nil, xerrors.Errorf("listen DNS relay UDP: %w", err)
	}
	udpAddr := udpConn.LocalAddr()
	if udpAddr == nil {
		_ = udpConn.Close()
		return nil, xerrors.New("listen DNS relay UDP: missing local address")
	}
	tcpListener, err := net.Listen("tcp", udpAddr.String())
	if err != nil {
		_ = udpConn.Close()
		return nil, xerrors.Errorf("listen DNS relay TCP: %w", err)
	}

	relay := &Relay{
		addr:          udpAddr,
		policy:        policy,
		onQuery:       onQuery,
		upstream:      upstream,
		limiter:       newDNSTokenBucket(defaultDNSQueriesPerSecond, defaultDNSQueryBurst),
		upstreamSlots: make(chan struct{}, defaultDNSMaxConcurrentUpstream),
	}
	relay.udpServer = &dns.Server{
		PacketConn:    udpConn,
		Handler:       relay,
		ReadTimeout:   defaultDNSTransportTimeout,
		WriteTimeout:  defaultDNSTransportTimeout,
		MsgAcceptFunc: acceptDNSMessage,
	}
	relay.tcpServer = &dns.Server{
		Listener:      tcpListener,
		Handler:       relay,
		ReadTimeout:   defaultDNSTransportTimeout,
		WriteTimeout:  defaultDNSTransportTimeout,
		MsgAcceptFunc: acceptDNSMessage,
	}
	if err := relay.start(relay.udpServer); err != nil {
		_ = udpConn.Close()
		_ = tcpListener.Close()
		relay.wg.Wait()
		return nil, xerrors.Errorf("start DNS relay UDP: %w", err)
	}
	if err := relay.start(relay.tcpServer); err != nil {
		_ = relay.udpServer.Shutdown()
		_ = tcpListener.Close()
		relay.wg.Wait()
		return nil, xerrors.Errorf("start DNS relay TCP: %w", err)
	}
	return relay, nil
}

// Addr returns the relay listener address. UDP and TCP share this port.
func (r *Relay) Addr() net.Addr {
	return r.addr
}

// Close stops both relay transports and waits for their serve loops.
func (r *Relay) Close() error {
	r.close.Do(func() {
		r.closeErr = errors.Join(r.udpServer.Shutdown(), r.tcpServer.Shutdown())
		r.wg.Wait()
	})
	return r.closeErr
}

// ServeDNS enforces policy and forwards an accepted DNS query.
func (r *Relay) ServeDNS(writer dns.ResponseWriter, request *dns.Msg) {
	event := DNSQueryEvent{Decision: DNSQueryDecisionDenied}
	policy := r.policy.policy.Load()
	if policy != nil {
		event.PolicyRevision = policy.revision
	}
	defer func() {
		r.emit(event)
	}()

	if request == nil {
		event.Decision = DNSQueryDecisionError
		return
	}
	if len(request.Question) > 0 {
		event.QName = normalizeHost(request.Question[0].Name)
		event.QType = request.Question[0].Qtype
	}
	if request.Opcode != dns.OpcodeQuery || len(request.Question) != 1 {
		writeDNSRcode(writer, request, dns.RcodeRefused)
		return
	}
	if !allowedDNSQueryType(event.QType) || !r.limiter.allow(time.Now()) {
		writeDNSRcode(writer, request, dns.RcodeRefused)
		return
	}

	decision := decideDNSName(policy, event.QName)
	if !decision.Allowed {
		writeDNSRcode(writer, request, dns.RcodeRefused)
		return
	}

	select {
	case r.upstreamSlots <- struct{}{}:
		defer func() {
			<-r.upstreamSlots
		}()
	default:
		event.Decision = DNSQueryDecisionError
		writeDNSRcode(writer, request, dns.RcodeServerFailure)
		return
	}

	response, err := r.exchange(request)
	if err != nil || response == nil {
		event.Decision = DNSQueryDecisionError
		writeDNSRcode(writer, request, dns.RcodeServerFailure)
		return
	}
	event.Decision = DNSQueryDecisionAllowed
	_ = writer.WriteMsg(response)
}

func (r *Relay) start(server *dns.Server) error {
	started := make(chan struct{})
	result := make(chan error, 1)
	server.NotifyStartedFunc = func() {
		close(started)
	}
	r.wg.Go(func() {
		result <- server.ActivateAndServe()
	})
	select {
	case <-started:
		return nil
	case err := <-result:
		if err == nil {
			return xerrors.New("DNS server stopped before startup")
		}
		return err
	}
}

func acceptDNSMessage(header dns.Header) dns.MsgAcceptAction {
	const responseBit = 1 << 15
	if header.Bits&responseBit != 0 {
		return dns.MsgIgnore
	}
	return dns.MsgAccept
}

func allowedDNSQueryType(queryType uint16) bool {
	switch queryType {
	case dns.TypeA, dns.TypeAAAA, dns.TypeCNAME, dns.TypeHTTPS, dns.TypeSVCB:
		return true
	default:
		return false
	}
}

func decideDNSName(policy *compiledPolicy, name string) Decision {
	if policy == nil {
		return Decision{}
	}
	name = normalizeHost(name)
	for _, rule := range policy.rules {
		// DNS authorization is name-only. A matching rule allows resolution
		// regardless of the ports that constrain later connection decisions.
		for port := range rule.ports {
			if rule.matches(name, port) {
				return Decision{Allowed: true, Revision: policy.revision}
			}
		}
	}
	return Decision{Revision: policy.revision}
}

func (r *Relay) exchange(request *dns.Msg) (*dns.Msg, error) {
	udpClient := &dns.Client{
		Net:          "udp",
		DialTimeout:  defaultDNSTransportTimeout,
		ReadTimeout:  defaultDNSTransportTimeout,
		WriteTimeout: defaultDNSTransportTimeout,
	}
	response, _, err := udpClient.Exchange(request, r.upstream)
	if err != nil || response == nil || !response.Truncated {
		return response, err
	}
	tcpClient := &dns.Client{
		Net:          "tcp",
		DialTimeout:  defaultDNSTransportTimeout,
		ReadTimeout:  defaultDNSTransportTimeout,
		WriteTimeout: defaultDNSTransportTimeout,
	}
	response, _, err = tcpClient.Exchange(request, r.upstream)
	return response, err
}

type dnsTokenBucket struct {
	mu sync.Mutex

	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newDNSTokenBucket(queriesPerSecond, burst int) *dnsTokenBucket {
	now := time.Now()
	return &dnsTokenBucket{
		rate:   float64(queriesPerSecond),
		burst:  float64(burst),
		tokens: float64(burst),
		last:   now,
	}
}

func (b *dnsTokenBucket) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(b.burst, b.tokens+elapsed*b.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func writeDNSRcode(writer dns.ResponseWriter, request *dns.Msg, rcode int) {
	response := new(dns.Msg).SetRcode(request, rcode)
	_ = writer.WriteMsg(response)
}

func (r *Relay) emit(event DNSQueryEvent) {
	if r.onQuery != nil {
		r.onQuery(event)
	}
}

func resolverAddress(path string) string {
	config, err := dns.ClientConfigFromFile(path)
	if err != nil || len(config.Servers) == 0 {
		return fallbackDNSUpstream
	}
	port := config.Port
	if port == "" {
		port = "53"
	}
	return net.JoinHostPort(config.Servers[0], port)
}

var _ dns.Handler = (*Relay)(nil)
