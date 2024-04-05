package tailnet_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/tailnet/test"
	"github.com/coder/coder/v2/testutil"
)

func TestCoordinator(t *testing.T) {
	t.Parallel()
	t.Run("ClientWithoutAgent", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitMedium)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(node []*tailnet.Node) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(server, id, uuid.New())
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{
			Addresses: []netip.Prefix{
				netip.PrefixFrom(tailnet.IP(), 128),
			},
			PreferredDERP: 10,
		})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
		require.NoError(t, client.Close())
		require.NoError(t, server.Close())
		_ = testutil.RequireRecvCtx(ctx, t, errChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeChan)
	})

	t.Run("ClientWithoutAgent_InvalidIPBits", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitMedium)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(node []*tailnet.Node) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(server, id, uuid.New())
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{
			Addresses: []netip.Prefix{
				netip.PrefixFrom(tailnet.IP(), 64),
			},
			PreferredDERP: 10,
		})

		_ = testutil.RequireRecvCtx(ctx, t, errChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeChan)
	})

	t.Run("AgentWithoutClients", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitMedium)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(node []*tailnet.Node) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(server, id, "")
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{
			Addresses: []netip.Prefix{
				netip.PrefixFrom(tailnet.IPFromUUID(id), 128),
			},
			PreferredDERP: 10,
		})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
		err := client.Close()
		require.NoError(t, err)
		_ = testutil.RequireRecvCtx(ctx, t, errChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeChan)
	})

	t.Run("AgentWithoutClients_ValidIPLegacy", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitMedium)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(node []*tailnet.Node) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(server, id, "")
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{
			Addresses: []netip.Prefix{
				netip.PrefixFrom(workspacesdk.AgentIP, 128),
			},
			PreferredDERP: 10,
		})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
		err := client.Close()
		require.NoError(t, err)
		_ = testutil.RequireRecvCtx(ctx, t, errChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeChan)
	})

	t.Run("AgentWithoutClients_InvalidIP", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitMedium)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(node []*tailnet.Node) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(server, id, "")
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{
			Addresses: []netip.Prefix{
				netip.PrefixFrom(tailnet.IP(), 128),
			},
			PreferredDERP: 10,
		})
		_ = testutil.RequireRecvCtx(ctx, t, errChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeChan)
	})

	t.Run("AgentWithoutClients_InvalidBits", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx := testutil.Context(t, testutil.WaitMedium)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(node []*tailnet.Node) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(server, id, "")
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{
			Addresses: []netip.Prefix{
				netip.PrefixFrom(tailnet.IPFromUUID(id), 64),
			},
			PreferredDERP: 10,
		})
		_ = testutil.RequireRecvCtx(ctx, t, errChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeChan)
	})

	t.Run("AgentWithClient", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger)
		defer func() {
			err := coordinator.Close()
			require.NoError(t, err)
		}()

		// in this test we use real websockets to test use of deadlines
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancel()
		agentWS, agentServerWS := websocketConn(ctx, t)
		defer agentWS.Close()
		agentNodeChan := make(chan []*tailnet.Node)
		sendAgentNode, agentErrChan := tailnet.ServeCoordinator(agentWS, func(nodes []*tailnet.Node) error {
			agentNodeChan <- nodes
			return nil
		})
		agentID := uuid.New()
		closeAgentChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		sendAgentNode(&tailnet.Node{PreferredDERP: 1})
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		clientWS, clientServerWS := websocketConn(ctx, t)
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientNodeChan := make(chan []*tailnet.Node)
		sendClientNode, clientErrChan := tailnet.ServeCoordinator(clientWS, func(nodes []*tailnet.Node) error {
			clientNodeChan <- nodes
			return nil
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		agentNodes := testutil.RequireRecvCtx(ctx, t, clientNodeChan)
		require.Len(t, agentNodes, 1)

		sendClientNode(&tailnet.Node{PreferredDERP: 2})
		clientNodes := testutil.RequireRecvCtx(ctx, t, agentNodeChan)
		require.Len(t, clientNodes, 1)

		// wait longer than the internal wait timeout.
		// this tests for regression of https://github.com/coder/coder/issues/7428
		time.Sleep(tailnet.WriteTimeout * 3 / 2)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&tailnet.Node{PreferredDERP: 3})
		agentNodes = testutil.RequireRecvCtx(ctx, t, clientNodeChan)
		require.Len(t, agentNodes, 1)

		// Close the agent WebSocket so a new one can connect.
		err := agentWS.Close()
		require.NoError(t, err)
		_ = testutil.RequireRecvCtx(ctx, t, agentErrChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeAgentChan)

		// Create a new agent connection. This is to simulate a reconnect!
		agentWS, agentServerWS = net.Pipe()
		defer agentWS.Close()
		agentNodeChan = make(chan []*tailnet.Node)
		_, agentErrChan = tailnet.ServeCoordinator(agentWS, func(nodes []*tailnet.Node) error {
			agentNodeChan <- nodes
			return nil
		})
		closeAgentChan = make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		// Ensure the existing listening client sends its node immediately!
		clientNodes = testutil.RequireRecvCtx(ctx, t, agentNodeChan)
		require.Len(t, clientNodes, 1)

		err = agentWS.Close()
		require.NoError(t, err)
		_ = testutil.RequireRecvCtx(ctx, t, agentErrChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeAgentChan)

		err = clientWS.Close()
		require.NoError(t, err)
		_ = testutil.RequireRecvCtx(ctx, t, clientErrChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeClientChan)
	})

	t.Run("AgentDoubleConnect", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger)
		ctx := testutil.Context(t, testutil.WaitLong)

		agentWS1, agentServerWS1 := net.Pipe()
		defer agentWS1.Close()
		agentNodeChan1 := make(chan []*tailnet.Node)
		sendAgentNode1, agentErrChan1 := tailnet.ServeCoordinator(agentWS1, func(nodes []*tailnet.Node) error {
			t.Logf("agent1 got node update: %v", nodes)
			agentNodeChan1 <- nodes
			return nil
		})
		agentID := uuid.New()
		closeAgentChan1 := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS1, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan1)
		}()
		sendAgentNode1(&tailnet.Node{PreferredDERP: 1})
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		clientWS, clientServerWS := net.Pipe()
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientNodeChan := make(chan []*tailnet.Node)
		sendClientNode, clientErrChan := tailnet.ServeCoordinator(clientWS, func(nodes []*tailnet.Node) error {
			t.Logf("client got node update: %v", nodes)
			clientNodeChan <- nodes
			return nil
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		agentNodes := testutil.RequireRecvCtx(ctx, t, clientNodeChan)
		require.Len(t, agentNodes, 1)
		sendClientNode(&tailnet.Node{PreferredDERP: 2})
		clientNodes := testutil.RequireRecvCtx(ctx, t, agentNodeChan1)
		require.Len(t, clientNodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode1(&tailnet.Node{PreferredDERP: 3})
		agentNodes = testutil.RequireRecvCtx(ctx, t, clientNodeChan)
		require.Len(t, agentNodes, 1)

		// Create a new agent connection without disconnecting the old one.
		agentWS2, agentServerWS2 := net.Pipe()
		defer agentWS2.Close()
		agentNodeChan2 := make(chan []*tailnet.Node)
		_, agentErrChan2 := tailnet.ServeCoordinator(agentWS2, func(nodes []*tailnet.Node) error {
			t.Logf("agent2 got node update: %v", nodes)
			agentNodeChan2 <- nodes
			return nil
		})
		closeAgentChan2 := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS2, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan2)
		}()

		// Ensure the existing listening client sends it's node immediately!
		clientNodes = testutil.RequireRecvCtx(ctx, t, agentNodeChan2)
		require.Len(t, clientNodes, 1)

		// This original agent websocket should've been closed forcefully.
		_ = testutil.RequireRecvCtx(ctx, t, agentErrChan1)
		_ = testutil.RequireRecvCtx(ctx, t, closeAgentChan1)

		err := agentWS2.Close()
		require.NoError(t, err)
		_ = testutil.RequireRecvCtx(ctx, t, agentErrChan2)
		_ = testutil.RequireRecvCtx(ctx, t, closeAgentChan2)

		err = clientWS.Close()
		require.NoError(t, err)
		_ = testutil.RequireRecvCtx(ctx, t, clientErrChan)
		_ = testutil.RequireRecvCtx(ctx, t, closeClientChan)
	})

	t.Run("AgentAck", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger)
		ctx := testutil.Context(t, testutil.WaitShort)

		clientID := uuid.New()
		agentID := uuid.New()

		aReq, aRes := coordinator.Coordinate(ctx, agentID, agentID.String(), tailnet.AgentCoordinateeAuth{ID: agentID})
		cReq, cRes := coordinator.Coordinate(ctx, clientID, clientID.String(), tailnet.ClientCoordinateeAuth{AgentID: agentID})

		{
			nk, err := key.NewNode().Public().MarshalBinary()
			require.NoError(t, err)
			dk, err := key.NewDisco().Public().MarshalText()
			require.NoError(t, err)
			cReq <- &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{
				Node: &proto.Node{
					Id:    3,
					Key:   nk,
					Disco: string(dk),
				},
			}}
		}

		cReq <- &proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{
			Id: agentID[:],
		}}

		testutil.RequireRecvCtx(ctx, t, aRes)

		aReq <- &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{{
			Id: clientID[:],
		}}}
		ack := testutil.RequireRecvCtx(ctx, t, cRes)
		require.NotNil(t, ack.PeerUpdates)
		require.Len(t, ack.PeerUpdates, 1)
		require.Equal(t, proto.CoordinateResponse_PeerUpdate_READY_FOR_HANDSHAKE, ack.PeerUpdates[0].Kind)
		require.Equal(t, agentID[:], ack.PeerUpdates[0].Id)
	})

	t.Run("AgentAck_NoPermission", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger)
		ctx := testutil.Context(t, testutil.WaitShort)

		clientID := uuid.New()
		agentID := uuid.New()

		aReq, aRes := coordinator.Coordinate(ctx, agentID, agentID.String(), tailnet.AgentCoordinateeAuth{ID: agentID})
		_, _ = coordinator.Coordinate(ctx, clientID, clientID.String(), tailnet.ClientCoordinateeAuth{AgentID: agentID})

		aReq <- &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{{
			Id: clientID[:],
		}}}

		rfhError := testutil.RequireRecvCtx(ctx, t, aRes)
		require.NotEmpty(t, rfhError.Error)
	})
}

// TestCoordinator_AgentUpdateWhileClientConnects tests for regression on
// https://github.com/coder/coder/issues/7295
func TestCoordinator_AgentUpdateWhileClientConnects(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger)
	agentWS, agentServerWS := net.Pipe()
	defer agentWS.Close()

	agentID := uuid.New()
	go func() {
		err := coordinator.ServeAgent(agentServerWS, agentID, "")
		assert.NoError(t, err)
	}()

	// send an agent update before the client connects so that there is
	// node data available to send right away.
	aNode := tailnet.Node{PreferredDERP: 0}
	aData, err := json.Marshal(&aNode)
	require.NoError(t, err)
	err = agentWS.SetWriteDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	_, err = agentWS.Write(aData)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return coordinator.Node(agentID) != nil
	}, testutil.WaitShort, testutil.IntervalFast)

	// Connect from the client
	clientWS, clientServerWS := net.Pipe()
	defer clientWS.Close()
	clientID := uuid.New()
	go func() {
		err := coordinator.ServeClient(clientServerWS, clientID, agentID)
		assert.NoError(t, err)
	}()

	// peek one byte from the node update, so we know the coordinator is
	// trying to write to the client.
	// buffer needs to be 2 characters longer because return value is a list
	// so, it needs [ and ]
	buf := make([]byte, len(aData)+2)
	err = clientWS.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	n, err := clientWS.Read(buf[:1])
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// send a second update
	aNode.PreferredDERP = 1
	require.NoError(t, err)
	aData, err = json.Marshal(&aNode)
	require.NoError(t, err)
	err = agentWS.SetWriteDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	_, err = agentWS.Write(aData)
	require.NoError(t, err)

	// read the rest of the update from the client, should be initial node.
	err = clientWS.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	n, err = clientWS.Read(buf[1:])
	require.NoError(t, err)
	var cNodes []*tailnet.Node
	err = json.Unmarshal(buf[:n+1], &cNodes)
	require.NoError(t, err)
	require.Len(t, cNodes, 1)
	require.Equal(t, 0, cNodes[0].PreferredDERP)

	// read second update
	// without a fix for https://github.com/coder/coder/issues/7295 our
	// read would time out here.
	err = clientWS.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	n, err = clientWS.Read(buf)
	require.NoError(t, err)
	err = json.Unmarshal(buf[:n], &cNodes)
	require.NoError(t, err)
	require.Len(t, cNodes, 1)
	require.Equal(t, 1, cNodes[0].PreferredDERP)
}

func TestCoordinator_BidirectionalTunnels(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.BidirectionalTunnels(ctx, t, coordinator)
}

func TestCoordinator_GracefulDisconnect(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.GracefulDisconnectTest(ctx, t, coordinator)
}

func TestCoordinator_Lost(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger)
	ctx := testutil.Context(t, testutil.WaitShort)
	test.LostTest(ctx, t, coordinator)
}

func TestCoordinator_MultiAgent_CoordClose(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	coord1 := tailnet.NewCoordinator(logger.Named("coord1"))
	defer coord1.Close()

	ma1 := tailnettest.NewTestMultiAgent(t, coord1)
	defer ma1.Close()

	err := coord1.Close()
	require.NoError(t, err)

	ma1.RequireEventuallyClosed(ctx)
}

func websocketConn(ctx context.Context, t *testing.T) (client net.Conn, server net.Conn) {
	t.Helper()
	sc := make(chan net.Conn, 1)
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		wss, err := websocket.Accept(rw, r, nil)
		require.NoError(t, err)
		conn := websocket.NetConn(r.Context(), wss, websocket.MessageBinary)
		sc <- conn
		close(sc) // there can be only one

		// hold open until context canceled
		<-ctx.Done()
	}))
	t.Cleanup(s.Close)
	// nolint: bodyclose
	wsc, _, err := websocket.Dial(ctx, s.URL, nil)
	require.NoError(t, err)
	client = websocket.NetConn(ctx, wsc, websocket.MessageBinary)
	server, ok := <-sc
	require.True(t, ok)
	return client, server
}

func TestInMemoryCoordination(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	clientID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	mCoord := tailnettest.NewMockCoordinator(gomock.NewController(t))
	fConn := &fakeCoordinatee{}

	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	mCoord.EXPECT().Coordinate(gomock.Any(), clientID, gomock.Any(), tailnet.ClientCoordinateeAuth{agentID}).
		Times(1).Return(reqs, resps)

	uut := tailnet.NewInMemoryCoordination(ctx, logger, clientID, agentID, mCoord, fConn)
	defer uut.Close()

	coordinationTest(ctx, t, uut, fConn, reqs, resps, agentID)

	select {
	case err := <-uut.Error():
		require.NoError(t, err)
	default:
		// OK!
	}
}

func TestRemoteCoordination(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	clientID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	mCoord := tailnettest.NewMockCoordinator(gomock.NewController(t))
	fConn := &fakeCoordinatee{}

	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	mCoord.EXPECT().Coordinate(gomock.Any(), clientID, gomock.Any(), tailnet.ClientCoordinateeAuth{agentID}).
		Times(1).Return(reqs, resps)

	var coord tailnet.Coordinator = mCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	svc, err := tailnet.NewClientService(
		logger.Named("svc"), &coordPtr,
		time.Hour,
		func() *tailcfg.DERPMap { panic("not implemented") },
	)
	require.NoError(t, err)
	sC, cC := net.Pipe()

	serveErr := make(chan error, 1)
	go func() {
		err := svc.ServeClient(ctx, proto.CurrentVersion.String(), sC, clientID, agentID)
		serveErr <- err
	}()

	client, err := tailnet.NewDRPCClient(cC, logger)
	require.NoError(t, err)
	protocol, err := client.Coordinate(ctx)
	require.NoError(t, err)

	uut := tailnet.NewRemoteCoordination(logger.Named("coordination"), protocol, fConn, agentID)
	defer uut.Close()

	coordinationTest(ctx, t, uut, fConn, reqs, resps, agentID)

	select {
	case err := <-uut.Error():
		require.ErrorContains(t, err, "stream terminated by sending close")
	default:
		// OK!
	}
}

func TestRemoteCoordination_SendsReadyForHandshake(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	clientID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	mCoord := tailnettest.NewMockCoordinator(gomock.NewController(t))
	fConn := &fakeCoordinatee{}

	reqs := make(chan *proto.CoordinateRequest, 100)
	resps := make(chan *proto.CoordinateResponse, 100)
	mCoord.EXPECT().Coordinate(gomock.Any(), clientID, gomock.Any(), tailnet.ClientCoordinateeAuth{agentID}).
		Times(1).Return(reqs, resps)

	var coord tailnet.Coordinator = mCoord
	coordPtr := atomic.Pointer[tailnet.Coordinator]{}
	coordPtr.Store(&coord)
	svc, err := tailnet.NewClientService(
		logger.Named("svc"), &coordPtr,
		time.Hour,
		func() *tailcfg.DERPMap { panic("not implemented") },
	)
	require.NoError(t, err)
	sC, cC := net.Pipe()

	serveErr := make(chan error, 1)
	go func() {
		err := svc.ServeClient(ctx, proto.CurrentVersion.String(), sC, clientID, agentID)
		serveErr <- err
	}()

	client, err := tailnet.NewDRPCClient(cC, logger)
	require.NoError(t, err)
	protocol, err := client.Coordinate(ctx)
	require.NoError(t, err)

	uut := tailnet.NewRemoteCoordination(logger.Named("coordination"), protocol, fConn, uuid.UUID{})
	defer uut.Close()

	nk, err := key.NewNode().Public().MarshalBinary()
	require.NoError(t, err)
	dk, err := key.NewDisco().Public().MarshalText()
	require.NoError(t, err)
	testutil.RequireSendCtx(ctx, t, resps, &proto.CoordinateResponse{
		PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{{
			Id:   clientID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: &proto.Node{
				Id:    3,
				Key:   nk,
				Disco: string(dk),
			},
		}},
	})

	rfh := testutil.RequireRecvCtx(ctx, t, reqs)
	require.NotNil(t, rfh.ReadyForHandshake)
	require.Len(t, rfh.ReadyForHandshake, 1)
	require.Equal(t, clientID[:], rfh.ReadyForHandshake[0].Id)

	require.NoError(t, uut.Close())

	select {
	case err := <-uut.Error():
		require.ErrorContains(t, err, "stream terminated by sending close")
	default:
		// OK!
	}
}

// coordinationTest tests that a coordination behaves correctly
func coordinationTest(
	ctx context.Context, t *testing.T,
	uut tailnet.Coordination, fConn *fakeCoordinatee,
	reqs chan *proto.CoordinateRequest, resps chan *proto.CoordinateResponse,
	agentID uuid.UUID,
) {
	// It should add the tunnel, since we configured as a client
	req := testutil.RequireRecvCtx(ctx, t, reqs)
	require.Equal(t, agentID[:], req.GetAddTunnel().GetId())

	// when we call the callback, it should send a node update
	require.NotNil(t, fConn.callback)
	fConn.callback(&tailnet.Node{PreferredDERP: 1})

	req = testutil.RequireRecvCtx(ctx, t, reqs)
	require.Equal(t, int32(1), req.GetUpdateSelf().GetNode().GetPreferredDerp())

	// When we send a peer update, it should update the coordinatee
	nk, err := key.NewNode().Public().MarshalBinary()
	require.NoError(t, err)
	dk, err := key.NewDisco().Public().MarshalText()
	require.NoError(t, err)
	updates := []*proto.CoordinateResponse_PeerUpdate{
		{
			Id:   agentID[:],
			Kind: proto.CoordinateResponse_PeerUpdate_NODE,
			Node: &proto.Node{
				Id:    2,
				Key:   nk,
				Disco: string(dk),
			},
		},
	}
	testutil.RequireSendCtx(ctx, t, resps, &proto.CoordinateResponse{PeerUpdates: updates})
	require.Eventually(t, func() bool {
		fConn.Lock()
		defer fConn.Unlock()
		return len(fConn.updates) > 0
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Len(t, fConn.updates[0], 1)
	require.Equal(t, agentID[:], fConn.updates[0][0].Id)

	err = uut.Close()
	require.NoError(t, err)
	uut.Error()

	// When we close, it should gracefully disconnect
	req = testutil.RequireRecvCtx(ctx, t, reqs)
	require.NotNil(t, req.Disconnect)

	// It should set all peers lost on the coordinatee
	require.Equal(t, 1, fConn.setAllPeersLostCalls)
}

type fakeCoordinatee struct {
	sync.Mutex
	callback             func(*tailnet.Node)
	updates              [][]*proto.CoordinateResponse_PeerUpdate
	setAllPeersLostCalls int
	tunnelDestinations   map[uuid.UUID]struct{}
}

func (f *fakeCoordinatee) UpdatePeers(updates []*proto.CoordinateResponse_PeerUpdate) error {
	f.Lock()
	defer f.Unlock()
	f.updates = append(f.updates, updates)
	return nil
}

func (f *fakeCoordinatee) SetAllPeersLost() {
	f.Lock()
	defer f.Unlock()
	f.setAllPeersLostCalls++
}

func (f *fakeCoordinatee) SetTunnelDestination(id uuid.UUID) {
	f.Lock()
	defer f.Unlock()

	if f.tunnelDestinations == nil {
		f.tunnelDestinations = map[uuid.UUID]struct{}{}
	}
	f.tunnelDestinations[id] = struct{}{}
}

func (f *fakeCoordinatee) SetNodeCallback(callback func(*tailnet.Node)) {
	f.Lock()
	defer f.Unlock()
	f.callback = callback
}
