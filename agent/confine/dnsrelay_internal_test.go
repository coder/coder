package confine

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestDNSRelayAllowedName(t *testing.T) {
	t.Parallel()

	host := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, net.IPv4(192, 0, 2, 10)))
	engine := testDNSPolicy(10, codersdk.AIEgressRule{Host: host, Ports: []int{8443}})
	relay := startTestDNSRelay(t, engine, nil, upstream.address)

	response := exchangeTestDNS(t, "udp", relay.Addr().String(), host, dns.TypeA)
	require.Equal(t, dns.RcodeSuccess, response.Rcode)
	require.Len(t, response.Answer, 1)
	answer, ok := response.Answer[0].(*dns.A)
	require.True(t, ok)
	require.True(t, answer.A.Equal(net.IPv4(192, 0, 2, 10)))
	require.Equal(t, int64(1), upstreamQueries.Load())
}

func TestDNSRelayDeniedName(t *testing.T) {
	t.Parallel()

	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	relay := startTestDNSRelay(t, testDNSPolicy(11), nil, upstream.address)

	response := exchangeTestDNS(t, "udp", relay.Addr().String(), testDNSHost(t), dns.TypeA)
	require.Equal(t, dns.RcodeRefused, response.Rcode)
	require.Zero(t, upstreamQueries.Load())
}

func TestDNSRelayWildcardPolicy(t *testing.T) {
	t.Parallel()

	baseHost := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	engine := testDNSPolicy(12, codersdk.AIEgressRule{Host: "*." + baseHost, Ports: []int{9443}})
	relay := startTestDNSRelay(t, engine, nil, upstream.address)

	response := exchangeTestDNS(t, "udp", relay.Addr().String(), "a."+baseHost, dns.TypeA)
	require.Equal(t, dns.RcodeSuccess, response.Rcode)

	response = exchangeTestDNS(t, "udp", relay.Addr().String(), baseHost, dns.TypeA)
	require.Equal(t, dns.RcodeRefused, response.Rcode)

	response = exchangeTestDNS(t, "udp", relay.Addr().String(), "a.b."+baseHost, dns.TypeA)
	require.Equal(t, dns.RcodeRefused, response.Rcode)
	require.Equal(t, int64(1), upstreamQueries.Load())
}

func TestDNSRelayNormalizesNames(t *testing.T) {
	t.Parallel()

	host := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	engine := testDNSPolicy(13, codersdk.AIEgressRule{Host: host})
	relay := startTestDNSRelay(t, engine, nil, upstream.address)

	queryNames := []string{host, strings.ToUpper(host), strings.ToUpper(host) + "."}
	for _, queryName := range queryNames {
		response := exchangeTestDNS(t, "udp", relay.Addr().String(), queryName, dns.TypeA)
		require.Equal(t, dns.RcodeSuccess, response.Rcode, queryName)
	}
	require.Equal(t, int64(len(queryNames)), upstreamQueries.Load())
}

func TestDNSRelayRefusesDisallowedQueryTypes(t *testing.T) {
	t.Parallel()

	host := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	engine := testDNSPolicy(14, codersdk.AIEgressRule{Host: host})
	relay := startTestDNSRelay(t, engine, nil, upstream.address)

	for _, queryType := range []uint16{dns.TypeTXT, dns.TypeMX} {
		response := exchangeTestDNS(t, "udp", relay.Addr().String(), host, queryType)
		require.Equal(t, dns.RcodeRefused, response.Rcode, dns.TypeToString[queryType])
	}
	require.Zero(t, upstreamQueries.Load())
}

func TestDNSRelayRefusesInvalidQuestionCounts(t *testing.T) {
	t.Parallel()

	host := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	engine := testDNSPolicy(15, codersdk.AIEgressRule{Host: host})
	relay := startTestDNSRelay(t, engine, nil, upstream.address)

	zeroQuestions := &dns.Msg{MsgHdr: dns.MsgHdr{Id: dns.Id(), RecursionDesired: true}}
	twoQuestions := new(dns.Msg).SetQuestion(dns.Fqdn(host), dns.TypeA)
	twoQuestions.Question = append(twoQuestions.Question, dns.Question{
		Name:   dns.Fqdn("other." + host),
		Qtype:  dns.TypeAAAA,
		Qclass: dns.ClassINET,
	})
	for _, request := range []*dns.Msg{zeroQuestions, twoQuestions} {
		response := exchangeTestDNSMessage(t, "udp", relay.Addr().String(), request)
		require.Equal(t, dns.RcodeRefused, response.Rcode)
	}
	require.Zero(t, upstreamQueries.Load())
}

func TestDNSRelayMalformedMessage(t *testing.T) {
	t.Parallel()

	host := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	engine := testDNSPolicy(16, codersdk.AIEgressRule{Host: host})
	relay := startTestDNSRelay(t, engine, nil, upstream.address)

	connection, err := net.Dial("udp4", relay.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = connection.Close()
	})
	ctx := testutil.Context(t, testutil.WaitShort)
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.NoError(t, connection.SetDeadline(deadline))

	_, err = connection.Write([]byte{0, 1, 2, 3})
	require.NoError(t, err)
	request := new(dns.Msg).SetQuestion(dns.Fqdn(host), dns.TypeA)
	packedRequest, err := request.Pack()
	require.NoError(t, err)
	_, err = connection.Write(packedRequest)
	require.NoError(t, err)

	buffer := make([]byte, dns.MaxMsgSize)
	for {
		read, err := connection.Read(buffer)
		require.NoError(t, err)
		response := new(dns.Msg)
		if err := response.Unpack(buffer[:read]); err != nil || response.Id != request.Id {
			continue
		}
		require.Equal(t, dns.RcodeSuccess, response.Rcode)
		break
	}
	require.Equal(t, int64(1), upstreamQueries.Load())
}

func TestDNSRelayTCP(t *testing.T) {
	t.Parallel()

	host := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, net.IPv4(192, 0, 2, 20)))
	engine := testDNSPolicy(17, codersdk.AIEgressRule{Host: host})
	relay := startTestDNSRelay(t, engine, nil, upstream.address)

	response := exchangeTestDNS(t, "tcp", relay.Addr().String(), host, dns.TypeA)
	require.Equal(t, dns.RcodeSuccess, response.Rcode)
	require.Len(t, response.Answer, 1)
	require.Equal(t, int64(1), upstreamQueries.Load())
}

func TestDNSRelayPolicyUpdate(t *testing.T) {
	t.Parallel()

	host := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	engine := testDNSPolicy(18)
	events := make(chan DNSQueryEvent, 2)
	relay := startTestDNSRelay(t, engine, func(event DNSQueryEvent) {
		events <- event
	}, upstream.address)

	response := exchangeTestDNS(t, "udp", relay.Addr().String(), host, dns.TypeA)
	require.Equal(t, dns.RcodeRefused, response.Rcode)
	ctx := testutil.Context(t, testutil.WaitShort)
	deniedEvent := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, DNSQueryDecisionDenied, deniedEvent.Decision)
	require.Equal(t, int64(18), deniedEvent.PolicyRevision)

	engine.Update(codersdk.AIEgressPolicy{
		Revision: 19,
		Rules: []codersdk.AIEgressRule{{
			Host:  host,
			Ports: []int{12345},
		}},
	})
	response = exchangeTestDNS(t, "udp", relay.Addr().String(), host, dns.TypeA)
	require.Equal(t, dns.RcodeSuccess, response.Rcode)
	allowedEvent := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, DNSQueryDecisionAllowed, allowedEvent.Decision)
	require.Equal(t, int64(19), allowedEvent.PolicyRevision)
	require.Equal(t, int64(1), upstreamQueries.Load())
}

func TestDNSRelayQueryEvents(t *testing.T) {
	t.Parallel()

	allowedHost := testDNSHost(t)
	deniedHost := "blocked." + allowedHost
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	engine := testDNSPolicy(20, codersdk.AIEgressRule{Host: allowedHost})
	events := make(chan DNSQueryEvent, 2)
	relay := startTestDNSRelay(t, engine, func(event DNSQueryEvent) {
		events <- event
	}, upstream.address)
	ctx := testutil.Context(t, testutil.WaitShort)

	response := exchangeTestDNS(t, "udp", relay.Addr().String(), strings.ToUpper(allowedHost)+".", dns.TypeAAAA)
	require.Equal(t, dns.RcodeSuccess, response.Rcode)
	require.Equal(t, DNSQueryEvent{
		QName:          allowedHost,
		QType:          dns.TypeAAAA,
		Decision:       DNSQueryDecisionAllowed,
		PolicyRevision: 20,
	}, testutil.RequireReceive(ctx, t, events))

	response = exchangeTestDNS(t, "udp", relay.Addr().String(), strings.ToUpper(deniedHost)+".", dns.TypeCNAME)
	require.Equal(t, dns.RcodeRefused, response.Rcode)
	require.Equal(t, DNSQueryEvent{
		QName:          deniedHost,
		QType:          dns.TypeCNAME,
		Decision:       DNSQueryDecisionDenied,
		PolicyRevision: 20,
	}, testutil.RequireReceive(ctx, t, events))
	require.Equal(t, int64(1), upstreamQueries.Load())
}

func TestDNSRelayUpstreamFailure(t *testing.T) {
	t.Parallel()

	host := testDNSHost(t)
	var upstreamQueries atomic.Int64
	upstream := startTestDNSUpstream(t, testDNSHandler(&upstreamQueries, nil))
	engine := testDNSPolicy(21, codersdk.AIEgressRule{Host: host})
	events := make(chan DNSQueryEvent, 1)
	relay := startTestDNSRelay(t, engine, func(event DNSQueryEvent) {
		events <- event
	}, upstream.address)
	upstream.stop(t)

	response := exchangeTestDNS(t, "udp", relay.Addr().String(), host, dns.TypeA)
	require.Equal(t, dns.RcodeServerFailure, response.Rcode)
	ctx := testutil.Context(t, testutil.WaitShort)
	require.Equal(t, DNSQueryEvent{
		QName:          host,
		QType:          dns.TypeA,
		Decision:       DNSQueryDecisionError,
		PolicyRevision: 21,
	}, testutil.RequireReceive(ctx, t, events))
	require.Zero(t, upstreamQueries.Load())
}

type testDNSUpstream struct {
	server   *dns.Server
	address  string
	done     chan error
	stopOnce sync.Once
	stopErr  error
	serveErr error
}

func startTestDNSUpstream(t *testing.T, handler dns.Handler) *testDNSUpstream {
	t.Helper()

	packetConnection, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	localAddress := packetConnection.LocalAddr()
	require.NotNil(t, localAddress)
	started := make(chan struct{})
	upstream := &testDNSUpstream{
		server: &dns.Server{
			PacketConn: packetConnection,
			Handler:    handler,
			NotifyStartedFunc: func() {
				close(started)
			},
		},
		address: localAddress.String(),
		done:    make(chan error, 1),
	}
	go func() {
		upstream.done <- upstream.server.ActivateAndServe()
	}()

	ctx := testutil.Context(t, testutil.WaitShort)
	select {
	case <-started:
	case err := <-upstream.done:
		require.FailNow(t, "DNS upstream stopped before startup", "error: %v", err)
	case <-ctx.Done():
		require.NoError(t, ctx.Err(), "start DNS upstream")
	}
	t.Cleanup(func() {
		upstream.stop(t)
	})
	return upstream
}

func (u *testDNSUpstream) stop(t *testing.T) {
	t.Helper()

	u.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		u.stopErr = u.server.ShutdownContext(ctx)
		select {
		case u.serveErr = <-u.done:
		case <-ctx.Done():
			u.serveErr = ctx.Err()
		}
	})
	require.NoError(t, u.stopErr)
	require.NoError(t, u.serveErr)
}

func startTestDNSRelay(t *testing.T, engine *PolicyEngine, callback DNSQueryCallback, upstream string) *Relay {
	t.Helper()

	relay, err := newRelay("127.0.0.1:0", engine, callback, upstream)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, relay.Close())
	})
	return relay
}

func testDNSPolicy(revision int64, rules ...codersdk.AIEgressRule) *PolicyEngine {
	engine := NewPolicyEngine("", 0)
	engine.Update(codersdk.AIEgressPolicy{Revision: revision, Rules: rules})
	return engine
}

func testDNSHandler(upstreamQueries *atomic.Int64, answerAddress net.IP) dns.Handler {
	return dns.HandlerFunc(func(writer dns.ResponseWriter, request *dns.Msg) {
		upstreamQueries.Add(1)
		response := new(dns.Msg).SetReply(request)
		if answerAddress != nil && len(request.Question) == 1 && request.Question[0].Qtype == dns.TypeA {
			response.Answer = []dns.RR{&dns.A{
				Hdr: dns.RR_Header{
					Name:   request.Question[0].Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: answerAddress.To4(),
			}}
		}
		_ = writer.WriteMsg(response)
	})
}

func exchangeTestDNS(t *testing.T, network, address, host string, queryType uint16) *dns.Msg {
	t.Helper()

	request := new(dns.Msg).SetQuestion(dns.Fqdn(host), queryType)
	return exchangeTestDNSMessage(t, network, address, request)
}

func exchangeTestDNSMessage(t *testing.T, network, address string, request *dns.Msg) *dns.Msg {
	t.Helper()

	client := &dns.Client{
		Net:          network,
		DialTimeout:  testutil.WaitShort,
		ReadTimeout:  testutil.WaitShort,
		WriteTimeout: testutil.WaitShort,
	}
	ctx := testutil.Context(t, testutil.WaitShort)
	response, _, err := client.ExchangeContext(ctx, request, address)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
}

func testDNSHost(t *testing.T) string {
	t.Helper()

	label := strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(strings.ToLower(t.Name()))
	return label + ".example.test"
}
