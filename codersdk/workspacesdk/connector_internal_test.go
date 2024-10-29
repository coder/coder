package workspacesdk

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func init() {
	// Give tests a bit more time to timeout. Darwin is particularly slow.
	tailnetConnectorGracefulTimeout = 5 * time.Second
}

func TestTailnetAPIConnector_Disconnects(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoredErrorIs: append(slogtest.DefaultIgnoredErrorIs,
			io.EOF,                   // we get EOF when we simulate a DERPMap error
			yamux.ErrSessionShutdown, // coordination can throw these when DERP error tears down session
		),
	}).Leveled(slog.LevelDebug)
	agentID := uuid.UUID{0x55}
	clientID := uuid.UUID{0x66}
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	derpMapCh := make(chan *tailcfg.DERPMap)
	defer close(derpMapCh)
	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                  logger.Named("svc"),
		CoordPtr:                &coordPtr,
		DERPMapUpdateFrequency:  time.Millisecond,
		DERPMapFn:               func() *tailcfg.DERPMap { return <-derpMapCh },
		NetworkTelemetryHandler: func([]*proto.TelemetryEvent) {},
		ResumeTokenProvider:     tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	dialer := &pipeDialer{
		ctx:    testCtx,
		logger: logger,
		t:      t,
		svc:    svc,
		streamID: tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{AgentID: agentID},
		},
	}

	fConn := newFakeTailnetConn()

	uut := newTailnetAPIConnector(ctx, logger.Named("tac"), agentID, dialer, quartz.NewReal())
	uut.runConnector(fConn)

	call := testutil.RequireRecvCtx(ctx, t, fCoord.CoordinateCalls)
	reqTun := testutil.RequireRecvCtx(ctx, t, call.Reqs)
	require.NotNil(t, reqTun.AddTunnel)

	// simulate a problem with DERPMaps by sending nil
	testutil.RequireSendCtx(ctx, t, derpMapCh, nil)

	// this should cause the coordinate call to hang up WITHOUT disconnecting
	reqNil := testutil.RequireRecvCtx(ctx, t, call.Reqs)
	require.Nil(t, reqNil)

	// ...and then reconnect
	call = testutil.RequireRecvCtx(ctx, t, fCoord.CoordinateCalls)
	reqTun = testutil.RequireRecvCtx(ctx, t, call.Reqs)
	require.NotNil(t, reqTun.AddTunnel)

	// canceling the context should trigger the disconnect message
	cancel()
	reqDisc := testutil.RequireRecvCtx(testCtx, t, call.Reqs)
	require.NotNil(t, reqDisc)
	require.NotNil(t, reqDisc.Disconnect)
	close(call.Resps)
}

func TestTailnetAPIConnector_TelemetrySuccess(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	agentID := uuid.UUID{0x55}
	clientID := uuid.UUID{0x66}
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	derpMapCh := make(chan *tailcfg.DERPMap)
	defer close(derpMapCh)
	eventCh := make(chan []*proto.TelemetryEvent, 1)
	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                 logger,
		CoordPtr:               &coordPtr,
		DERPMapUpdateFrequency: time.Millisecond,
		DERPMapFn:              func() *tailcfg.DERPMap { return <-derpMapCh },
		NetworkTelemetryHandler: func(batch []*proto.TelemetryEvent) {
			select {
			case <-ctx.Done():
				t.Error("timeout sending telemetry event")
			case eventCh <- batch:
				t.Log("sent telemetry batch")
			}
		},
		ResumeTokenProvider: tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	dialer := &pipeDialer{
		ctx:    ctx,
		logger: logger,
		t:      t,
		svc:    svc,
		streamID: tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{AgentID: agentID},
		},
	}

	fConn := newFakeTailnetConn()

	uut := newTailnetAPIConnector(ctx, logger, agentID, dialer, quartz.NewReal())
	uut.runConnector(fConn)
	// Coordinate calls happen _after_ telemetry is connected up, so we use this
	// to ensure telemetry is connected before sending our event
	cc := testutil.RequireRecvCtx(ctx, t, fCoord.CoordinateCalls)
	defer close(cc.Resps)

	uut.SendTelemetryEvent(&proto.TelemetryEvent{
		Id: []byte("test event"),
	})

	testEvents := testutil.RequireRecvCtx(ctx, t, eventCh)

	require.Len(t, testEvents, 1)
	require.Equal(t, []byte("test event"), testEvents[0].Id)
}

type fakeTailnetConn struct{}

func (*fakeTailnetConn) UpdatePeers([]*proto.CoordinateResponse_PeerUpdate) error {
	// TODO implement me
	panic("implement me")
}

func (*fakeTailnetConn) SetAllPeersLost() {}

func (*fakeTailnetConn) SetNodeCallback(func(*tailnet.Node)) {}

func (*fakeTailnetConn) SetDERPMap(*tailcfg.DERPMap) {}

func (*fakeTailnetConn) SetTunnelDestination(uuid.UUID) {}

func newFakeTailnetConn() *fakeTailnetConn {
	return &fakeTailnetConn{}
}

type pipeDialer struct {
	ctx      context.Context
	logger   slog.Logger
	t        testing.TB
	svc      *tailnet.ClientService
	streamID tailnet.StreamID
}

func (p *pipeDialer) Dial(_ context.Context, _ tailnet.ResumeTokenController) (tailnet.ControlProtocolClients, error) {
	s, c := net.Pipe()
	go func() {
		err := p.svc.ServeConnV2(p.ctx, s, p.streamID)
		p.logger.Debug(p.ctx, "piped tailnet service complete", slog.Error(err))
	}()
	client, err := tailnet.NewDRPCClient(c, p.logger)
	if !assert.NoError(p.t, err) {
		_ = c.Close()
		return tailnet.ControlProtocolClients{}, err
	}
	coord, err := client.Coordinate(context.Background())
	if !assert.NoError(p.t, err) {
		_ = c.Close()
		return tailnet.ControlProtocolClients{}, err
	}

	derps := &tailnet.DERPFromDRPCWrapper{}
	derps.Client, err = client.StreamDERPMaps(context.Background(), &proto.StreamDERPMapsRequest{})
	if !assert.NoError(p.t, err) {
		_ = c.Close()
		return tailnet.ControlProtocolClients{}, err
	}
	return tailnet.ControlProtocolClients{
		Closer:      client.DRPCConn(),
		Coordinator: coord,
		DERP:        derps,
		ResumeToken: client,
		Telemetry:   client,
	}, nil
}
