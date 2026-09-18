package agentegress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
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
	dnsListenRetries = 8
)

// dnsServer answers the workspace's DNS queries. Names that are not
// exempt from egress get placeholder addresses so the eventual TCP or UDP
// flow can be tunneled by name; everything else is resolved by the system
// resolvers (exempt names) or by the exit node (record types the fake pool
// cannot synthesize).
type dnsServer struct {
	logger   slog.Logger
	fake     *fakeIPPool
	exempt   func(name string) bool
	upstream []netip.AddrPort
	relay    *dnsRelay

	udp  *net.UDPConn
	tcp  net.Listener
	addr netip.AddrPort
	wg   sync.WaitGroup
}

// listen binds UDP and TCP on the same loopback port.
func (s *dnsServer) listen(ctx context.Context, host string) error {
	var lc net.ListenConfig
	var lastErr error
	for range dnsListenRetries {
		pc, err := lc.ListenPacket(ctx, "udp4", net.JoinHostPort(host, "0"))
		if err != nil {
			return xerrors.Errorf("listen dns udp: %w", err)
		}
		udp, ok := pc.(*net.UDPConn)
		if !ok {
			_ = pc.Close()
			return xerrors.Errorf("unexpected packet conn type %T", pc)
		}
		addr, err := udpAddrPort(udp)
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
		s.udp, s.tcp, s.addr = udp, tcp, addr
		return nil
	}
	return xerrors.Errorf("listen dns tcp on udp port: %w", lastErr)
}

func (s *dnsServer) serve(ctx context.Context) {
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		s.serveUDP(ctx)
	}()
	go func() {
		defer s.wg.Done()
		s.serveTCP(ctx)
	}()
}

func (s *dnsServer) close() {
	if s.udp != nil {
		_ = s.udp.Close()
	}
	if s.tcp != nil {
		_ = s.tcp.Close()
	}
	if s.relay != nil {
		s.relay.close()
	}
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
		msg := make([]byte, n)
		copy(msg, buf[:n])
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			resp := s.handle(ctx, msg, dnsMaxUDPPayload)
			if resp == nil {
				return
			}
			if _, err := s.udp.WriteToUDPAddrPort(resp, src); err != nil && ctx.Err() == nil {
				s.logger.Debug(ctx, "write dns response", slog.Error(err))
			}
		}()
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
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serveTCPConn(ctx, conn)
		}()
	}
}

// serveTCPConn answers queries on one DNS-over-TCP connection in order.
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
		if resp == nil {
			return
		}
		if err := writeFrame(conn, resp); err != nil {
			return
		}
	}
}

// handle produces the response for one query, or nil when the query is
// too malformed to answer. Responses longer than maxSize are truncated.
func (s *dnsServer) handle(ctx context.Context, msg []byte, maxSize int) []byte {
	var parser dnsmessage.Parser
	hdr, err := parser.Start(msg)
	if err != nil {
		return nil
	}
	q, err := parser.Question()
	if err != nil {
		return buildResponse(hdr, nil, dnsmessage.RCodeFormatError, false, nil)
	}
	if hdr.Response || hdr.OpCode != 0 {
		return buildResponse(hdr, &q, dnsmessage.RCodeNotImplemented, false, nil)
	}
	resp := s.answer(ctx, hdr, q, msg)
	if len(resp) > maxSize {
		return buildResponse(hdr, &q, dnsmessage.RCodeSuccess, true, nil)
	}
	return resp
}

func (s *dnsServer) answer(ctx context.Context, hdr dnsmessage.Header, q dnsmessage.Question, msg []byte) []byte {
	name := normalizeName(q.Name.String())
	if q.Class != dnsmessage.ClassINET {
		return buildResponse(hdr, &q, dnsmessage.RCodeNotImplemented, false, nil)
	}
	if s.exempt(name) {
		return s.forward(ctx, hdr, q, msg, s.exchangeUpstream, "system resolvers")
	}
	switch q.Type {
	case dnsmessage.TypeA:
		addr := s.fake.Lookup(name)
		return buildResponse(hdr, &q, dnsmessage.RCodeSuccess, false, func(b *dnsmessage.Builder) error {
			return b.AResource(dnsmessage.ResourceHeader{
				Name:  q.Name,
				Type:  dnsmessage.TypeA,
				Class: dnsmessage.ClassINET,
				TTL:   fakeIPTTL,
			}, dnsmessage.AResource{A: addr.As4()})
		})
	case dnsmessage.TypeAAAA:
		// No AAAA answer steers dual-stack clients to the fake IPv4.
		return buildResponse(hdr, &q, dnsmessage.RCodeSuccess, false, nil)
	case dnsmessage.TypePTR:
		addr, ok := parseReverseName(name)
		if ok && s.fake.Contains(addr) {
			target, found := s.fake.Reverse(addr)
			if !found {
				return buildResponse(hdr, &q, dnsmessage.RCodeNameError, false, nil)
			}
			ptr, err := dnsmessage.NewName(target + ".")
			if err != nil {
				return buildResponse(hdr, &q, dnsmessage.RCodeServerFailure, false, nil)
			}
			return buildResponse(hdr, &q, dnsmessage.RCodeSuccess, false, func(b *dnsmessage.Builder) error {
				return b.PTRResource(dnsmessage.ResourceHeader{
					Name:  q.Name,
					Type:  dnsmessage.TypePTR,
					Class: dnsmessage.ClassINET,
					TTL:   fakeIPTTL,
				}, dnsmessage.PTRResource{PTR: ptr})
			})
		}
	}
	if s.relay == nil {
		return buildResponse(hdr, &q, dnsmessage.RCodeServerFailure, false, nil)
	}
	return s.forward(ctx, hdr, q, msg, s.relay.exchange, "exit node")
}

type dnsExchanger func(ctx context.Context, msg []byte) ([]byte, error)

// forward sends the query verbatim through exchange and returns its answer,
// or SERVFAIL when the exchange fails.
func (s *dnsServer) forward(ctx context.Context, hdr dnsmessage.Header, q dnsmessage.Question, msg []byte, exchange dnsExchanger, via string) []byte {
	ctx, cancel := context.WithTimeout(ctx, dnsExchangeTimeout)
	defer cancel()
	resp, err := exchange(ctx, msg)
	if err != nil {
		s.logger.Debug(ctx, "dns forward failed",
			slog.F("name", q.Name.String()),
			slog.F("type", q.Type.String()),
			slog.F("via", via),
			slog.Error(err),
		)
		return buildResponse(hdr, &q, dnsmessage.RCodeServerFailure, false, nil)
	}
	return resp
}

// exchangeUpstream tries each system resolver over TCP. TCP is used because
// enforcement exempts only TCP/53 to the resolvers; UDP/53 is redirected
// back into this server.
func (s *dnsServer) exchangeUpstream(ctx context.Context, msg []byte) ([]byte, error) {
	if len(s.upstream) == 0 {
		return nil, xerrors.New("no upstream resolvers configured")
	}
	var errs []error
	for _, rs := range s.upstream {
		resp, err := exchangeTCP(ctx, rs, msg)
		if err == nil {
			return resp, nil
		}
		errs = append(errs, xerrors.Errorf("%s: %w", rs, err))
	}
	return nil, errors.Join(errs...)
}

// exchangeTCP performs one DNS-over-TCP round trip.
func exchangeTCP(ctx context.Context, addr netip.AddrPort, msg []byte) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr.String())
	if err != nil {
		return nil, xerrors.Errorf("dial: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := writeFrame(conn, msg); err != nil {
		return nil, xerrors.Errorf("write query: %w", err)
	}
	resp, err := readFrame(conn)
	if err != nil {
		return nil, xerrors.Errorf("read response: %w", err)
	}
	return resp, nil
}

// buildResponse assembles a reply for q. answers, when non-nil, appends
// answer records to the builder. A nil q omits the question section, which
// is only appropriate for FORMERR.
func buildResponse(req dnsmessage.Header, q *dnsmessage.Question, rcode dnsmessage.RCode, truncated bool, answers func(*dnsmessage.Builder) error) []byte {
	hdr := dnsmessage.Header{
		ID:                 req.ID,
		Response:           true,
		OpCode:             req.OpCode,
		RecursionDesired:   req.RecursionDesired,
		RecursionAvailable: true,
		Truncated:          truncated,
		RCode:              rcode,
	}
	b := dnsmessage.NewBuilder(make([]byte, 0, dnsMaxUDPPayload), hdr)
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil
	}
	if q != nil {
		if err := b.Question(*q); err != nil {
			return nil
		}
	}
	if answers != nil {
		if err := b.StartAnswers(); err != nil {
			return nil
		}
		if err := answers(&b); err != nil {
			return nil
		}
	}
	out, err := b.Finish()
	if err != nil {
		return nil
	}
	return out
}

// parseReverseName turns d.c.b.a.in-addr.arpa into a.b.c.d.
func parseReverseName(name string) (netip.Addr, bool) {
	rest, ok := strings.CutSuffix(name, ".in-addr.arpa")
	if !ok {
		return netip.Addr{}, false
	}
	parts := strings.Split(rest, ".")
	if len(parts) != 4 {
		return netip.Addr{}, false
	}
	var raw [4]byte
	for i, p := range parts {
		v, err := strconv.ParseUint(p, 10, 8)
		if err != nil {
			return netip.Addr{}, false
		}
		raw[3-i] = byte(v)
	}
	return netip.AddrFrom4(raw), true
}

// dnsRelay multiplexes DNS queries over one long-lived CONNECT stream to
// the exit node. Responses may arrive out of order, so each in-flight
// query is given a private message ID and matched on the way back.
type dnsRelay struct {
	logger  slog.Logger
	connect func(ctx context.Context) (net.Conn, error)

	mu   sync.Mutex
	conn net.Conn
	// connecting is non-nil while a stream is being opened; waiters block
	// on it so only one CONNECT is in flight at a time.
	connecting chan struct{}
	writeMu    sync.Mutex
	pending    map[uint16]chan []byte
	nextID     uint16
	closed     bool
	wg         sync.WaitGroup
}

func newDNSRelay(logger slog.Logger, connect func(ctx context.Context) (net.Conn, error)) *dnsRelay {
	return &dnsRelay{
		logger:  logger,
		connect: connect,
		pending: make(map[uint16]chan []byte),
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

// exchange sends msg to the exit node and waits for the matching response.
// The response carries the caller's original message ID.
func (r *dnsRelay) exchange(ctx context.Context, msg []byte) ([]byte, error) {
	if len(msg) < 12 {
		return nil, xerrors.New("dns message too short")
	}
	conn, err := r.getConn(ctx)
	if err != nil {
		return nil, err
	}
	origID := uint16(msg[0])<<8 | uint16(msg[1])

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

	out := make([]byte, len(msg))
	copy(out, msg)
	out[0], out[1] = byte(id>>8), byte(id)
	r.writeMu.Lock()
	err = writeFrame(conn, out)
	r.writeMu.Unlock()
	if err != nil {
		r.fail(conn, err)
		return nil, xerrors.Errorf("write dns query to exit node: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, xerrors.New("exit node dns stream closed")
		}
		resp[0], resp[1] = byte(origID>>8), byte(origID)
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// getConn returns the shared stream, opening it if needed. Only one
// connect is in flight at a time; other callers wait for its outcome so
// the exit node sees a single long-lived stream.
func (r *dnsRelay) getConn(ctx context.Context) (net.Conn, error) {
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, xerrors.New("dns relay closed")
		}
		if r.conn != nil {
			conn := r.conn
			r.mu.Unlock()
			return conn, nil
		}
		if r.connecting != nil {
			wait := r.connecting
			r.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		done := make(chan struct{})
		r.connecting = done
		r.mu.Unlock()

		conn, err := r.connect(ctx)

		r.mu.Lock()
		r.connecting = nil
		close(done)
		if err != nil {
			r.mu.Unlock()
			return nil, xerrors.Errorf("open dns stream to exit node: %w", err)
		}
		if r.closed {
			r.mu.Unlock()
			_ = conn.Close()
			return nil, xerrors.New("dns relay closed")
		}
		r.conn = conn
		r.wg.Add(1)
		go r.readLoop(conn)
		r.mu.Unlock()
		return conn, nil
	}
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

// readLoop dispatches responses to waiting exchanges until the stream
// breaks, at which point every in-flight query fails and the next exchange
// reconnects.
func (r *dnsRelay) readLoop(conn net.Conn) {
	defer r.wg.Done()
	for {
		resp, err := readFrame(conn)
		if err != nil {
			r.fail(conn, err)
			return
		}
		if len(resp) < 12 {
			continue
		}
		id := uint16(resp[0])<<8 | uint16(resp[1])
		r.mu.Lock()
		ch, ok := r.pending[id]
		if ok {
			delete(r.pending, id)
		}
		r.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

// fail tears down conn if it is still the active stream and wakes every
// waiter with a closed channel.
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
