package agentegress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

const (
	// udpMaxDatagram is the largest datagram relayed to the exit node. The
	// tailnet path is WireGuard with a 1280 byte MTU, so larger datagrams
	// would fragment across the CONNECT stream; QUIC and DNS stay under it.
	udpMaxDatagram = 1400
	// udpIdleTimeout ends a session with no traffic in either direction.
	udpIdleTimeout = 60 * time.Second
	// udpDNSIdleTimeout is shorter because DNS exchanges are one shot.
	udpDNSIdleTimeout = 30 * time.Second
	// udpDenyTTL is how long datagrams for a denied session are dropped
	// before the exit node is asked again.
	udpDenyTTL = 30 * time.Second
	// udpQueueDepth bounds datagrams buffered while the CONNECT is in
	// flight.
	udpQueueDepth = 32
	// udpOOBSize fits the IP_RECVORIGDSTADDR control message.
	udpOOBSize = 128
)

// udpProxy relays redirected UDP datagrams to the exit node. Each
// (client, destination) pair gets its own CONNECT stream carrying
// length-prefixed datagrams; replies are written back through the listener
// so conntrack rewrites them to look like they came from the destination.
type udpProxy struct {
	logger  slog.Logger
	clock   quartz.Clock
	fake    *fakeIPPool
	connect func(ctx context.Context, target string) (net.Conn, error)
	// origDst recovers the pre-redirect destination from control messages.
	// It is a field so tests can supply destinations without netfilter.
	origDst func(oob []byte) netip.AddrPort

	conn *net.UDPConn
	addr netip.AddrPort

	mu       sync.Mutex
	sessions map[udpSessionKey]*udpSession
	denied   map[udpSessionKey]time.Time
	wg       sync.WaitGroup

	// oversized counts datagrams dropped for exceeding udpMaxDatagram.
	oversized atomic.Int64
	// unknownDst counts datagrams dropped because the original destination
	// could not be recovered.
	unknownDst atomic.Int64
}

type udpSessionKey struct {
	src netip.AddrPort
	dst netip.AddrPort
}

func (p *udpProxy) listen(ctx context.Context, host string) error {
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp4", net.JoinHostPort(host, "0"))
	if err != nil {
		return xerrors.Errorf("listen udp: %w", err)
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return xerrors.Errorf("unexpected packet conn type %T", pc)
	}
	if err := enableUDPOriginalDst(conn); err != nil {
		_ = conn.Close()
		return xerrors.Errorf("enable original destination reporting: %w", err)
	}
	addr, err := udpAddrPort(conn)
	if err != nil {
		_ = conn.Close()
		return err
	}
	p.conn = conn
	p.addr = addr
	p.sessions = make(map[udpSessionKey]*udpSession)
	p.denied = make(map[udpSessionKey]time.Time)
	return nil
}

func (p *udpProxy) serve(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.readLoop(ctx)
	}()
}

func (p *udpProxy) close() {
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.mu.Lock()
	sessions := make([]*udpSession, 0, len(p.sessions))
	for _, s := range p.sessions {
		sessions = append(sessions, s)
	}
	p.mu.Unlock()
	for _, s := range sessions {
		s.close()
	}
	p.wg.Wait()
}

func (p *udpProxy) readLoop(ctx context.Context) {
	buf := make([]byte, maxFrameSize)
	oob := make([]byte, udpOOBSize)
	for {
		n, oobn, _, src, err := p.conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			p.logger.Debug(ctx, "read redirected udp datagram", slog.Error(err))
			continue
		}
		dst := p.origDst(oob[:oobn])
		if !dst.IsValid() || dst == p.addr {
			// REDIRECT rewrote the IP header before delivery, so the
			// kernel can only report our own address. Recovering the
			// real destination needs TPROXY or conntrack access.
			p.unknownDst.Add(1)
			key := udpSessionKey{src: src, dst: dst}
			if p.markDenied(key) {
				p.logger.Warn(ctx, "dropping redirected udp datagrams, original destination unavailable",
					slog.F("client", src.String()), slog.F("bytes", n))
			}
			continue
		}
		if n > udpMaxDatagram {
			p.oversized.Add(1)
			p.logger.Debug(ctx, "dropping oversized udp datagram",
				slog.F("client", src.String()), slog.F("destination", dst.String()), slog.F("bytes", n))
			continue
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		key := udpSessionKey{src: src, dst: dst}
		s := p.session(ctx, key)
		if s == nil {
			continue
		}
		s.send(payload)
	}
}

// markDenied records key in the negative cache and reports whether it was
// newly added, so callers log once per key per udpDenyTTL.
func (p *udpProxy) markDenied(key udpSessionKey) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clock.Now()
	if until, ok := p.denied[key]; ok && now.Before(until) {
		return false
	}
	p.denied[key] = now.Add(udpDenyTTL)
	return true
}

// session returns the session for key, creating and connecting it if
// needed. It returns nil when key is in the negative cache.
func (p *udpProxy) session(ctx context.Context, key udpSessionKey) *udpSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	if until, ok := p.denied[key]; ok {
		if p.clock.Now().Before(until) {
			return nil
		}
		delete(p.denied, key)
	}
	if s, ok := p.sessions[key]; ok {
		return s
	}
	timeout := udpIdleTimeout
	if key.dst.Port() == 53 {
		timeout = udpDNSIdleTimeout
	}
	s := &udpSession{
		proxy:   p,
		key:     key,
		target:  p.target(key.dst),
		timeout: timeout,
	}
	s.timer = p.clock.AfterFunc(timeout, s.expire, "udp_idle")
	p.sessions[key] = s
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		s.run(ctx)
	}()
	return s
}

// target names the CONNECT destination for dst, translating fake IPs back
// to the hostname the client resolved.
func (p *udpProxy) target(dst netip.AddrPort) string {
	if name, ok := p.fake.Reverse(dst.Addr()); ok {
		return net.JoinHostPort(name, strconv.Itoa(int(dst.Port())))
	}
	return dst.String()
}

func (p *udpProxy) remove(s *udpSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions[s.key] == s {
		delete(p.sessions, s.key)
	}
}

// udpAddrPort returns the bound address of a UDP socket.
func udpAddrPort(conn *net.UDPConn) (netip.AddrPort, error) {
	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return netip.AddrPort{}, xerrors.Errorf("unexpected udp local address type %T", conn.LocalAddr())
	}
	return udpAddr.AddrPort(), nil
}

// udpSession is one client-to-destination flow and its exit node stream.
type udpSession struct {
	proxy   *udpProxy
	key     udpSessionKey
	target  string
	timeout time.Duration
	timer   *quartz.Timer

	mu       sync.Mutex
	upstream net.Conn
	// queue holds datagrams that arrived before the CONNECT completed.
	queue  [][]byte
	closed bool
}

// send relays one datagram, queuing it while the stream is still being
// established. Datagrams beyond the queue depth are dropped, as they would
// be on a congested network.
func (s *udpSession) send(payload []byte) {
	s.timer.Reset(s.timeout)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.upstream == nil {
		if len(s.queue) < udpQueueDepth {
			s.queue = append(s.queue, payload)
		}
		return
	}
	if err := writeFrame(s.upstream, payload); err != nil {
		s.closeLocked()
	}
}

// run opens the stream, flushes queued datagrams and relays replies until
// the stream ends or the session is closed.
func (s *udpSession) run(ctx context.Context) {
	defer s.proxy.remove(s)
	defer s.close()
	upstream, err := s.proxy.connect(ctx, s.target)
	if err != nil {
		var denied *DeniedError
		if errors.As(err, &denied) {
			if s.proxy.markDenied(s.key) {
				s.proxy.logger.Info(ctx, "udp egress denied by exit node",
					slog.F("client", s.key.src.String()),
					slog.F("target", s.target),
					slog.F("reason", denied.Reason))
			}
			return
		}
		if ctx.Err() == nil {
			s.proxy.logger.Warn(ctx, "udp egress through exit node failed",
				slog.F("target", s.target), slog.Error(err))
		}
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = upstream.Close()
		return
	}
	s.upstream = upstream
	queue := s.queue
	s.queue = nil
	for _, payload := range queue {
		if err := writeFrame(upstream, payload); err != nil {
			s.closeLocked()
			s.mu.Unlock()
			return
		}
	}
	s.mu.Unlock()

	for {
		payload, err := readFrame(upstream)
		if err != nil {
			return
		}
		s.timer.Reset(s.timeout)
		if _, err := s.proxy.conn.WriteToUDPAddrPort(payload, s.key.src); err != nil {
			return
		}
	}
}

func (s *udpSession) expire() {
	s.proxy.logger.Debug(context.Background(), "udp session idle, closing",
		slog.F("client", s.key.src.String()), slog.F("target", s.target))
	s.close()
}

func (s *udpSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
}

func (s *udpSession) closeLocked() {
	if s.closed {
		return
	}
	s.closed = true
	s.queue = nil
	s.timer.Stop()
	if s.upstream != nil {
		_ = s.upstream.Close()
	}
}
