package workspacesdk

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"nhooyr.io/websocket"
	"storj.io/drpc"
	"storj.io/drpc/drpcerr"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
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
		Logger:                  logger,
		CoordPtr:                &coordPtr,
		DERPMapUpdateFrequency:  time.Millisecond,
		DERPMapFn:               func() *tailcfg.DERPMap { return <-derpMapCh },
		NetworkTelemetryHandler: func(batch []*proto.TelemetryEvent) {},
		ResumeTokenProvider:     tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sws, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		ctx, nc := codersdk.WebsocketNetConn(r.Context(), sws, websocket.MessageBinary)
		err = svc.ServeConnV2(ctx, nc, tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{AgentID: agentID},
		})
		assert.NoError(t, err)
	}))

	fConn := newFakeTailnetConn()

	uut := newTailnetAPIConnector(ctx, logger, agentID, svr.URL, &websocket.DialOptions{})
	uut.runConnector(fConn)

	call := testutil.RequireRecvCtx(ctx, t, fCoord.CoordinateCalls)
	reqTun := testutil.RequireRecvCtx(ctx, t, call.Reqs)
	require.NotNil(t, reqTun.AddTunnel)

	_ = testutil.RequireRecvCtx(ctx, t, uut.connected)

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
}

func TestTailnetAPIConnector_UplevelVersion(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	agentID := uuid.UUID{0x55}

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sVer := apiversion.New(proto.CurrentMajor, proto.CurrentMinor-1)

		// the following matches what Coderd does;
		// c.f. coderd/workspaceagents.go: workspaceAgentClientCoordinate
		cVer := r.URL.Query().Get("version")
		if err := sVer.Validate(cVer); err != nil {
			httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
				Message: AgentAPIMismatchMessage,
				Validations: []codersdk.ValidationError{
					{Field: "version", Detail: err.Error()},
				},
			})
			return
		}
	}))

	fConn := newFakeTailnetConn()

	uut := newTailnetAPIConnector(ctx, logger, agentID, svr.URL, &websocket.DialOptions{})
	uut.runConnector(fConn)

	err := testutil.RequireRecvCtx(ctx, t, uut.connected)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Equal(t, AgentAPIMismatchMessage, sdkErr.Message)
	require.NotEmpty(t, sdkErr.Helper)
}

func TestTailnetAPIConnector_ResumeToken(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}).Leveled(slog.LevelDebug)
	agentID := uuid.UUID{0x55}
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	derpMapCh := make(chan *tailcfg.DERPMap)
	defer close(derpMapCh)

	resumeTokenProvider := tailnet.NewResumeTokenKeyProvider([64]byte{1}, time.Second)
	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                  logger,
		CoordPtr:                &coordPtr,
		DERPMapUpdateFrequency:  time.Millisecond,
		DERPMapFn:               func() *tailcfg.DERPMap { return <-derpMapCh },
		NetworkTelemetryHandler: func(batch []*proto.TelemetryEvent) {},
		ResumeTokenProvider:     resumeTokenProvider,
	})
	require.NoError(t, err)

	var (
		websocketConnCh   = make(chan *websocket.Conn, 64)
		peerIDCh          = make(chan uuid.UUID, 64)
		expectResumeToken = ""
	)
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept a resume_token query parameter to use the same peer ID. This
		// behavior matches the actual client coordinate route.
		var (
			peerID      = uuid.New()
			resumeToken = r.URL.Query().Get("resume_token")
		)
		t.Logf("received resume token: %s", resumeToken)
		assert.Equal(t, expectResumeToken, resumeToken)
		if resumeToken != "" {
			peerID, err = resumeTokenProvider.VerifyResumeToken(resumeToken)
			assert.NoError(t, err, "failed to parse resume token")
			if err != nil {
				httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
					Message: CoordinateAPIInvalidResumeToken,
					Detail:  err.Error(),
					Validations: []codersdk.ValidationError{
						{Field: "resume_token", Detail: CoordinateAPIInvalidResumeToken},
					},
				})
				return
			}
		}
		testutil.RequireSendCtx(ctx, t, peerIDCh, peerID)

		sws, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		testutil.RequireSendCtx(ctx, t, websocketConnCh, sws)
		ctx, nc := codersdk.WebsocketNetConn(r.Context(), sws, websocket.MessageBinary)
		err = svc.ServeConnV2(ctx, nc, tailnet.StreamID{
			Name: "client",
			ID:   peerID,
			Auth: tailnet.ClientCoordinateeAuth{AgentID: agentID},
		})
		assert.NoError(t, err)
	}))

	fConn := newFakeTailnetConn()

	uut := newTailnetAPIConnector(ctx, logger, agentID, svr.URL, &websocket.DialOptions{})
	uut.runConnector(fConn)

	// Wait for the resume token to be fetched for the first time.
	require.Eventually(t, func() bool {
		return uut.resumeToken.Load() != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	originalResumeToken := uut.resumeToken.Load()
	require.NotNil(t, originalResumeToken)
	expectResumeToken = originalResumeToken.Token
	t.Logf("expecting resume token: %s", expectResumeToken)

	// Sever the connection and expect it to reconnect with the resume token and
	// assume the same peer ID.
	originalPeerID := testutil.RequireRecvCtx(ctx, t, peerIDCh)
	wsConn := testutil.RequireRecvCtx(ctx, t, websocketConnCh)
	_ = wsConn.Close(websocket.StatusGoingAway, "test")

	// Wait for the resume token to be refreshed.
	require.Eventually(t, func() bool {
		rt := uut.resumeToken.Load()
		return rt != nil && rt.Token != originalResumeToken.Token
	}, testutil.WaitShort, testutil.IntervalFast)

	// Peer ID should be identical.
	require.Equal(t, originalPeerID, testutil.RequireRecvCtx(ctx, t, peerIDCh))
}

type resumeTokenProvider struct {
	genFn   func(uuid.UUID) (*proto.RefreshResumeTokenResponse, error)
	parseFn func(string) (uuid.UUID, error)
}

var _ tailnet.ResumeTokenProvider = resumeTokenProvider{}

// GenerateResumeToken implements tailnet.ResumeTokenProvider.
func (r resumeTokenProvider) GenerateResumeToken(peerID uuid.UUID) (*proto.RefreshResumeTokenResponse, error) {
	return r.genFn(peerID)
}

// VerifyResumeToken implements tailnet.ResumeTokenProvider.
func (r resumeTokenProvider) VerifyResumeToken(token string) (uuid.UUID, error) {
	return r.parseFn(token)
}

func TestTailnetAPIConnector_ResumeTokenFailure(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}).Leveled(slog.LevelDebug)
	agentID := uuid.UUID{0x55}
	fCoord := tailnettest.NewFakeCoordinator()
	var coord tailnet.Coordinator = fCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	derpMapCh := make(chan *tailcfg.DERPMap)
	defer close(derpMapCh)

	resumeTokenProvider := resumeTokenProvider{
		genFn: func(uuid.UUID) (*proto.RefreshResumeTokenResponse, error) {
			return &proto.RefreshResumeTokenResponse{
				Token:     uuid.NewString(),
				RefreshIn: durationpb.New(time.Minute),
				ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
			}, nil
		},
		parseFn: func(string) (uuid.UUID, error) {
			return uuid.UUID{}, xerrors.New("test error")
		},
	}
	svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
		Logger:                  logger,
		CoordPtr:                &coordPtr,
		DERPMapUpdateFrequency:  time.Millisecond,
		DERPMapFn:               func() *tailcfg.DERPMap { return <-derpMapCh },
		NetworkTelemetryHandler: func(batch []*proto.TelemetryEvent) {},
		ResumeTokenProvider:     resumeTokenProvider,
	})
	require.NoError(t, err)

	var (
		websocketConnCh = make(chan *websocket.Conn, 64)
		peerIDCh        = make(chan uuid.UUID, 64)
		didFail         int64
	)
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept a resume_token query parameter to use the same peer ID.
		var (
			peerID      = uuid.New()
			resumeToken = r.URL.Query().Get("resume_token")
		)
		t.Logf("received resume token: %s", resumeToken)
		if resumeToken != "" {
			_, err = resumeTokenProvider.VerifyResumeToken(resumeToken)
			assert.Error(t, err, "parse resume token should return an error")
			atomic.AddInt64(&didFail, 1)
			httpapi.Write(ctx, w, http.StatusUnauthorized, codersdk.Response{
				Message: CoordinateAPIInvalidResumeToken,
				Detail:  err.Error(),
				Validations: []codersdk.ValidationError{
					{Field: "resume_token", Detail: CoordinateAPIInvalidResumeToken},
				},
			})
			return
		}
		testutil.RequireSendCtx(ctx, t, peerIDCh, peerID)

		sws, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		testutil.RequireSendCtx(ctx, t, websocketConnCh, sws)
		ctx, nc := codersdk.WebsocketNetConn(r.Context(), sws, websocket.MessageBinary)
		err = svc.ServeConnV2(ctx, nc, tailnet.StreamID{
			Name: "client",
			ID:   peerID,
			Auth: tailnet.ClientCoordinateeAuth{AgentID: agentID},
		})
		assert.NoError(t, err)
	}))

	fConn := newFakeTailnetConn()

	uut := newTailnetAPIConnector(ctx, logger, agentID, svr.URL, &websocket.DialOptions{})
	uut.runConnector(fConn)

	// Wait for the resume token to be fetched for the first time.
	require.Eventually(t, func() bool {
		return uut.resumeToken.Load() != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	originalResumeToken := uut.resumeToken.Load()
	require.NotNil(t, originalResumeToken)

	// Sever the connection and expect it to reconnect with the resume token,
	// which should fail and cause the client to be disconnected. The client
	// should then reconnect with no resume token.
	originalPeerID := testutil.RequireRecvCtx(ctx, t, peerIDCh)
	wsConn := testutil.RequireRecvCtx(ctx, t, websocketConnCh)
	_ = wsConn.Close(websocket.StatusGoingAway, "test")

	// Wait for the resume token to be refreshed.
	require.Eventually(t, func() bool {
		rt := uut.resumeToken.Load()
		return rt != nil && rt.Token != originalResumeToken.Token
	}, testutil.WaitShort, testutil.IntervalFast)

	// Peer ID should be different.
	require.NotEqual(t, originalPeerID, testutil.RequireRecvCtx(ctx, t, peerIDCh))

	// The resume token should have failed to parse.
	require.EqualValues(t, 1, atomic.LoadInt64(&didFail))
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
			testutil.RequireSendCtx(ctx, t, eventCh, batch)
		},
		ResumeTokenProvider: tailnet.NewInsecureTestResumeTokenProvider(),
	})
	require.NoError(t, err)

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sws, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		ctx, nc := codersdk.WebsocketNetConn(r.Context(), sws, websocket.MessageBinary)
		err = svc.ServeConnV2(ctx, nc, tailnet.StreamID{
			Name: "client",
			ID:   clientID,
			Auth: tailnet.ClientCoordinateeAuth{AgentID: agentID},
		})
		assert.NoError(t, err)
	}))

	fConn := newFakeTailnetConn()

	uut := newTailnetAPIConnector(ctx, logger, agentID, svr.URL, &websocket.DialOptions{})
	uut.runConnector(fConn)
	require.Eventually(t, func() bool {
		uut.clientMu.Lock()
		defer uut.clientMu.Unlock()
		return uut.client != nil
	}, testutil.WaitShort, testutil.IntervalFast)

	uut.SendTelemetryEvent(&proto.TelemetryEvent{
		Id: []byte("test event"),
	})

	testEvents := testutil.RequireRecvCtx(ctx, t, eventCh)

	require.Len(t, testEvents, 1)
	require.Equal(t, []byte("test event"), testEvents[0].Id)
}

func TestTailnetAPIConnector_TelemetryUnimplemented(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	agentID := uuid.UUID{0x55}
	fConn := newFakeTailnetConn()

	fakeDRPCClient := newFakeDRPCClient()
	uut := &tailnetAPIConnector{
		ctx:           ctx,
		logger:        logger,
		agentID:       agentID,
		coordinateURL: "",
		dialOptions:   &websocket.DialOptions{},
		conn:          nil,
		connected:     make(chan error, 1),
		closed:        make(chan struct{}),
		customDialFn: func() (proto.DRPCTailnetClient, error) {
			return fakeDRPCClient, nil
		},
	}
	uut.runConnector(fConn)
	require.Eventually(t, func() bool {
		uut.clientMu.Lock()
		defer uut.clientMu.Unlock()
		return uut.client != nil
	}, testutil.WaitShort, testutil.IntervalFast)

	fakeDRPCClient.telemetryError = drpcerr.WithCode(xerrors.New("Unimplemented"), 0)
	uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	require.False(t, uut.telemetryUnavailable.Load())
	require.Equal(t, int64(1), atomic.LoadInt64(&fakeDRPCClient.postTelemetryCalls))

	fakeDRPCClient.telemetryError = drpcerr.WithCode(xerrors.New("Unimplemented"), drpcerr.Unimplemented)
	uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	require.True(t, uut.telemetryUnavailable.Load())
	uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	require.Equal(t, int64(2), atomic.LoadInt64(&fakeDRPCClient.postTelemetryCalls))
}

func TestTailnetAPIConnector_TelemetryNotRecognised(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	agentID := uuid.UUID{0x55}
	fConn := newFakeTailnetConn()

	fakeDRPCClient := newFakeDRPCClient()
	uut := &tailnetAPIConnector{
		ctx:           ctx,
		logger:        logger,
		agentID:       agentID,
		coordinateURL: "",
		dialOptions:   &websocket.DialOptions{},
		conn:          nil,
		connected:     make(chan error, 1),
		closed:        make(chan struct{}),
		customDialFn: func() (proto.DRPCTailnetClient, error) {
			return fakeDRPCClient, nil
		},
	}
	uut.runConnector(fConn)
	require.Eventually(t, func() bool {
		uut.clientMu.Lock()
		defer uut.clientMu.Unlock()
		return uut.client != nil
	}, testutil.WaitShort, testutil.IntervalFast)

	fakeDRPCClient.telemetryError = drpc.ProtocolError.New("Protocol Error")
	uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	require.False(t, uut.telemetryUnavailable.Load())
	require.Equal(t, int64(1), atomic.LoadInt64(&fakeDRPCClient.postTelemetryCalls))

	fakeDRPCClient.telemetryError = drpc.ProtocolError.New("unknown rpc: /coder.tailnet.v2.Tailnet/PostTelemetry")
	uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	require.True(t, uut.telemetryUnavailable.Load())
	uut.SendTelemetryEvent(&proto.TelemetryEvent{})
	require.Equal(t, int64(2), atomic.LoadInt64(&fakeDRPCClient.postTelemetryCalls))
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

type fakeDRPCClient struct {
	postTelemetryCalls int64
	refreshTokenFn     func(context.Context, *proto.RefreshResumeTokenRequest) (*proto.RefreshResumeTokenResponse, error)
	telemetryError     error
	fakeDRPPCMapStream
}

var _ proto.DRPCTailnetClient = &fakeDRPCClient{}

func newFakeDRPCClient() *fakeDRPCClient {
	return &fakeDRPCClient{
		postTelemetryCalls: 0,
		fakeDRPPCMapStream: fakeDRPPCMapStream{
			fakeDRPCStream: fakeDRPCStream{
				ch: make(chan struct{}),
			},
		},
	}
}

// Coordinate implements proto.DRPCTailnetClient.
func (f *fakeDRPCClient) Coordinate(_ context.Context) (proto.DRPCTailnet_CoordinateClient, error) {
	return &f.fakeDRPCStream, nil
}

// DRPCConn implements proto.DRPCTailnetClient.
func (*fakeDRPCClient) DRPCConn() drpc.Conn {
	return &fakeDRPCConn{}
}

// PostTelemetry implements proto.DRPCTailnetClient.
func (f *fakeDRPCClient) PostTelemetry(_ context.Context, _ *proto.TelemetryRequest) (*proto.TelemetryResponse, error) {
	atomic.AddInt64(&f.postTelemetryCalls, 1)
	return nil, f.telemetryError
}

// StreamDERPMaps implements proto.DRPCTailnetClient.
func (f *fakeDRPCClient) StreamDERPMaps(_ context.Context, _ *proto.StreamDERPMapsRequest) (proto.DRPCTailnet_StreamDERPMapsClient, error) {
	return &f.fakeDRPPCMapStream, nil
}

// RefreshResumeToken implements proto.DRPCTailnetClient.
func (f *fakeDRPCClient) RefreshResumeToken(_ context.Context, _ *proto.RefreshResumeTokenRequest) (*proto.RefreshResumeTokenResponse, error) {
	if f.refreshTokenFn != nil {
		return f.refreshTokenFn(context.Background(), nil)
	}

	return &proto.RefreshResumeTokenResponse{
		Token:     "test",
		RefreshIn: durationpb.New(30 * time.Minute),
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
	}, nil
}

type fakeDRPCConn struct{}

var _ drpc.Conn = &fakeDRPCConn{}

// Close implements drpc.Conn.
func (*fakeDRPCConn) Close() error {
	return nil
}

// Closed implements drpc.Conn.
func (*fakeDRPCConn) Closed() <-chan struct{} {
	return nil
}

// Invoke implements drpc.Conn.
func (*fakeDRPCConn) Invoke(_ context.Context, _ string, _ drpc.Encoding, _ drpc.Message, _ drpc.Message) error {
	return nil
}

// NewStream implements drpc.Conn.
func (*fakeDRPCConn) NewStream(_ context.Context, _ string, _ drpc.Encoding) (drpc.Stream, error) {
	return nil, nil
}

type fakeDRPCStream struct {
	ch chan struct{}
}

var _ proto.DRPCTailnet_CoordinateClient = &fakeDRPCStream{}

// Close implements proto.DRPCTailnet_CoordinateClient.
func (f *fakeDRPCStream) Close() error {
	close(f.ch)
	return nil
}

// CloseSend implements proto.DRPCTailnet_CoordinateClient.
func (*fakeDRPCStream) CloseSend() error {
	return nil
}

// Context implements proto.DRPCTailnet_CoordinateClient.
func (*fakeDRPCStream) Context() context.Context {
	return nil
}

// MsgRecv implements proto.DRPCTailnet_CoordinateClient.
func (*fakeDRPCStream) MsgRecv(_ drpc.Message, _ drpc.Encoding) error {
	return nil
}

// MsgSend implements proto.DRPCTailnet_CoordinateClient.
func (*fakeDRPCStream) MsgSend(_ drpc.Message, _ drpc.Encoding) error {
	return nil
}

// Recv implements proto.DRPCTailnet_CoordinateClient.
func (f *fakeDRPCStream) Recv() (*proto.CoordinateResponse, error) {
	<-f.ch
	return &proto.CoordinateResponse{}, nil
}

// Send implements proto.DRPCTailnet_CoordinateClient.
func (f *fakeDRPCStream) Send(*proto.CoordinateRequest) error {
	<-f.ch
	return nil
}

type fakeDRPPCMapStream struct {
	fakeDRPCStream
}

var _ proto.DRPCTailnet_StreamDERPMapsClient = &fakeDRPPCMapStream{}

// Recv implements proto.DRPCTailnet_StreamDERPMapsClient.
func (f *fakeDRPPCMapStream) Recv() (*proto.DERPMap, error) {
	<-f.fakeDRPCStream.ch
	return &proto.DERPMap{}, nil
}
