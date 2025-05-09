package vpn

import (
	"context"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"tailscale.com/util/dnsname"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

func newFakeClient(ctx context.Context, t *testing.T) *fakeClient {
	return &fakeClient{
		t:   t,
		ctx: ctx,
		ch:  make(chan *fakeConn, 1),
	}
}

type fakeClient struct {
	t   *testing.T
	ctx context.Context
	ch  chan *fakeConn
}

var _ Client = (*fakeClient)(nil)

func (f *fakeClient) NewConn(context.Context, *url.URL, string, *Options) (Conn, error) {
	select {
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	case conn := <-f.ch:
		return conn, nil
	}
}

func newFakeConn(state tailnet.WorkspaceUpdate, hsTime time.Time) *fakeConn {
	return &fakeConn{
		closed: make(chan struct{}),
		state:  state,
		hsTime: hsTime,
	}
}

type fakeConn struct {
	state   tailnet.WorkspaceUpdate
	hsTime  time.Time
	closed  chan struct{}
	doClose sync.Once
}

var _ Conn = (*fakeConn)(nil)

func (f *fakeConn) CurrentWorkspaceState() (tailnet.WorkspaceUpdate, error) {
	return f.state, nil
}

func (f *fakeConn) GetPeerDiagnostics(uuid.UUID) tailnet.PeerDiagnostics {
	return tailnet.PeerDiagnostics{
		LastWireguardHandshake: f.hsTime,
	}
}

func (f *fakeConn) Close() error {
	f.doClose.Do(func() {
		close(f.closed)
	})
	return nil
}

func TestTunnel_StartStop(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := newFakeClient(ctx, t)
	conn := newFakeConn(tailnet.WorkspaceUpdate{}, time.Time{})

	_, mgr := setupTunnel(t, ctx, client, quartz.NewMock(t))

	errCh := make(chan error, 1)
	var resp *TunnelMessage
	// When: we start the tunnel
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					TunnelFileDescriptor: 2,
					CoderUrl:             "https://coder.example.com",
					ApiToken:             "fakeToken",
					Headers: []*StartRequest_Header{
						{Name: "X-Test-Header", Value: "test"},
					},
					DeviceOs:            "macOS",
					DeviceId:            "device001",
					CoderDesktopVersion: "0.24.8",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	// Then: `NewConn` is called,
	testutil.RequireSend(ctx, t, client.ch, conn)
	// And: a response is received
	err := testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	_, ok := resp.Msg.(*TunnelMessage_Start)
	require.True(t, ok)

	// When: we stop the tunnel
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Stop{},
		})
		resp = r
		errCh <- err
	}()
	// Then: `Close` is called on the connection
	testutil.TryReceive(ctx, t, conn.closed)
	// And: a Stop response is received
	err = testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	_, ok = resp.Msg.(*TunnelMessage_Stop)
	require.True(t, ok)

	err = mgr.Close()
	require.NoError(t, err)
}

func TestTunnel_PeerUpdate(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	wsID1 := uuid.UUID{1}
	wsID2 := uuid.UUID{2}

	client := newFakeClient(ctx, t)
	conn := newFakeConn(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{
				ID: wsID1,
			},
			{
				ID: wsID2,
			},
		},
	}, time.Time{})

	tun, mgr := setupTunnel(t, ctx, client, quartz.NewMock(t))

	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					TunnelFileDescriptor: 2,
					CoderUrl:             "https://coder.example.com",
					ApiToken:             "fakeToken",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	testutil.RequireSend(ctx, t, client.ch, conn)
	err := testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	_, ok := resp.Msg.(*TunnelMessage_Start)
	require.True(t, ok)

	// When: we inform the tunnel of a WorkspaceUpdate
	err = tun.Update(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{
				ID: wsID2,
			},
		},
	})
	require.NoError(t, err)
	// Then: the tunnel sends a PeerUpdate message
	req := testutil.TryReceive(ctx, t, mgr.requests)
	require.Nil(t, req.msg.Rpc)
	require.NotNil(t, req.msg.GetPeerUpdate())
	require.Len(t, req.msg.GetPeerUpdate().UpsertedWorkspaces, 1)
	require.Equal(t, wsID2[:], req.msg.GetPeerUpdate().UpsertedWorkspaces[0].Id)

	// When: the manager requests a PeerUpdate
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_GetPeerUpdate{},
		})
		resp = r
		errCh <- err
	}()
	// Then: a PeerUpdate message is sent using the Conn's state
	err = testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	_, ok = resp.Msg.(*TunnelMessage_PeerUpdate)
	require.True(t, ok)
	require.Len(t, resp.GetPeerUpdate().UpsertedWorkspaces, 2)
	require.Equal(t, wsID1[:], resp.GetPeerUpdate().UpsertedWorkspaces[0].Id)
	require.Equal(t, wsID2[:], resp.GetPeerUpdate().UpsertedWorkspaces[1].Id)
}

func TestTunnel_NetworkSettings(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	client := newFakeClient(ctx, t)
	conn := newFakeConn(tailnet.WorkspaceUpdate{}, time.Time{})

	tun, mgr := setupTunnel(t, ctx, client, quartz.NewMock(t))

	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					TunnelFileDescriptor: 2,
					CoderUrl:             "https://coder.example.com",
					ApiToken:             "fakeToken",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	testutil.RequireSend(ctx, t, client.ch, conn)
	err := testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	_, ok := resp.Msg.(*TunnelMessage_Start)
	require.True(t, ok)

	// When: we inform the tunnel of network settings
	go func() {
		err := tun.ApplyNetworkSettings(ctx, &NetworkSettingsRequest{
			Mtu: 1200,
		})
		errCh <- err
	}()
	// Then: the tunnel sends a NetworkSettings message
	req := testutil.TryReceive(ctx, t, mgr.requests)
	require.NotNil(t, req.msg.Rpc)
	require.Equal(t, uint32(1200), req.msg.GetNetworkSettings().Mtu)
	go func() {
		testutil.RequireSend(ctx, t, mgr.sendCh, &ManagerMessage{
			Rpc: &RPC{ResponseTo: req.msg.Rpc.MsgId},
			Msg: &ManagerMessage_NetworkSettings{
				NetworkSettings: &NetworkSettingsResponse{
					Success: true,
				},
			},
		})
	}()
	// And: `ApplyNetworkSettings` returns without error once the manager responds
	err = testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
}

func TestUpdater_createPeerUpdate(t *testing.T) {
	t.Parallel()

	w1ID := uuid.UUID{1}
	w2ID := uuid.UUID{2}
	w1a1ID := uuid.UUID{4}
	w2a1ID := uuid.UUID{5}
	w1a1IP := netip.MustParseAddr("fd60:627a:a42b:0101::")
	w2a1IP := netip.MustParseAddr("fd60:627a:a42b:0301::")

	ctx := testutil.Context(t, testutil.WaitShort)

	hsTime := time.Now().Add(-time.Minute).UTC()
	updater := updater{
		ctx:         ctx,
		netLoopDone: make(chan struct{}),
		agents:      map[uuid.UUID]tailnet.Agent{},
		workspaces:  map[uuid.UUID]tailnet.Workspace{},
		conn:        newFakeConn(tailnet.WorkspaceUpdate{}, hsTime),
	}

	update := updater.createPeerUpdateLocked(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{ID: w1ID, Name: "w1", Status: proto.Workspace_STARTING},
		},
		UpsertedAgents: []*tailnet.Agent{
			{
				ID: w1a1ID, Name: "w1a1", WorkspaceID: w1ID,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"w1.coder.":            {w1a1IP},
					"w1a1.w1.me.coder.":    {w1a1IP},
					"w1a1.w1.testy.coder.": {w1a1IP},
				},
			},
		},
		DeletedWorkspaces: []*tailnet.Workspace{
			{ID: w2ID, Name: "w2", Status: proto.Workspace_STOPPED},
		},
		DeletedAgents: []*tailnet.Agent{
			{
				ID: w2a1ID, Name: "w2a1", WorkspaceID: w2ID,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"w2.coder.":            {w2a1IP},
					"w2a1.w2.me.coder.":    {w2a1IP},
					"w2a1.w2.testy.coder.": {w2a1IP},
				},
			},
		},
	})
	require.Len(t, update.UpsertedAgents, 1)
	slices.SortFunc(update.UpsertedAgents[0].Fqdn, strings.Compare)
	slices.SortFunc(update.DeletedAgents[0].Fqdn, strings.Compare)
	require.Equal(t, update, &PeerUpdate{
		UpsertedWorkspaces: []*Workspace{
			{Id: w1ID[:], Name: "w1", Status: Workspace_Status(proto.Workspace_STARTING)},
		},
		UpsertedAgents: []*Agent{
			{
				Id: w1a1ID[:], Name: "w1a1", WorkspaceId: w1ID[:],
				Fqdn:          []string{"w1.coder.", "w1a1.w1.me.coder.", "w1a1.w1.testy.coder."},
				IpAddrs:       []string{w1a1IP.String()},
				LastHandshake: timestamppb.New(hsTime),
			},
		},
		DeletedWorkspaces: []*Workspace{
			{Id: w2ID[:], Name: "w2", Status: Workspace_Status(proto.Workspace_STOPPED)},
		},
		DeletedAgents: []*Agent{
			{
				Id: w2a1ID[:], Name: "w2a1", WorkspaceId: w2ID[:],
				Fqdn:          []string{"w2.coder.", "w2a1.w2.me.coder.", "w2a1.w2.testy.coder."},
				IpAddrs:       []string{w2a1IP.String()},
				LastHandshake: nil,
			},
		},
	})
}

func TestTunnel_sendAgentUpdate(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	mClock := quartz.NewMock(t)

	wID1 := uuid.UUID{1}
	aID1 := uuid.UUID{2}
	aID2 := uuid.UUID{3}
	hsTime := time.Now().Add(-time.Minute).UTC()

	client := newFakeClient(ctx, t)
	conn := newFakeConn(tailnet.WorkspaceUpdate{}, hsTime)

	tun, mgr := setupTunnel(t, ctx, client, mClock)
	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					TunnelFileDescriptor: 2,
					CoderUrl:             "https://coder.example.com",
					ApiToken:             "fakeToken",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	testutil.RequireSend(ctx, t, client.ch, conn)
	err := testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	_, ok := resp.Msg.(*TunnelMessage_Start)
	require.True(t, ok)

	// Inform the tunnel of the initial state
	err = tun.Update(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{
				ID: wID1, Name: "w1", Status: proto.Workspace_STARTING,
			},
		},
		UpsertedAgents: []*tailnet.Agent{
			{
				ID:          aID1,
				Name:        "agent1",
				WorkspaceID: wID1,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"agent1.coder.": {netip.MustParseAddr("fd60:627a:a42b:0101::")},
				},
			},
		},
	})
	require.NoError(t, err)
	req := testutil.TryReceive(ctx, t, mgr.requests)
	require.Nil(t, req.msg.Rpc)
	require.NotNil(t, req.msg.GetPeerUpdate())
	require.Len(t, req.msg.GetPeerUpdate().UpsertedAgents, 1)
	require.Equal(t, aID1[:], req.msg.GetPeerUpdate().UpsertedAgents[0].Id)

	// `sendAgentUpdate` produces the same PeerUpdate message until an agent
	// update is received
	for range 2 {
		mClock.AdvanceNext()
		// Then: the tunnel sends a PeerUpdate message of agent upserts,
		// with the last handshake and latency set
		req = testutil.TryReceive(ctx, t, mgr.requests)
		require.Nil(t, req.msg.Rpc)
		require.NotNil(t, req.msg.GetPeerUpdate())
		require.Len(t, req.msg.GetPeerUpdate().UpsertedAgents, 1)
		require.Equal(t, aID1[:], req.msg.GetPeerUpdate().UpsertedAgents[0].Id)
		require.Equal(t, hsTime, req.msg.GetPeerUpdate().UpsertedAgents[0].LastHandshake.AsTime())
	}

	// Upsert a new agent
	err = tun.Update(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{},
		UpsertedAgents: []*tailnet.Agent{
			{
				ID:          aID2,
				Name:        "agent2",
				WorkspaceID: wID1,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"agent2.coder.": {netip.MustParseAddr("fd60:627a:a42b:0101::")},
				},
			},
		},
	})
	require.NoError(t, err)
	testutil.TryReceive(ctx, t, mgr.requests)

	// The new update includes the new agent
	mClock.AdvanceNext()
	req = testutil.TryReceive(ctx, t, mgr.requests)
	require.Nil(t, req.msg.Rpc)
	require.NotNil(t, req.msg.GetPeerUpdate())
	require.Len(t, req.msg.GetPeerUpdate().UpsertedAgents, 2)
	slices.SortFunc(req.msg.GetPeerUpdate().UpsertedAgents, func(a, b *Agent) int {
		return strings.Compare(a.Name, b.Name)
	})

	require.Equal(t, aID1[:], req.msg.GetPeerUpdate().UpsertedAgents[0].Id)
	require.Equal(t, hsTime, req.msg.GetPeerUpdate().UpsertedAgents[0].LastHandshake.AsTime())
	require.Equal(t, aID2[:], req.msg.GetPeerUpdate().UpsertedAgents[1].Id)
	require.Equal(t, hsTime, req.msg.GetPeerUpdate().UpsertedAgents[1].LastHandshake.AsTime())

	// Delete an agent
	err = tun.Update(tailnet.WorkspaceUpdate{
		DeletedAgents: []*tailnet.Agent{
			{
				ID:          aID1,
				Name:        "agent1",
				WorkspaceID: wID1,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"agent1.coder.": {netip.MustParseAddr("fd60:627a:a42b:0101::")},
				},
			},
		},
	})
	require.NoError(t, err)
	testutil.TryReceive(ctx, t, mgr.requests)

	// The new update doesn't include the deleted agent
	mClock.AdvanceNext()
	req = testutil.TryReceive(ctx, t, mgr.requests)
	require.Nil(t, req.msg.Rpc)
	require.NotNil(t, req.msg.GetPeerUpdate())
	require.Len(t, req.msg.GetPeerUpdate().UpsertedAgents, 1)
	require.Equal(t, aID2[:], req.msg.GetPeerUpdate().UpsertedAgents[0].Id)
	require.Equal(t, hsTime, req.msg.GetPeerUpdate().UpsertedAgents[0].LastHandshake.AsTime())
}

func TestTunnel_sendAgentUpdateReconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	mClock := quartz.NewMock(t)

	wID1 := uuid.UUID{1}
	aID1 := uuid.UUID{2}
	aID2 := uuid.UUID{3}

	hsTime := time.Now().Add(-time.Minute).UTC()

	client := newFakeClient(ctx, t)
	conn := newFakeConn(tailnet.WorkspaceUpdate{}, hsTime)

	tun, mgr := setupTunnel(t, ctx, client, mClock)
	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					TunnelFileDescriptor: 2,
					CoderUrl:             "https://coder.example.com",
					ApiToken:             "fakeToken",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	testutil.RequireSend(ctx, t, client.ch, conn)
	err := testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	_, ok := resp.Msg.(*TunnelMessage_Start)
	require.True(t, ok)

	// Inform the tunnel of the initial state
	err = tun.Update(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{
				ID: wID1, Name: "w1", Status: proto.Workspace_STARTING,
			},
		},
		UpsertedAgents: []*tailnet.Agent{
			{
				ID:          aID1,
				Name:        "agent1",
				WorkspaceID: wID1,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"agent1.coder.": {netip.MustParseAddr("fd60:627a:a42b:0101::")},
				},
			},
		},
	})
	require.NoError(t, err)
	req := testutil.TryReceive(ctx, t, mgr.requests)
	require.Nil(t, req.msg.Rpc)
	require.NotNil(t, req.msg.GetPeerUpdate())
	require.Len(t, req.msg.GetPeerUpdate().UpsertedAgents, 1)
	require.Equal(t, aID1[:], req.msg.GetPeerUpdate().UpsertedAgents[0].Id)

	// Upsert a new agent simulating a reconnect
	err = tun.Update(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{
				ID: wID1, Name: "w1", Status: proto.Workspace_STARTING,
			},
		},
		UpsertedAgents: []*tailnet.Agent{
			{
				ID:          aID2,
				Name:        "agent2",
				WorkspaceID: wID1,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"agent2.coder.": {netip.MustParseAddr("fd60:627a:a42b:0101::")},
				},
			},
		},
		Kind: tailnet.Snapshot,
	})
	require.NoError(t, err)

	// The new update only contains the new agent
	req = testutil.TryReceive(ctx, t, mgr.requests)
	require.Nil(t, req.msg.Rpc)
	peerUpdate := req.msg.GetPeerUpdate()
	require.NotNil(t, peerUpdate)
	require.Len(t, peerUpdate.UpsertedAgents, 1)
	require.Len(t, peerUpdate.DeletedAgents, 1)
	require.Len(t, peerUpdate.DeletedWorkspaces, 0)

	require.Equal(t, aID2[:], peerUpdate.UpsertedAgents[0].Id)
	require.Equal(t, hsTime, peerUpdate.UpsertedAgents[0].LastHandshake.AsTime())

	require.Equal(t, aID1[:], peerUpdate.DeletedAgents[0].Id)
}

func TestTunnel_sendAgentUpdateWorkspaceReconnect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	mClock := quartz.NewMock(t)

	wID1 := uuid.UUID{1}
	wID2 := uuid.UUID{2}
	aID1 := uuid.UUID{3}
	aID3 := uuid.UUID{4}

	hsTime := time.Now().Add(-time.Minute).UTC()

	client := newFakeClient(ctx, t)
	conn := newFakeConn(tailnet.WorkspaceUpdate{}, hsTime)

	tun, mgr := setupTunnel(t, ctx, client, mClock)
	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					TunnelFileDescriptor: 2,
					CoderUrl:             "https://coder.example.com",
					ApiToken:             "fakeToken",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	testutil.RequireSend(ctx, t, client.ch, conn)
	err := testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	_, ok := resp.Msg.(*TunnelMessage_Start)
	require.True(t, ok)

	// Inform the tunnel of the initial state
	err = tun.Update(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{
				ID: wID1, Name: "w1", Status: proto.Workspace_STARTING,
			},
		},
		UpsertedAgents: []*tailnet.Agent{
			{
				ID:          aID1,
				Name:        "agent1",
				WorkspaceID: wID1,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"agent1.coder.": {netip.MustParseAddr("fd60:627a:a42b:0101::")},
				},
			},
		},
	})
	require.NoError(t, err)
	req := testutil.TryReceive(ctx, t, mgr.requests)
	require.Nil(t, req.msg.Rpc)
	require.NotNil(t, req.msg.GetPeerUpdate())
	require.Len(t, req.msg.GetPeerUpdate().UpsertedAgents, 1)
	require.Equal(t, aID1[:], req.msg.GetPeerUpdate().UpsertedAgents[0].Id)

	// Upsert a new agent with a new workspace while simulating a reconnect
	err = tun.Update(tailnet.WorkspaceUpdate{
		UpsertedWorkspaces: []*tailnet.Workspace{
			{
				ID: wID2, Name: "w2", Status: proto.Workspace_STARTING,
			},
		},
		UpsertedAgents: []*tailnet.Agent{
			{
				ID:          aID3,
				Name:        "agent3",
				WorkspaceID: wID2,
				Hosts: map[dnsname.FQDN][]netip.Addr{
					"agent3.coder.": {netip.MustParseAddr("fd60:627a:a42b:0101::")},
				},
			},
		},
		Kind: tailnet.Snapshot,
	})
	require.NoError(t, err)

	// The new update only contains the new agent
	mClock.AdvanceNext()
	req = testutil.TryReceive(ctx, t, mgr.requests)
	mClock.AdvanceNext()

	require.Nil(t, req.msg.Rpc)
	peerUpdate := req.msg.GetPeerUpdate()
	require.NotNil(t, peerUpdate)
	require.Len(t, peerUpdate.UpsertedWorkspaces, 1)
	require.Len(t, peerUpdate.UpsertedAgents, 1)
	require.Len(t, peerUpdate.DeletedAgents, 1)
	require.Len(t, peerUpdate.DeletedWorkspaces, 1)

	require.Equal(t, wID2[:], peerUpdate.UpsertedWorkspaces[0].Id)
	require.Equal(t, aID3[:], peerUpdate.UpsertedAgents[0].Id)
	require.Equal(t, hsTime, peerUpdate.UpsertedAgents[0].LastHandshake.AsTime())

	require.Equal(t, aID1[:], peerUpdate.DeletedAgents[0].Id)
	require.Equal(t, wID1[:], peerUpdate.DeletedWorkspaces[0].Id)
}

//nolint:revive // t takes precedence
func setupTunnel(t *testing.T, ctx context.Context, client *fakeClient, mClock *quartz.Mock) (*Tunnel, *speaker[*ManagerMessage, *TunnelMessage, TunnelMessage]) {
	mp, tp := net.Pipe()
	t.Cleanup(func() { _ = mp.Close() })
	t.Cleanup(func() { _ = tp.Close() })
	logger := testutil.Logger(t)

	trap := mClock.Trap().NewTicker()
	defer trap.Close()

	var tun *Tunnel
	var mgr *speaker[*ManagerMessage, *TunnelMessage, TunnelMessage]

	errCh := make(chan error, 2)
	go func() {
		tunnel, err := NewTunnel(ctx, logger.Named("tunnel"), tp, client, WithClock(mClock))
		tun = tunnel
		errCh <- err
	}()
	go func() {
		manager, err := newSpeaker[*ManagerMessage, *TunnelMessage](ctx, logger.Named("manager"), mp, SpeakerRoleManager, SpeakerRoleTunnel)
		mgr = manager
		errCh <- err
	}()
	err := testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	err = testutil.TryReceive(ctx, t, errCh)
	require.NoError(t, err)
	mgr.start()

	trap.MustWait(ctx).Release()
	return tun, mgr
}

func TestProcessFreshState(t *testing.T) {
	t.Parallel()

	wsID1 := uuid.New()
	wsID2 := uuid.New()
	wsID3 := uuid.New()
	wsID4 := uuid.New()

	agentID1 := uuid.New()
	agentID2 := uuid.New()
	agentID3 := uuid.New()
	agentID4 := uuid.New()

	agent1 := tailnet.Agent{ID: agentID1, Name: "agent1", WorkspaceID: wsID1}
	agent2 := tailnet.Agent{ID: agentID2, Name: "agent2", WorkspaceID: wsID2}
	agent3 := tailnet.Agent{ID: agentID3, Name: "agent3", WorkspaceID: wsID3}
	agent4 := tailnet.Agent{ID: agentID4, Name: "agent4", WorkspaceID: wsID1}

	ws1 := tailnet.Workspace{ID: wsID1, Name: "ws1", Status: proto.Workspace_RUNNING}
	ws2 := tailnet.Workspace{ID: wsID2, Name: "ws2", Status: proto.Workspace_RUNNING}
	ws3 := tailnet.Workspace{ID: wsID3, Name: "ws3", Status: proto.Workspace_RUNNING}
	ws4 := tailnet.Workspace{ID: wsID4, Name: "ws4", Status: proto.Workspace_RUNNING}

	initialAgents := map[uuid.UUID]tailnet.Agent{
		agentID1: agent1,
		agentID2: agent2,
		agentID4: agent4,
	}
	initialWorkspaces := map[uuid.UUID]tailnet.Workspace{
		wsID1: ws1,
		wsID2: ws2,
	}

	tests := []struct {
		name              string
		initialAgents     map[uuid.UUID]tailnet.Agent
		initialWorkspaces map[uuid.UUID]tailnet.Workspace
		update            *tailnet.WorkspaceUpdate
		expectedDelete    *tailnet.WorkspaceUpdate // We only care about deletions added by the function
	}{
		{
			name:              "NoChange",
			initialAgents:     initialAgents,
			initialWorkspaces: initialWorkspaces,
			update: &tailnet.WorkspaceUpdate{
				Kind:               tailnet.Snapshot,
				UpsertedWorkspaces: []*tailnet.Workspace{&ws1, &ws2},
				UpsertedAgents:     []*tailnet.Agent{&agent1, &agent2, &agent4},
				DeletedWorkspaces:  []*tailnet.Workspace{},
				DeletedAgents:      []*tailnet.Agent{},
			},
			expectedDelete: &tailnet.WorkspaceUpdate{ // Expect no *additional* deletions
				DeletedWorkspaces: []*tailnet.Workspace{},
				DeletedAgents:     []*tailnet.Agent{},
			},
		},
		{
			name:              "AgentAdded", // Agent 3 added in update
			initialAgents:     initialAgents,
			initialWorkspaces: initialWorkspaces,
			update: &tailnet.WorkspaceUpdate{
				Kind:               tailnet.Snapshot,
				UpsertedWorkspaces: []*tailnet.Workspace{&ws1, &ws2, &ws3},
				UpsertedAgents:     []*tailnet.Agent{&agent1, &agent2, &agent3, &agent4},
				DeletedWorkspaces:  []*tailnet.Workspace{},
				DeletedAgents:      []*tailnet.Agent{},
			},
			expectedDelete: &tailnet.WorkspaceUpdate{
				DeletedWorkspaces: []*tailnet.Workspace{},
				DeletedAgents:     []*tailnet.Agent{},
			},
		},
		{
			name:              "AgentRemovedWorkspaceAlsoRemoved", // Agent 2 removed, ws2 also removed
			initialAgents:     initialAgents,
			initialWorkspaces: initialWorkspaces,
			update: &tailnet.WorkspaceUpdate{
				Kind:               tailnet.Snapshot,
				UpsertedWorkspaces: []*tailnet.Workspace{&ws1},         // ws2 not present
				UpsertedAgents:     []*tailnet.Agent{&agent1, &agent4}, // agent2 not present
				DeletedWorkspaces:  []*tailnet.Workspace{},
				DeletedAgents:      []*tailnet.Agent{},
			},
			expectedDelete: &tailnet.WorkspaceUpdate{
				DeletedWorkspaces: []*tailnet.Workspace{
					{ID: wsID2, Name: "ws2", Status: proto.Workspace_RUNNING},
				}, // Expect ws2 to be deleted
				DeletedAgents: []*tailnet.Agent{ // Expect agent2 to be deleted
					{ID: agentID2, Name: "agent2", WorkspaceID: wsID2},
				},
			},
		},
		{
			name:              "AgentRemovedWorkspaceStays", // Agent 4 removed, but ws1 stays (due to agent1)
			initialAgents:     initialAgents,
			initialWorkspaces: initialWorkspaces,
			update: &tailnet.WorkspaceUpdate{
				Kind:               tailnet.Snapshot,
				UpsertedWorkspaces: []*tailnet.Workspace{&ws1, &ws2},   // ws1 still present
				UpsertedAgents:     []*tailnet.Agent{&agent1, &agent2}, // agent4 not present
				DeletedWorkspaces:  []*tailnet.Workspace{},
				DeletedAgents:      []*tailnet.Agent{},
			},
			expectedDelete: &tailnet.WorkspaceUpdate{
				DeletedWorkspaces: []*tailnet.Workspace{}, // ws1 should NOT be deleted
				DeletedAgents: []*tailnet.Agent{ // Expect agent4 to be deleted
					{ID: agentID4, Name: "agent4", WorkspaceID: wsID1},
				},
			},
		},
		{
			name:              "InitialAgentsEmpty",
			initialAgents:     map[uuid.UUID]tailnet.Agent{}, // Start with no agents known
			initialWorkspaces: map[uuid.UUID]tailnet.Workspace{},
			update: &tailnet.WorkspaceUpdate{
				Kind:               tailnet.Snapshot,
				UpsertedWorkspaces: []*tailnet.Workspace{&ws1, &ws2},
				UpsertedAgents:     []*tailnet.Agent{&agent1, &agent2},
				DeletedWorkspaces:  []*tailnet.Workspace{},
				DeletedAgents:      []*tailnet.Agent{},
			},
			expectedDelete: &tailnet.WorkspaceUpdate{ // Expect no deletions added
				DeletedWorkspaces: []*tailnet.Workspace{},
				DeletedAgents:     []*tailnet.Agent{},
			},
		},
		{
			name:              "UpdateEmpty", // Snapshot says nothing exists
			initialAgents:     initialAgents,
			initialWorkspaces: initialWorkspaces,
			update: &tailnet.WorkspaceUpdate{
				Kind:               tailnet.Snapshot,
				UpsertedWorkspaces: []*tailnet.Workspace{},
				UpsertedAgents:     []*tailnet.Agent{},
				DeletedWorkspaces:  []*tailnet.Workspace{},
				DeletedAgents:      []*tailnet.Agent{},
			},
			expectedDelete: &tailnet.WorkspaceUpdate{ // Expect all initial agents/workspaces to be deleted
				DeletedWorkspaces: []*tailnet.Workspace{
					{ID: wsID1, Name: "ws1", Status: proto.Workspace_RUNNING},
					{ID: wsID2, Name: "ws2", Status: proto.Workspace_RUNNING},
				}, // ws1 and ws2 deleted
				DeletedAgents: []*tailnet.Agent{ // agent1, agent2, agent4 deleted
					{ID: agentID1, Name: "agent1", WorkspaceID: wsID1},
					{ID: agentID2, Name: "agent2", WorkspaceID: wsID2},
					{ID: agentID4, Name: "agent4", WorkspaceID: wsID1},
				},
			},
		},
		{
			name:              "WorkspaceWithNoAgents", // Snapshot says nothing exists
			initialAgents:     initialAgents,
			initialWorkspaces: map[uuid.UUID]tailnet.Workspace{wsID1: ws1, wsID2: ws2, wsID4: ws4}, // ws4 has no agents
			update: &tailnet.WorkspaceUpdate{
				Kind:               tailnet.Snapshot,
				UpsertedWorkspaces: []*tailnet.Workspace{&ws1, &ws2},
				UpsertedAgents:     []*tailnet.Agent{&agent1, &agent2, &agent4},
				DeletedWorkspaces:  []*tailnet.Workspace{},
				DeletedAgents:      []*tailnet.Agent{},
			},
			expectedDelete: &tailnet.WorkspaceUpdate{ // Expect all initial agents/workspaces to be deleted
				DeletedWorkspaces: []*tailnet.Workspace{
					{ID: wsID4, Name: "ws4", Status: proto.Workspace_RUNNING},
				}, // ws4 should be deleted
				DeletedAgents: []*tailnet.Agent{},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			agentsCopy := make(map[uuid.UUID]tailnet.Agent)
			maps.Copy(agentsCopy, tt.initialAgents)

			workspaceCopy := make(map[uuid.UUID]tailnet.Workspace)
			maps.Copy(workspaceCopy, tt.initialWorkspaces)

			processSnapshotUpdate(tt.update, agentsCopy, workspaceCopy)

			require.ElementsMatch(t, tt.expectedDelete.DeletedAgents, tt.update.DeletedAgents, "DeletedAgents mismatch")
			require.ElementsMatch(t, tt.expectedDelete.DeletedWorkspaces, tt.update.DeletedWorkspaces, "DeletedWorkspaces mismatch")
		})
	}
}
