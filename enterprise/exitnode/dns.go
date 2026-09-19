package exitnode

import (
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
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
	dnsTargetHost              = "dns"
	dnsPort                    = 53
	maxInflightDNSQueries      = 64
	resolvConfPath             = "/etc/resolv.conf"
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

// ResolvConfServers returns the nameserver entries of a resolv.conf file as
// port 53 endpoints.
func ResolvConfServers(path string) ([]netip.AddrPort, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, xerrors.Errorf("read %s: %w", path, err)
	}
	var servers []netip.AddrPort
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		if addr, err := netip.ParseAddr(strings.Trim(fields[1], "[]")); err == nil {
			servers = append(servers, netip.AddrPortFrom(addr.Unmap(), dnsPort))
		}
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
	var lastErr error
	for _, server := range e.Servers {
		resp, err := e.exchangeWith(ctx, server, msg)
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

func (e *StaticDNSExchanger) exchangeWith(ctx context.Context, server netip.AddrPort, msg []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(max(e.Timeout, 0), DefaultDNSExchangeTimeout))
	defer cancel()
	resp, err := exchangeOn(ctx, e.Dial, "udp", server, msg)
	if err != nil {
		return nil, err
	}
	var p dnsmessage.Parser
	h, err := p.Start(resp)
	if err != nil {
		return nil, xerrors.Errorf("parse udp response from %s: %w", server, err)
	}
	if !h.Truncated {
		return resp, nil
	}
	if resp, err = exchangeOn(ctx, e.Dial, "tcp", server, msg); err != nil {
		return nil, xerrors.Errorf("retry truncated response over tcp: %w", err)
	}
	return resp, nil
}

// exchangeOn sends msg to server over network and returns the reply. Over
// udp, replies whose ID does not match are skipped so a stale answer to an
// earlier query on a reused port is not mistaken for this one.
func exchangeOn(ctx context.Context, dial DialFunc, network string, server netip.AddrPort, msg []byte) ([]byte, error) {
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	conn, err := dial(ctx, network, server.String())
	if err != nil {
		return nil, xerrors.Errorf("dial %s: %w", server, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	buf := make([]byte, maxFramePayload)
	if network == "tcp" {
		if err := (&frameWriter{w: conn}).write(msg); err != nil {
			return nil, xerrors.Errorf("send query to %s: %w", server, err)
		}
		if buf, err = readFrame(conn, buf); err != nil {
			return nil, xerrors.Errorf("read response from %s: %w", server, err)
		}
		return buf, nil
	}
	if _, err := conn.Write(msg); err != nil {
		return nil, xerrors.Errorf("send query to %s: %w", server, err)
	}
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, xerrors.Errorf("query to %s: %w", server, ctx.Err())
			}
			return nil, xerrors.Errorf("read response from %s: %w", server, err)
		}
		if n >= frameHeaderLen && binary.BigEndian.Uint16(buf[:n]) == binary.BigEndian.Uint16(msg) {
			return buf[:n], nil
		}
	}
}

type dnsQuery struct {
	header   dnsmessage.Header
	question dnsmessage.Question
	name     string
}

func (q dnsQuery) qtype() string {
	return strings.TrimPrefix(q.question.Type.String(), "Type")
}

// parseDNSQuery returns a parsed question and whether its header was valid.
func parseDNSQuery(msg []byte) (q dnsQuery, hdrOK bool, err error) {
	var p dnsmessage.Parser
	if q.header, err = p.Start(msg); err != nil {
		return q, false, xerrors.Errorf("parse header: %w", err)
	}
	if q.question, err = p.Question(); err != nil {
		return q, true, xerrors.Errorf("parse question: %w", err)
	}
	q.name = NormalizeHost(q.question.Name.String())
	return q, true, nil
}

// answerDNS writes an answerless response with the given RCODE.
func answerDNS(ctx context.Context, logger slog.Logger, fw *frameWriter, q dnsQuery, rcode dnsmessage.RCode) {
	b := dnsmessage.NewBuilder(make([]byte, frameHeaderLen, 512), dnsmessage.Header{
		ID:                 q.header.ID,
		Response:           true,
		OpCode:             q.header.OpCode,
		RecursionDesired:   q.header.RecursionDesired,
		RecursionAvailable: true,
		RCode:              rcode,
	})
	var err error
	if q.question.Name.Length > 0 {
		err = errors.Join(b.StartQuestions(), b.Question(q.question))
	}
	msg, finishErr := b.Finish()
	if err = errors.Join(err, finishErr); err != nil {
		logger.Debug(ctx, "build dns response", slog.Error(err))
		return
	}
	if err := fw.write(msg[frameHeaderLen:]); err != nil {
		logger.Debug(ctx, "write dns response", slog.Error(err))
	}
}

// handleDNS serves framed DNS queries, refusing denied names locally.
func (p *ConnectProxy) handleDNS(ctx context.Context, logger slog.Logger, client net.Conn, agentID uuid.UUID) {
	if !respondConnect(ctx, logger, client, http.StatusOK, Decision{}) {
		return
	}
	p.Metrics.ActiveFlows.Inc()
	defer p.Metrics.ActiveFlows.Dec()

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
			if hdrOK {
				answerDNS(ctx, logger, fw, q, dnsmessage.RCodeFormatError)
			}
			continue
		}

		info := FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: q.name, Port: dnsPort}
		flow := newFlow(p.Clock.Now(), agentID, info)
		flow.BytesOut = int64(len(msg))
		qlogger := logger.With(slog.F("qname", q.name), slog.F("qtype", q.qtype()))
		if !p.evaluateFlow(ctx, qlogger, &flow, info, " ("+q.qtype()+" query)") {
			answerDNS(ctx, qlogger, fw, q, dnsmessage.RCodeRefused)
			continue
		}

		select {
		case inflight <- struct{}{}:
		case <-ctx.Done():
			return
		}
		wg.Go(func() {
			defer func() { <-inflight }()
			p.forwardDNSQuery(ctx, qlogger, fw, q, msg, flow)
		})
	}
}

func (p *ConnectProxy) forwardDNSQuery(ctx context.Context, logger slog.Logger, fw *frameWriter, q dnsQuery, msg []byte, flow codersdk.ExitNodeFlowReport) {
	resp, server, err := p.DNSExchanger.Exchange(ctx, msg)
	if err != nil {
		logger.Debug(ctx, "upstream dns exchange failed", slog.Error(err))
		answerDNS(ctx, logger, fw, q, dnsmessage.RCodeServerFailure)
	} else {
		if err := fw.write(resp); err != nil {
			logger.Debug(ctx, "write dns response", slog.Error(err))
		}
		flow.DestinationIP = server.Addr().String()
		flow.BytesIn = int64(len(resp))
	}

	p.allowFlow(&flow)
	p.Metrics.BytesTotal.WithLabelValues("in").Add(float64(flow.BytesIn))
	p.Metrics.BytesTotal.WithLabelValues("out").Add(float64(flow.BytesOut))
	if p.DNSReportSampleRate <= 1 || p.dnsSampleCounter.Add(1)%int64(p.DNSReportSampleRate) == 0 {
		p.recordEnded(flow)
	}
}
