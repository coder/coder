package exitnode_test

import (
	"context"
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
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

// fakeDNSServer answers every query with a single TXT record "hello". When
// truncateUDP is set, UDP answers carry the TC bit and no records so the
// client must retry over TCP.
type fakeDNSServer struct {
	addr        netip.AddrPort
	truncateUDP bool
	udpQueries  atomic.Int64
	tcpQueries  atomic.Int64
}

// start listens on a random loopback UDP port and, when truncateUDP is set,
// the same TCP port.
func (s *fakeDNSServer) start(t *testing.T) *fakeDNSServer {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })
	udpAddr, ok := pc.LocalAddr().(*net.UDPAddr)
	require.True(t, ok)
	s.addr = udpAddr.AddrPort()
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			s.udpQueries.Add(1)
			resp, err := s.answer(buf[:n], "udp")
			if err != nil {
				continue
			}
			_, _ = pc.WriteTo(resp, addr)
		}
	}()
	if s.truncateUDP {
		ln, err := net.Listen("tcp", s.addr.String())
		require.NoError(t, err, "tcp port matching the udp port must be free")
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
					s.tcpQueries.Add(1)
					resp, err := s.answer(msg, "tcp")
					if err != nil {
						return
					}
					_, _ = conn.Write(frame(resp))
				}()
			}
		}()
	}
	return s
}

// answer builds the response for a query received over transport.
func (s *fakeDNSServer) answer(query []byte, transport string) ([]byte, error) {
	truncate := transport == "udp" && s.truncateUDP
	var p dnsmessage.Parser
	h, err := p.Start(query)
	if err != nil {
		return nil, err
	}
	q, err := p.Question()
	if err != nil {
		return nil, err
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID:                 h.ID,
		Response:           true,
		RecursionDesired:   h.RecursionDesired,
		RecursionAvailable: true,
		Truncated:          truncate,
	})
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(q); err != nil {
		return nil, err
	}
	if !truncate {
		if err := b.StartAnswers(); err != nil {
			return nil, err
		}
		err = b.TXTResource(dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET, TTL: 60},
			dnsmessage.TXTResource{TXT: []string{"hello"}})
		if err != nil {
			return nil, err
		}
	}
	return b.Finish()
}

func buildQuery(t *testing.T, id uint16, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, RecursionDesired: true})
	require.NoError(t, b.StartQuestions())
	require.NoError(t, b.Question(dnsmessage.Question{
		Name:  dnsmessage.MustNewName(name + "."),
		Type:  typ,
		Class: dnsmessage.ClassINET,
	}))
	msg, err := b.Finish()
	require.NoError(t, err)
	return msg
}

// parseResponse returns the header and the TXT strings in the answer section.
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

const dnsPolicy = `
default: deny
rules:
  - id: no-evil
    deny:
      hosts: ["evil.example"]
`

type failingExchanger struct{}

func (failingExchanger) Exchange(context.Context, []byte) ([]byte, netip.AddrPort, error) {
	return nil, netip.AddrPort{}, xerrors.New("upstream is down")
}

func TestConnectProxy_DNS(t *testing.T) {
	t.Parallel()

	t.Run("AllowForwardsAndReports", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		upstream := (&fakeDNSServer{}).start(t)

		// default: deny must not apply to dns queries.
		h := newProxyHarness(t, dnsPolicy, func(o *exitnode.ConnectProxyOptions) {
			o.DNSExchanger = &exitnode.StaticDNSExchanger{Servers: []netip.AddrPort{upstream.addr}}
		})
		src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
		client, br, resp, done := h.connectWithHeaders(ctx, t, src, "dns:53", dnsHeader)
		require.True(t, resp.ok)
		require.Equal(t, http.StatusOK, resp.status)

		query := buildQuery(t, 0x1234, "allowed.example", dnsmessage.TypeTXT)
		_, err := client.Write(frame(query))
		require.NoError(t, err)
		answer := readTestFrame(t, br)
		hdr, txt := parseResponse(t, answer)
		require.Equal(t, uint16(0x1234), hdr.ID)
		require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
		require.Equal(t, []string{"hello"}, txt)

		report := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, codersdk.ExitNodeFlowAllow, report.Decision)
		require.Equal(t, codersdk.ExitNodeProtocolDNS, report.Protocol)
		require.Equal(t, "allowed.example", report.Host)
		require.Equal(t, upstream.addr.Addr().String(), report.DestinationIP)
		require.Equal(t, 53, report.DestinationPort)
		require.Contains(t, report.Reason, "TXT")
		require.Empty(t, report.RuleID)
		require.Equal(t, int64(len(query)), report.BytesOut)
		require.Equal(t, int64(len(answer)), report.BytesIn)
		require.NotNil(t, report.DisconnectTime)

		_ = client.Close()
		testutil.RequireReceive(ctx, t, done)
	})

	t.Run("DenyReturnsRefused", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		upstream := (&fakeDNSServer{}).start(t)

		h := newProxyHarness(t, dnsPolicy, func(o *exitnode.ConnectProxyOptions) {
			o.DNSExchanger = &exitnode.StaticDNSExchanger{Servers: []netip.AddrPort{upstream.addr}}
		})
		src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
		client, br, resp, done := h.connectWithHeaders(ctx, t, src, "dns:53", dnsHeader)
		require.True(t, resp.ok)
		require.Equal(t, http.StatusOK, resp.status)

		_, err := client.Write(frame(buildQuery(t, 0xbeef, "EVIL.example", dnsmessage.TypeA)))
		require.NoError(t, err)
		hdr, txt := parseResponse(t, readTestFrame(t, br))
		require.Equal(t, uint16(0xbeef), hdr.ID)
		require.True(t, hdr.Response)
		require.Equal(t, dnsmessage.RCodeRefused, hdr.RCode)
		require.Empty(t, txt)

		report := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
		require.Equal(t, codersdk.ExitNodeProtocolDNS, report.Protocol)
		require.Equal(t, "no-evil", report.RuleID)
		require.Equal(t, "evil.example", report.Host)
		require.Contains(t, report.Reason, "A query")
		require.Empty(t, report.DestinationIP)

		// Allowed queries on the same stream still work afterwards, and the
		// denied one never reached the upstream.
		_, err = client.Write(frame(buildQuery(t, 1, "ok.example", dnsmessage.TypeTXT)))
		require.NoError(t, err)
		hdr, _ = parseResponse(t, readTestFrame(t, br))
		require.Equal(t, uint16(1), hdr.ID)
		require.Equal(t, dnsmessage.RCodeSuccess, hdr.RCode)
		require.Equal(t, int64(1), upstream.udpQueries.Load())

		_ = client.Close()
		testutil.RequireReceive(ctx, t, done)
	})

	t.Run("UpstreamFailureReturnsServfail", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		h := newProxyHarness(t, "default: allow\n", func(o *exitnode.ConnectProxyOptions) {
			o.DNSExchanger = failingExchanger{}
		})
		src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
		client, br, resp, done := h.connectWithHeaders(ctx, t, src, "dns:53", dnsHeader)
		require.True(t, resp.ok)
		require.Equal(t, http.StatusOK, resp.status)

		_, err := client.Write(frame(buildQuery(t, 7, "a.example", dnsmessage.TypeA)))
		require.NoError(t, err)
		hdr, _ := parseResponse(t, readTestFrame(t, br))
		require.Equal(t, uint16(7), hdr.ID)
		require.Equal(t, dnsmessage.RCodeServerFailure, hdr.RCode)

		report := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, codersdk.ExitNodeFlowAllow, report.Decision)
		require.Empty(t, report.DestinationIP)
		require.Zero(t, report.BytesIn)

		_ = client.Close()
		testutil.RequireReceive(ctx, t, done)
	})

	t.Run("Sampling", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		upstream := (&fakeDNSServer{}).start(t)

		h := newProxyHarness(t, dnsPolicy, func(o *exitnode.ConnectProxyOptions) {
			o.DNSExchanger = &exitnode.StaticDNSExchanger{Servers: []netip.AddrPort{upstream.addr}}
			o.DNSReportSampleRate = 3
		})
		src := tailnet.TailscaleServicePrefix.AddrFromUUID(h.agentID)
		client, br, resp, done := h.connectWithHeaders(ctx, t, src, "dns:53", dnsHeader)
		require.True(t, resp.ok)
		require.Equal(t, http.StatusOK, resp.status)

		for id := uint16(1); id <= 6; id++ {
			_, err := client.Write(frame(buildQuery(t, id, "ok.example", dnsmessage.TypeA)))
			require.NoError(t, err)
			hdr, _ := parseResponse(t, readTestFrame(t, br))
			require.Equal(t, id, hdr.ID)
		}
		// Denials are never sampled away.
		_, err := client.Write(frame(buildQuery(t, 99, "evil.example", dnsmessage.TypeA)))
		require.NoError(t, err)
		hdr, _ := parseResponse(t, readTestFrame(t, br))
		require.Equal(t, dnsmessage.RCodeRefused, hdr.RCode)

		// Reports trail responses, so wait for the handler to finish before
		// counting.
		_ = client.Close()
		testutil.RequireReceive(ctx, t, done)

		var allows, denies int
		for _, r := range h.flows.reports {
			switch r.Decision {
			case codersdk.ExitNodeFlowAllow:
				allows++
			case codersdk.ExitNodeFlowDeny:
				denies++
			}
		}
		require.Equal(t, 2, allows, "one in three allowed queries is reported")
		require.Equal(t, 1, denies)
	})
}

func TestStaticDNSExchanger_TruncatedFallsBackToTCP(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	upstream := (&fakeDNSServer{truncateUDP: true}).start(t)

	ex := &exitnode.StaticDNSExchanger{Servers: []netip.AddrPort{upstream.addr}}
	resp, server, err := ex.Exchange(ctx, buildQuery(t, 42, "big.example", dnsmessage.TypeTXT))
	require.NoError(t, err)
	require.Equal(t, upstream.addr, server)
	hdr, txt := parseResponse(t, resp)
	require.Equal(t, uint16(42), hdr.ID)
	require.False(t, hdr.Truncated)
	require.Equal(t, []string{"hello"}, txt)
	require.Equal(t, int64(1), upstream.udpQueries.Load())
	require.Equal(t, int64(1), upstream.tcpQueries.Load())
}

func TestResolvConfServers(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "resolv.conf")
	require.NoError(t, os.WriteFile(path, []byte(`
# comment
search example.com
nameserver 10.0.0.53
nameserver [fd00::53]
nameserver not-an-ip
options edns0
`), 0o600))
	servers, err := exitnode.ResolvConfServers(path)
	require.NoError(t, err)
	require.Equal(t, []netip.AddrPort{
		netip.MustParseAddrPort("10.0.0.53:53"),
		netip.MustParseAddrPort("[fd00::53]:53"),
	}, servers)

	require.NoError(t, os.WriteFile(path, []byte("search example.com\n"), 0o600))
	_, err = exitnode.ResolvConfServers(path)
	require.ErrorContains(t, err, "no nameservers")

	_, err = exitnode.ResolvConfServers(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}
