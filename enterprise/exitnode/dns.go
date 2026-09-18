package exitnode

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/codersdk"
)

const (
	// DefaultDNSExchangeTimeout bounds one query to one upstream server.
	DefaultDNSExchangeTimeout = 5 * time.Second
	// DefaultDNSReportSampleRate reports every allowed dns query. Raise it
	// to report only one in N; denials are always reported.
	DefaultDNSReportSampleRate = 1
	// dnsTargetHost is the CONNECT host a dns stream must name; there is no
	// real destination because the exit node picks the upstream.
	dnsTargetHost = "dns"
	dnsPort       = 53
	// maxInflightDNSQueries bounds concurrent upstream exchanges per stream
	// so a flood of queries cannot spawn unbounded goroutines.
	maxInflightDNSQueries = 64
	// resolvConfPath is where the default upstream servers come from.
	resolvConfPath = "/etc/resolv.conf"
)

// DNSExchanger sends one DNS message to an upstream server and returns the
// raw response along with the server that answered.
type DNSExchanger interface {
	Exchange(ctx context.Context, msg []byte) (resp []byte, server netip.AddrPort, err error)
}

// StaticDNSExchanger forwards queries to a fixed list of servers, trying each
// in order. Queries go over UDP first and are retried over TCP when the
// response has the TC bit set.
type StaticDNSExchanger struct {
	Servers []netip.AddrPort
	// Dial defaults to a net.Dialer. Tests inject one to reach a fake server.
	Dial DialFunc
	// Timeout bounds the exchange with a single server. Defaults to
	// DefaultDNSExchangeTimeout.
	Timeout time.Duration
}

var _ DNSExchanger = (*StaticDNSExchanger)(nil)

// ResolvConfServers returns the nameserver entries of a resolv.conf file as
// port 53 endpoints.
func ResolvConfServers(path string) ([]netip.AddrPort, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var servers []netip.AddrPort
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		addr, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(fields[1], "["), "]"))
		if err != nil {
			continue
		}
		servers = append(servers, netip.AddrPortFrom(addr.Unmap(), dnsPort))
	}
	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("read %s: %w", path, err)
	}
	if len(servers) == 0 {
		return nil, xerrors.Errorf("%s lists no nameservers", path)
	}
	return servers, nil
}

// Exchange implements DNSExchanger.
func (e *StaticDNSExchanger) Exchange(ctx context.Context, msg []byte) ([]byte, netip.AddrPort, error) {
	if len(e.Servers) == 0 {
		return nil, netip.AddrPort{}, xerrors.New("no upstream dns servers configured")
	}
	dial := e.Dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = DefaultDNSExchangeTimeout
	}

	var lastErr error
	for _, server := range e.Servers {
		resp, err := exchangeWith(ctx, dial, server, msg, timeout)
		if err == nil {
			return resp, server, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, netip.AddrPort{}, lastErr
}

func exchangeWith(ctx context.Context, dial DialFunc, server netip.AddrPort, msg []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := exchangeUDP(ctx, dial, server, msg)
	if err != nil {
		return nil, err
	}
	var hdr dnsmessage.Parser
	h, err := hdr.Start(resp)
	if err != nil {
		return nil, xerrors.Errorf("parse udp response from %s: %w", server, err)
	}
	if !h.Truncated {
		return resp, nil
	}
	resp, err = exchangeTCP(ctx, dial, server, msg)
	if err != nil {
		return nil, xerrors.Errorf("retry truncated response over tcp: %w", err)
	}
	return resp, nil
}

func exchangeUDP(ctx context.Context, dial DialFunc, server netip.AddrPort, msg []byte) ([]byte, error) {
	conn, err := dial(ctx, "udp", server.String())
	if err != nil {
		return nil, xerrors.Errorf("dial %s: %w", server, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if _, err := conn.Write(msg); err != nil {
		return nil, xerrors.Errorf("send query to %s: %w", server, err)
	}
	want := binary.BigEndian.Uint16(msg)
	buf := make([]byte, maxFramePayload)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, xerrors.Errorf("query to %s: %w", server, ctx.Err())
			}
			return nil, xerrors.Errorf("read response from %s: %w", server, err)
		}
		// A stale answer to an earlier query on a reused port is skipped
		// rather than mistaken for this one.
		if n >= frameHeaderLen && binary.BigEndian.Uint16(buf[:n]) == want {
			return append([]byte(nil), buf[:n]...), nil
		}
	}
}

func exchangeTCP(ctx context.Context, dial DialFunc, server netip.AddrPort, msg []byte) ([]byte, error) {
	conn, err := dial(ctx, "tcp", server.String())
	if err != nil {
		return nil, xerrors.Errorf("dial %s: %w", server, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	fw := &frameWriter{w: conn}
	if err := fw.write(msg); err != nil {
		return nil, xerrors.Errorf("send query to %s: %w", server, err)
	}
	resp, err := readFrame(conn, make([]byte, maxFramePayload))
	if err != nil {
		return nil, xerrors.Errorf("read response from %s: %w", server, err)
	}
	return resp, nil
}

// dnsQuery is the part of a query the policy and the flow report care about.
type dnsQuery struct {
	header   dnsmessage.Header
	question dnsmessage.Question
	// name is the normalized query name without the trailing dot.
	name string
}

// qtype is the record type without the "Type" prefix dnsmessage adds, for
// example "TXT".
func (q dnsQuery) qtype() string {
	return strings.TrimPrefix(q.question.Type.String(), "Type")
}

// parseDNSQuery extracts the header and first question. The returned header
// is valid whenever hdrOK is true, even if the question could not be parsed,
// so a FORMERR response can still carry the right ID.
func parseDNSQuery(msg []byte) (q dnsQuery, hdrOK bool, err error) {
	var p dnsmessage.Parser
	q.header, err = p.Start(msg)
	if err != nil {
		return q, false, xerrors.Errorf("parse header: %w", err)
	}
	q.question, err = p.Question()
	if err != nil {
		return q, true, xerrors.Errorf("parse question: %w", err)
	}
	q.name = NormalizeHost(q.question.Name.String())
	return q, true, nil
}

// buildDNSResponse synthesizes an answerless response to q with the given
// RCODE, echoing the ID, opcode, recursion flag, and question (when one was
// parsed) so the client resolver matches it to the query.
func buildDNSResponse(q dnsQuery, rcode dnsmessage.RCode) ([]byte, error) {
	b := dnsmessage.NewBuilder(make([]byte, frameHeaderLen, 512), dnsmessage.Header{
		ID:                 q.header.ID,
		Response:           true,
		OpCode:             q.header.OpCode,
		RecursionDesired:   q.header.RecursionDesired,
		RecursionAvailable: true,
		RCode:              rcode,
	})
	if q.question.Name.Length > 0 {
		if err := b.StartQuestions(); err != nil {
			return nil, xerrors.Errorf("start questions: %w", err)
		}
		if err := b.Question(q.question); err != nil {
			return nil, xerrors.Errorf("add question: %w", err)
		}
	}
	msg, err := b.Finish()
	if err != nil {
		return nil, xerrors.Errorf("finish message: %w", err)
	}
	// NewBuilder was given a 2-byte prefix so it could skip the length;
	// drop it again because the frame writer adds its own.
	return msg[frameHeaderLen:], nil
}

// handleDNS serves a dns stream. Each frame is one DNS message; the policy is
// applied to its query name and the message is either refused locally or
// forwarded to an upstream server. Responses are written as they arrive and
// may be out of order, so the client must correlate by message ID.
//
// Denied queries get a response with RCODE REFUSED. REFUSED, rather than
// NXDOMAIN, tells the workspace the name may exist but policy forbids
// resolving it, which is the truthful answer and avoids negative caching of a
// name that a policy reload could allow moments later. Upstream failures get
// SERVFAIL so the client resolver does not wait for a timeout.
func (p *ConnectProxy) handleDNS(ctx context.Context, logger slog.Logger, client net.Conn, agentID uuid.UUID) {
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		logger.Debug(ctx, "write connect response", slog.Error(err))
		return
	}
	p.metrics.ActiveFlows.Inc()
	defer p.metrics.ActiveFlows.Dec()

	fw := &frameWriter{w: client}
	inflight := make(chan struct{}, maxInflightDNSQueries)
	var wg sync.WaitGroup
	defer wg.Wait()

	buf := make([]byte, maxFramePayload)
	for {
		frame, err := readFrame(client, buf)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logger.Debug(ctx, "dns stream ended", slog.Error(err))
			}
			return
		}
		msg := append([]byte(nil), frame...)

		q, hdrOK, err := parseDNSQuery(msg)
		if err != nil {
			logger.Debug(ctx, "malformed dns query", slog.Error(err))
			if !hdrOK {
				// Without an ID there is nothing the client could match a
				// response to.
				continue
			}
			writeDNSResponse(ctx, logger, fw, q, dnsmessage.RCodeFormatError)
			continue
		}

		now := p.clock.Now()
		flow := codersdk.ExitNodeFlowReport{
			FlowID:          uuid.New(),
			AgentID:         agentID,
			Protocol:        codersdk.ExitNodeProtocolDNS,
			DestinationPort: dnsPort,
			Host:            q.name,
			BytesOut:        int64(len(msg)),
			ConnectTime:     now,
		}
		qlogger := logger.With(slog.F("qname", q.name), slog.F("qtype", q.qtype()))

		decision := p.policy.Evaluate(FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: q.name})
		decision.Reason = decision.Reason + " (" + q.qtype() + " query)"
		if !decision.Allow {
			writeDNSResponse(ctx, qlogger, fw, q, dnsmessage.RCodeRefused)
			p.recordDeny(ctx, qlogger, flow, decision)
			continue
		}
		flow.Decision = codersdk.ExitNodeFlowAllow
		flow.RuleID = decision.RuleID
		flow.Reason = decision.Reason

		select {
		case inflight <- struct{}{}:
		case <-ctx.Done():
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-inflight }()
			p.forwardDNSQuery(ctx, qlogger, fw, q, msg, flow)
		}()
	}
}

// forwardDNSQuery resolves one allowed query upstream, relays the response,
// and reports the flow subject to sampling.
func (p *ConnectProxy) forwardDNSQuery(ctx context.Context, logger slog.Logger, fw *frameWriter, q dnsQuery, msg []byte, flow codersdk.ExitNodeFlowReport) {
	resp, server, err := p.dnsExchanger.Exchange(ctx, msg)
	if err != nil {
		logger.Debug(ctx, "upstream dns exchange failed", slog.Error(err))
		writeDNSResponse(ctx, logger, fw, q, dnsmessage.RCodeServerFailure)
	} else {
		if err := fw.write(resp); err != nil {
			logger.Debug(ctx, "write dns response", slog.Error(err))
		}
		flow.DestinationIP = server.Addr().String()
		flow.BytesIn = int64(len(resp))
	}

	p.metrics.FlowsTotal.WithLabelValues(string(codersdk.ExitNodeFlowAllow)).Inc()
	p.metrics.BytesTotal.WithLabelValues("in").Add(float64(flow.BytesIn))
	p.metrics.BytesTotal.WithLabelValues("out").Add(float64(flow.BytesOut))
	if p.dnsSampleRate > 1 && p.dnsSampleCounter.Add(1)%uint64(p.dnsSampleRate) != 0 {
		return
	}
	now := p.clock.Now()
	flow.DisconnectTime = &now
	p.flows.Record(flow)
}

func writeDNSResponse(ctx context.Context, logger slog.Logger, fw *frameWriter, q dnsQuery, rcode dnsmessage.RCode) {
	resp, err := buildDNSResponse(q, rcode)
	if err != nil {
		logger.Debug(ctx, "build dns response", slog.Error(err))
		return
	}
	if err := fw.write(resp); err != nil {
		logger.Debug(ctx, "write dns response", slog.Error(err))
	}
}
