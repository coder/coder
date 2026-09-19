package agentegress

import (
	"container/list"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/codersdk"
)

const (
	// dnsRelayTarget is the CONNECT target the exit node recognizes as "resolve
	// these DNS messages yourself".
	dnsRelayTarget = "dns:53"
	// fakeIPTTL keeps clients coming back to the DNS proxy so the fake pool
	// stays in step with what clients connect to.
	fakeIPTTL = 1
	// dnsMaxUDPPayload is the classic DNS limit; larger answers are
	// truncated so the client retries over TCP, which we also serve.
	dnsMaxUDPPayload = 512
	// dnsExchangeTimeout bounds one upstream or exit node exchange.
	dnsExchangeTimeout = 5 * time.Second
	// dnsTCPIdleTimeout closes idle DNS-over-TCP client connections.
	dnsTCPIdleTimeout = 30 * time.Second
	// dnsListenRetries bounds the search for a port free for both UDP and
	// TCP.
	dnsListenRetries     = 8
	dnsDecisionCacheSize = 1024
	dnsMinTTL            = 5 * time.Second
	dnsMaxTTL            = 300 * time.Second
	dnsMaxCacheTTL       = 30 * time.Second
	dnsWarnInterval      = 30 * time.Second
)

// dnsServer resolves exempt names and synthesizes fake addresses for egress.
type dnsServer struct {
	logger    slog.Logger
	fake      *fakeIPPool
	exempt    func(name string) bool
	upstream  []netip.AddrPort
	dial      func(context.Context, string, string) (net.Conn, error)
	relay     *dnsRelay
	clock     quartz.Clock
	decisions *dnsDecisionCache

	warn rateLimiter

	udp  *net.UDPConn
	tcp  net.Listener
	addr netip.AddrPort
	wg   sync.WaitGroup
}

func (s *dnsServer) listen(ctx context.Context, host string, preferredPort uint16) error {
	var lc net.ListenConfig
	var lastErr error
	for _, port := range []uint16{preferredPort, 0} {
		for range dnsListenRetries {
			pc, err := lc.ListenPacket(ctx, "udp4", net.JoinHostPort(host, strconv.Itoa(int(port))))
			if err != nil {
				lastErr = err
				break
			}
			udp, ok := pc.(*net.UDPConn)
			if !ok {
				_ = pc.Close()
				return xerrors.Errorf("unexpected packet conn type %T", pc)
			}
			addr, err := addrPortOf(udp.LocalAddr())
			if err != nil {
				_ = udp.Close()
				return err
			}
			tcp, err := lc.Listen(ctx, "tcp4", addr.String())
			if err != nil {
				lastErr = err
				_ = udp.Close()
				continue
			}
			if port == 0 {
				s.logger.Warn(ctx, "preferred dns proxy port unavailable, using ephemeral port",
					slog.F("preferred_port", preferredPort), slog.F("listen_addr", addr), slog.Error(lastErr))
			}
			s.udp, s.tcp, s.addr = udp, tcp, addr
			return nil
		}
	}
	return xerrors.Errorf("listen dns tcp and udp: %w", lastErr)
}

func (s *dnsServer) serve(ctx context.Context) {
	s.wg.Go(func() { s.serveUDP(ctx) })
	s.wg.Go(func() { s.serveTCP(ctx) })
}

func (s *dnsServer) close() {
	_ = s.udp.Close()
	_ = s.tcp.Close()
	s.relay.close()
	s.wg.Wait()
}

func (s *dnsServer) serveUDP(ctx context.Context) {
	buf := make([]byte, maxFrameSize)
	for {
		n, src, err := s.udp.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			s.logger.Debug(ctx, "read dns query", slog.Error(err))
			continue
		}
		msg := slices.Clone(buf[:n])
		s.wg.Go(func() {
			resp := s.handle(ctx, msg, dnsMaxUDPPayload)
			if resp == nil {
				return
			}
			if _, err := s.udp.WriteToUDPAddrPort(resp, src); err != nil && ctx.Err() == nil {
				s.logger.Debug(ctx, "write dns response", slog.Error(err))
			}
		})
	}
}

func (s *dnsServer) serveTCP(ctx context.Context) {
	for {
		conn, err := s.tcp.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			s.logger.Debug(ctx, "accept dns tcp connection", slog.Error(err))
			continue
		}
		s.wg.Go(func() { s.serveTCPConn(ctx, conn) })
	}
}

func (s *dnsServer) serveTCPConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(dnsTCPIdleTimeout))
		msg, err := readFrame(conn)
		if err != nil {
			return
		}
		resp := s.handle(ctx, msg, maxFrameSize)
		if resp == nil || writeFrame(conn, resp) != nil {
			return
		}
	}
}

// handle answers one query, truncating responses larger than maxSize.
func (s *dnsServer) handle(ctx context.Context, msg []byte, maxSize int) []byte {
	var parser dnsmessage.Parser
	hdr, err := parser.Start(msg)
	if err != nil {
		return nil
	}
	q, err := parser.Question()
	if err != nil {
		return dnsReply(hdr, nil, dnsmessage.RCodeFormatError)
	}
	if hdr.Response || hdr.OpCode != 0 {
		return dnsReply(hdr, &q, dnsmessage.RCodeNotImplemented)
	}
	resp := s.answer(ctx, hdr, q, msg)
	if len(resp) > maxSize {
		// Reply with just the question and TC set so the client retries
		// over TCP. TC is bit 9 of the flags word (RFC 1035 4.1.1).
		if resp = dnsReply(hdr, &q, dnsmessage.RCodeSuccess); resp != nil {
			resp[2] |= 0x02
		}
	}
	return resp
}

func (s *dnsServer) answer(ctx context.Context, hdr dnsmessage.Header, q dnsmessage.Question, msg []byte) []byte {
	name := normalizeName(q.Name.String())
	if q.Class != dnsmessage.ClassINET {
		return dnsReply(hdr, &q, dnsmessage.RCodeNotImplemented)
	}
	if s.exempt(name) {
		return s.forward(ctx, hdr, q, msg, s.exchangeUpstream, "system resolvers")
	}
	if q.Type == dnsmessage.TypeA || q.Type == dnsmessage.TypeAAAA {
		return s.answerAddress(ctx, hdr, q, msg, name)
	}
	synthesized := func(body dnsmessage.ResourceBody) []byte {
		return dnsReply(hdr, &q, dnsmessage.RCodeSuccess, dnsmessage.Resource{
			Header: dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: fakeIPTTL},
			Body:   body,
		})
	}
	if q.Type == dnsmessage.TypePTR {
		if addr, ok := parseReverseName(name); ok && s.fake.Contains(addr) {
			target, found := s.fake.Reverse(addr)
			if !found {
				return dnsReply(hdr, &q, dnsmessage.RCodeNameError)
			}
			ptr, err := dnsmessage.NewName(target + ".")
			if err != nil {
				return dnsReply(hdr, &q, dnsmessage.RCodeServerFailure)
			}
			return synthesized(&dnsmessage.PTRResource{PTR: ptr})
		}
	}
	return s.forward(ctx, hdr, q, msg, s.relay.exchange, "exit node")
}

type dnsDecision struct {
	rcode   dnsmessage.RCode
	expires time.Time
}

type dnsDecisionKey struct {
	name  string
	qtype dnsmessage.Type
}

type dnsDecisionEntry struct {
	key      dnsDecisionKey
	decision dnsDecision
}

type dnsDecisionCache struct {
	mu    sync.Mutex
	max   int
	byKey map[dnsDecisionKey]*list.Element
	lru   *list.List
}

func newDNSDecisionCache(maxEntries int) *dnsDecisionCache {
	return &dnsDecisionCache{max: maxEntries, byKey: make(map[dnsDecisionKey]*list.Element), lru: list.New()}
}

func (c *dnsDecisionCache) get(key dnsDecisionKey, now time.Time) (dnsDecision, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byKey[key]
	if !ok {
		return dnsDecision{}, false
	}
	entry, ok := e.Value.(dnsDecisionEntry)
	if !ok {
		return dnsDecision{}, false
	}
	if !now.Before(entry.decision.expires) {
		c.lru.Remove(e)
		delete(c.byKey, key)
		return dnsDecision{}, false
	}
	c.lru.MoveToFront(e)
	return entry.decision, true
}

func (c *dnsDecisionCache) put(key dnsDecisionKey, decision dnsDecision) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.byKey[key]; ok {
		e.Value = dnsDecisionEntry{key: key, decision: decision}
		c.lru.MoveToFront(e)
		return
	}
	e := c.lru.PushFront(dnsDecisionEntry{key: key, decision: decision})
	c.byKey[key] = e
	for c.lru.Len() > c.max {
		oldest := c.lru.Back()
		entry, ok := oldest.Value.(dnsDecisionEntry)
		if !ok {
			c.lru.Remove(oldest)
			continue
		}
		delete(c.byKey, entry.key)
		c.lru.Remove(oldest)
	}
}

func (c *dnsDecisionCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.byKey)
	c.lru.Init()
}

func (s *dnsServer) answerAddress(ctx context.Context, hdr dnsmessage.Header, q dnsmessage.Question, msg []byte, name string) []byte {
	now := s.clock.Now("dns_decision")
	key := dnsDecisionKey{name: name, qtype: q.Type}
	decision, ok := s.decisions.get(key, now)
	if !ok {
		exchangeCtx, cancel := context.WithTimeout(ctx, dnsExchangeTimeout)
		resp, err := s.relay.exchange(exchangeCtx, msg)
		cancel()
		if err != nil {
			s.warnForwardFailure(ctx, name, q.Type, err)
			return dnsReply(hdr, &q, dnsmessage.RCodeServerFailure)
		}
		decision, err = parseDNSDecision(resp, now)
		if err != nil {
			s.warnForwardFailure(ctx, name, q.Type, err)
			return dnsReply(hdr, &q, dnsmessage.RCodeServerFailure)
		}
		if decision.rcode == dnsmessage.RCodeSuccess || decision.rcode == dnsmessage.RCodeRefused || decision.rcode == dnsmessage.RCodeNameError {
			s.decisions.put(key, decision)
		}
	}
	if decision.rcode != dnsmessage.RCodeSuccess {
		return dnsReply(hdr, &q, decision.rcode)
	}
	if q.Type == dnsmessage.TypeAAAA {
		// An empty AAAA answer forces dual-stack clients onto the fake IPv4
		// path. IPv6-only destinations still work because the exit node
		// resolves the hostname from the CONNECT target itself.
		return dnsReply(hdr, &q, dnsmessage.RCodeSuccess)
	}
	addr, err := s.fake.Allocate(name)
	if err != nil {
		s.warnForwardFailure(ctx, name, q.Type, err)
		return dnsReply(hdr, &q, dnsmessage.RCodeServerFailure)
	}
	current := s.clock.Now("dns_decision_ttl")
	remaining := max(decision.expires.Sub(current), time.Second)
	seconds := min((remaining+time.Second-1)/time.Second, dnsMaxCacheTTL/time.Second)
	// #nosec G115 -- seconds is clamped to dnsMaxCacheTTL above.
	ttl := uint32(seconds)
	return dnsReply(hdr, &q, dnsmessage.RCodeSuccess, dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{Name: q.Name, Class: dnsmessage.ClassINET, TTL: ttl},
		Body:   &dnsmessage.AResource{A: addr.As4()},
	})
}

func parseDNSDecision(resp []byte, now time.Time) (dnsDecision, error) {
	var msg dnsmessage.Message
	if err := msg.Unpack(resp); err != nil {
		return dnsDecision{}, xerrors.Errorf("parse exit node dns response: %w", err)
	}
	ttl := uint32(dnsMaxTTL / time.Second)
	for _, resource := range slices.Concat(msg.Answers, msg.Authorities, msg.Additionals) {
		ttl = min(ttl, resource.Header.TTL)
	}
	ttl = min(max(ttl, uint32(dnsMinTTL/time.Second)), uint32(dnsMaxTTL/time.Second))
	cacheTTL := min(time.Duration(ttl)*time.Second, dnsMaxCacheTTL)
	return dnsDecision{rcode: msg.RCode, expires: now.Add(cacheTTL)}, nil
}

func (s *dnsServer) warnForwardFailure(ctx context.Context, name string, typ dnsmessage.Type, err error) {
	if !errors.Is(err, ErrNoLiveExitNodeReplicas) && s.warn.allow(s.clock.Now("dns_warning"), dnsWarnInterval) {
		s.logger.Warn(ctx, "dns decision through exit node failed",
			slog.F("name", name), slog.F("type", typ.String()), slog.Error(err))
	}
}

type dnsExchanger func(ctx context.Context, msg []byte) ([]byte, error)

func (s *dnsServer) forward(ctx context.Context, hdr dnsmessage.Header, q dnsmessage.Question, msg []byte, exchange dnsExchanger, via string) []byte {
	ctx, cancel := context.WithTimeout(ctx, dnsExchangeTimeout)
	defer cancel()
	resp, err := exchange(ctx, msg)
	if err != nil {
		if !errors.Is(err, ErrNoLiveExitNodeReplicas) {
			s.logger.Debug(ctx, "dns forward failed",
				slog.F("name", q.Name.String()),
				slog.F("type", q.Type.String()),
				slog.F("via", via),
				slog.Error(err),
			)
		}
		return dnsReply(hdr, &q, dnsmessage.RCodeServerFailure)
	}
	return resp
}

// exchangeUpstream tries each system resolver over TCP to avoid UDP capture.
func (s *dnsServer) exchangeUpstream(ctx context.Context, msg []byte) ([]byte, error) {
	if len(s.upstream) == 0 {
		return nil, xerrors.New("no upstream resolvers configured")
	}
	var errs []error
	for _, rs := range s.upstream {
		resp, err := exchangeTCP(ctx, s.dial, rs, msg)
		if err == nil {
			return resp, nil
		}
		errs = append(errs, xerrors.Errorf("%s: %w", rs, err))
	}
	return nil, errors.Join(errs...)
}

func resolverDialer() net.Dialer {
	return net.Dialer{Control: bypassControl}
}

func exchangeTCP(ctx context.Context, dial func(context.Context, string, string) (net.Conn, error), addr netip.AddrPort, msg []byte) ([]byte, error) {
	conn, err := dial(ctx, "tcp", addr.String())
	if err != nil {
		return nil, xerrors.Errorf("dial: %w", err)
	}
	defer conn.Close()
	// A zero deadline means none, matching a context without one.
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if err := writeFrame(conn, msg); err != nil {
		return nil, xerrors.Errorf("write query: %w", err)
	}
	resp, err := readFrame(conn)
	if err != nil {
		return nil, xerrors.Errorf("read response: %w", err)
	}
	return resp, nil
}

// dnsReply packs a response and omits a nil question for FORMERR.
func dnsReply(req dnsmessage.Header, q *dnsmessage.Question, rcode dnsmessage.RCode, answers ...dnsmessage.Resource) []byte {
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 req.ID,
			Response:           true,
			OpCode:             req.OpCode,
			RecursionDesired:   req.RecursionDesired,
			RecursionAvailable: true,
			RCode:              rcode,
		},
		Answers: answers,
	}
	if q != nil {
		msg.Questions = []dnsmessage.Question{*q}
	}
	out, err := msg.AppendPack(make([]byte, 0, dnsMaxUDPPayload))
	if err != nil {
		return nil
	}
	return out
}

func parseReverseName(name string) (netip.Addr, bool) {
	rest, ok := strings.CutSuffix(name, ".in-addr.arpa")
	if !ok {
		return netip.Addr{}, false
	}
	octets := strings.Split(rest, ".")
	slices.Reverse(octets)
	addr, err := netip.ParseAddr(strings.Join(octets, "."))
	return addr, err == nil && addr.Is4()
}

// dnsRelay multiplexes correlated queries over one CONNECT stream.
type dnsRelay struct {
	logger  slog.Logger
	connect connectFunc

	// streamMu serializes stream creation and frame writes.
	streamMu sync.Mutex
	mu       sync.Mutex
	conn     net.Conn
	pending  map[uint16]chan []byte
	nextID   uint16
	closed   bool
	wg       sync.WaitGroup
}

func newDNSRelay(logger slog.Logger, connect connectFunc) *dnsRelay {
	return &dnsRelay{
		logger:  logger,
		connect: connect,
		pending: make(map[uint16]chan []byte),
	}
}

func (r *dnsRelay) reset() {
	r.streamMu.Lock()
	defer r.streamMu.Unlock()
	r.mu.Lock()
	conn := r.conn
	r.conn = nil
	pending := r.pending
	r.pending = make(map[uint16]chan []byte)
	r.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	for _, ch := range pending {
		close(ch)
	}
}

func (r *dnsRelay) close() {
	r.mu.Lock()
	r.closed = true
	conn := r.conn
	r.conn = nil
	r.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	r.wg.Wait()
}

func (r *dnsRelay) exchange(ctx context.Context, msg []byte) ([]byte, error) {
	if len(msg) < 12 {
		return nil, xerrors.New("dns message too short")
	}
	r.mu.Lock()
	id, ok := r.allocIDLocked()
	if !ok {
		r.mu.Unlock()
		return nil, xerrors.New("too many in-flight dns queries")
	}
	ch := make(chan []byte, 1)
	r.pending[id] = ch
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.pending, id)
		r.mu.Unlock()
	}()

	out := slices.Clone(msg)
	binary.BigEndian.PutUint16(out, id)
	r.streamMu.Lock()
	conn, err := r.connLocked(ctx)
	if err == nil {
		if err = writeFrame(conn, out); err != nil {
			r.fail(conn, err)
			err = xerrors.Errorf("write dns query to exit node: %w", err)
		}
	}
	r.streamMu.Unlock()
	if err != nil {
		return nil, err
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, xerrors.New("exit node dns stream closed")
		}
		copy(resp, msg[:2])
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *dnsRelay) connLocked(ctx context.Context) (net.Conn, error) {
	r.mu.Lock()
	conn, closed := r.conn, r.closed
	r.mu.Unlock()
	if closed {
		return nil, xerrors.New("dns relay closed")
	}
	if conn != nil {
		return conn, nil
	}
	conn, err := r.connect(ctx, dnsRelayTarget, codersdk.ExitNodeProtocolDNS)
	if err != nil {
		return nil, xerrors.Errorf("open dns stream to exit node: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		_ = conn.Close()
		return nil, xerrors.New("dns relay closed")
	}
	r.conn = conn
	r.wg.Go(func() { r.readLoop(conn) })
	return conn, nil
}

func (r *dnsRelay) allocIDLocked() (uint16, bool) {
	for range 65536 {
		r.nextID++
		if _, taken := r.pending[r.nextID]; !taken {
			return r.nextID, true
		}
	}
	return 0, false
}

// readLoop dispatches responses until failure wakes all pending exchanges.
func (r *dnsRelay) readLoop(conn net.Conn) {
	for {
		resp, err := readFrame(conn)
		if err != nil {
			r.fail(conn, err)
			return
		}
		if len(resp) < 12 {
			continue
		}
		id := binary.BigEndian.Uint16(resp)
		r.mu.Lock()
		ch, ok := r.pending[id]
		delete(r.pending, id)
		r.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

func (r *dnsRelay) fail(conn net.Conn, err error) {
	r.mu.Lock()
	if r.conn != conn {
		r.mu.Unlock()
		return
	}
	r.conn = nil
	pending := r.pending
	r.pending = make(map[uint16]chan []byte)
	closed := r.closed
	r.mu.Unlock()
	_ = conn.Close()
	for _, ch := range pending {
		close(ch)
	}
	if !closed && !errors.Is(err, net.ErrClosed) {
		r.logger.Debug(context.Background(), "exit node dns stream broke, will reconnect", slog.Error(err))
	}
}
