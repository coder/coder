package exitnode_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/exitnode"
	"github.com/coder/coder/v2/testutil"
)

type fakeDNSServer struct {
	addr        netip.AddrPort
	truncateUDP bool
	udp, tcp    atomic.Int64
}

func (s *fakeDNSServer) start(t *testing.T) *fakeDNSServer {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })
	s.addr = pc.LocalAddr().(*net.UDPAddr).AddrPort()
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			s.udp.Add(1)
			if resp, err := answerDNSQuery(buf[:n], s); err == nil {
				_, _ = pc.WriteTo(resp, addr)
			}
		}
	}()
	if !s.truncateUDP {
		return s
	}
	ln, err := net.Listen("tcp", s.addr.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var hdr [2]byte
				if _, err := io.ReadFull(conn, hdr[:]); err != nil {
					return
				}
				msg := make([]byte, int(hdr[0])<<8|int(hdr[1]))
				if _, err := io.ReadFull(conn, msg); err != nil {
					return
				}
				s.tcp.Add(1)
				if resp, err := answerDNSQuery(msg, nil); err == nil {
					_, _ = conn.Write(frame(resp))
				}
			}()
		}
	}()
	return s
}
func answerDNSQuery(query []byte, server *fakeDNSServer) ([]byte, error) {
	truncated := server != nil && server.truncateUDP
	var p dnsmessage.Parser
	h, err := p.Start(query)
	if err != nil {
		return nil, err
	}
	q, err := p.Question()
	if err != nil {
		return nil, err
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: h.ID, Response: true,
		RecursionDesired: h.RecursionDesired, RecursionAvailable: true, Truncated: truncated})
	if err := errors.Join(b.StartQuestions(), b.Question(q)); err != nil {
		return nil, err
	}
	if !truncated {
		if err := b.StartAnswers(); err != nil {
			return nil, err
		}
		if err := b.TXTResource(dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeTXT,
			Class: dnsmessage.ClassINET, TTL: 60}, dnsmessage.TXTResource{TXT: []string{"hello"}}); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}
func buildQuery(t *testing.T, id uint16, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, RecursionDesired: true})
	require.NoError(t, errors.Join(b.StartQuestions(), b.Question(dnsmessage.Question{
		Name: dnsmessage.MustNewName(name + "."), Type: typ, Class: dnsmessage.ClassINET})))
	msg, err := b.Finish()
	require.NoError(t, err)
	return msg
}
func parseResponse(t *testing.T, msg []byte) (dnsmessage.Header, []string) {
	t.Helper()
	var p dnsmessage.Parser
	h, err := p.Start(msg)
	require.NoError(t, err)
	require.NoError(t, p.SkipAllQuestions())
	answers, err := p.AllAnswers()
	require.NoError(t, err)
	var txt []string
	for _, a := range answers {
		if r, ok := a.Body.(*dnsmessage.TXTResource); ok {
			txt = append(txt, r.TXT...)
		}
	}
	return h, txt
}

var dnsHeader = http.Header{codersdk.ExitNodeProtocolHeader: []string{"dns"}}

const dnsPolicy = "default: deny\nrules:\n  - id: no-evil\n    deny: {hosts: [\"evil.example\"]}\n"

type failingExchanger struct{}

func (failingExchanger) Exchange(context.Context, []byte) ([]byte, netip.AddrPort, error) {
	return nil, netip.AddrPort{}, xerrors.New("upstream is down")
}
func dnsStream(ctx context.Context, t *testing.T, policy string, exchanger exitnode.DNSExchanger, configure ...func(*exitnode.ConnectProxyOptions)) (*proxyHarness, net.Conn, <-chan struct{}, func(uint16, string, dnsmessage.Type) []byte) {
	t.Helper()
	configure = append([]func(*exitnode.ConnectProxyOptions){func(o *exitnode.ConnectProxyOptions) { o.DNSExchanger = exchanger }}, configure...)
	h := newProxyHarness(t, policy, configure...)
	client, br, _, done := h.connectStatus(ctx, t, "dns:53", http.StatusOK, dnsHeader)
	return h, client, done, func(id uint16, name string, typ dnsmessage.Type) []byte {
		_, err := client.Write(frame(buildQuery(t, id, name, typ)))
		require.NoError(t, err)
		return readTestFrame(t, br)
	}
}
func TestConnectProxy_DNS(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, policy, host string
		exchanger          exitnode.DNSExchanger
		rcode              dnsmessage.RCode
		decision           codersdk.ExitNodeFlowDecision
		rule               string
	}{
		{"passthrough", dnsPolicy, "allowed.example", nil, dnsmessage.RCodeSuccess, codersdk.ExitNodeFlowAllow, ""},
		{"refused", dnsPolicy, "EVIL.example", nil, dnsmessage.RCodeRefused, codersdk.ExitNodeFlowDeny, "no-evil"},
		{"upstream failure", "default: allow\n", "a.example", failingExchanger{}, dnsmessage.RCodeServerFailure, codersdk.ExitNodeFlowAllow, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			upstream := (&fakeDNSServer{}).start(t)
			ex := tt.exchanger
			if ex == nil {
				ex = &exitnode.StaticDNSExchanger{Servers: []netip.AddrPort{upstream.addr}}
			}
			h, client, done, query := dnsStream(ctx, t, tt.policy, ex)
			answer := query(42, tt.host, dnsmessage.TypeTXT)
			hdr, txt := parseResponse(t, answer)
			require.Equal(t, uint16(42), hdr.ID)
			require.Equal(t, tt.rcode, hdr.RCode)
			if tt.rcode == dnsmessage.RCodeSuccess {
				require.Equal(t, []string{"hello"}, txt)
			} else {
				require.Empty(t, txt)
			}
			report := testutil.RequireReceive(ctx, t, h.flows.ch)
			require.Equal(t, tt.decision, report.Decision)
			require.Equal(t, codersdk.ExitNodeProtocolDNS, report.Protocol)
			require.Equal(t, tt.rule, report.RuleID)
			require.Equal(t, exitnode.NormalizeHost(tt.host), report.Host)
			require.Contains(t, report.Reason, "TXT")
			if tt.rcode == dnsmessage.RCodeSuccess {
				require.Equal(t, upstream.addr.Addr().String(), report.DestinationIP)
				require.Equal(t, int64(len(answer)), report.BytesIn)
			} else {
				require.Empty(t, report.DestinationIP)
			}
			_ = client.Close()
			testutil.RequireReceive(ctx, t, done)
		})
	}
}
func TestConnectProxy_DNSSampling(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	upstream := (&fakeDNSServer{}).start(t)
	h, client, done, query := dnsStream(ctx, t, dnsPolicy, &exitnode.StaticDNSExchanger{Servers: []netip.AddrPort{upstream.addr}},
		func(o *exitnode.ConnectProxyOptions) { o.DNSReportSampleRate = 3 })
	for id := uint16(1); id <= 6; id++ {
		require.Equal(t, id, mustDNSHeader(t, query(id, "ok.example", dnsmessage.TypeA)).ID)
	}
	require.Equal(t, dnsmessage.RCodeRefused, mustDNSHeader(t, query(99, "evil.example", dnsmessage.TypeA)).RCode)
	_ = client.Close()
	testutil.RequireReceive(ctx, t, done)
	var allows, denies int
	for _, report := range h.flows.reports {
		if report.Decision == codersdk.ExitNodeFlowAllow {
			allows++
		} else {
			denies++
		}
	}
	require.Equal(t, 2, allows)
	require.Equal(t, 1, denies)
}
func mustDNSHeader(t *testing.T, msg []byte) dnsmessage.Header {
	h, _ := parseResponse(t, msg)
	return h
}
func TestStaticDNSExchanger_TruncatedFallsBackToTCP(t *testing.T) {
	t.Parallel()
	upstream := (&fakeDNSServer{truncateUDP: true}).start(t)
	resp, server, err := (&exitnode.StaticDNSExchanger{Servers: []netip.AddrPort{upstream.addr}}).
		Exchange(testutil.Context(t, testutil.WaitLong), buildQuery(t, 42, "big.example", dnsmessage.TypeTXT))
	require.NoError(t, err)
	require.Equal(t, upstream.addr, server)
	hdr, txt := parseResponse(t, resp)
	require.False(t, hdr.Truncated)
	require.Equal(t, []string{"hello"}, txt)
	require.Equal(t, int64(1), upstream.udp.Load())
	require.Equal(t, int64(1), upstream.tcp.Load())
}
func TestResolvConfServers(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "resolv.conf")
	require.NoError(t, os.WriteFile(path, []byte("search example.com\nnameserver 10.0.0.53\nnameserver [fd00::53]\nnameserver invalid\n"), 0o600))
	servers, err := exitnode.ResolvConfServers(path)
	require.NoError(t, err)
	require.Equal(t, []netip.AddrPort{netip.MustParseAddrPort("10.0.0.53:53"), netip.MustParseAddrPort("[fd00::53]:53")}, servers)
	require.NoError(t, os.WriteFile(path, []byte("search example.com\n"), 0o600))
	_, err = exitnode.ResolvConfServers(path)
	require.ErrorContains(t, err, "no nameservers")
	_, err = exitnode.ResolvConfServers(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}
