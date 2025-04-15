package vpn_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"tailscale.com/util/dnsname"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"tailscale.com/net/dns"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/vpn"
	"github.com/coder/websocket"
)

func TestClient_WorkspaceUpdates(t *testing.T) {
	t.Parallel()

	userID := uuid.UUID{1}
	wsID := uuid.UUID{2}
	peerID := uuid.UUID{3}
	agentID := uuid.UUID{4}

	testCases := []struct {
		name                string
		agentConnectionInfo workspacesdk.AgentConnectionInfo
		hostnames           []string
	}{
		{
			name:                "empty",
			agentConnectionInfo: workspacesdk.AgentConnectionInfo{},
			hostnames:           []string{"wrk.coder.", "agnt.wrk.me.coder.", "agnt.wrk.rootbeer.coder."},
		},
		{
			name:                "suffix",
			agentConnectionInfo: workspacesdk.AgentConnectionInfo{HostnameSuffix: "float"},
			hostnames:           []string{"wrk.float.", "agnt.wrk.me.float.", "agnt.wrk.rootbeer.float."},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			logger := testutil.Logger(t)

			fCoord := tailnettest.NewFakeCoordinator()
			var coord tailnet.Coordinator = fCoord
			coordPtr := atomic.Pointer[tailnet.Coordinator]{}
			coordPtr.Store(&coord)
			ctrl := gomock.NewController(t)
			mProvider := tailnettest.NewMockWorkspaceUpdatesProvider(ctrl)

			mSub := tailnettest.NewMockSubscription(ctrl)
			outUpdateCh := make(chan *proto.WorkspaceUpdate, 1)
			inUpdateCh := make(chan tailnet.WorkspaceUpdate, 1)
			mProvider.EXPECT().Subscribe(gomock.Any(), userID).Times(1).Return(mSub, nil)
			mSub.EXPECT().Updates().MinTimes(1).Return(outUpdateCh)
			mSub.EXPECT().Close().Times(1).Return(nil)

			svc, err := tailnet.NewClientService(tailnet.ClientServiceOptions{
				Logger:                   logger,
				CoordPtr:                 &coordPtr,
				DERPMapUpdateFrequency:   time.Hour,
				DERPMapFn:                func() *tailcfg.DERPMap { return &tailcfg.DERPMap{} },
				WorkspaceUpdatesProvider: mProvider,
				ResumeTokenProvider:      tailnet.NewInsecureTestResumeTokenProvider(),
			})
			require.NoError(t, err)

			user := make(chan struct{})
			connInfo := make(chan struct{})
			serveErrCh := make(chan error)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/users/me":
					httpapi.Write(ctx, w, http.StatusOK, codersdk.User{
						ReducedUser: codersdk.ReducedUser{
							MinimalUser: codersdk.MinimalUser{
								ID:       userID,
								Username: "rootbeer",
							},
						},
					})
					user <- struct{}{}

				case "/api/v2/workspaceagents/connection":
					httpapi.Write(ctx, w, http.StatusOK, tc.agentConnectionInfo)
					connInfo <- struct{}{}

				case "/api/v2/tailnet":
					// need 2.3 for WorkspaceUpdates RPC
					cVer := r.URL.Query().Get("version")
					assert.Equal(t, "2.3", cVer)

					sws, err := websocket.Accept(w, r, nil)
					if !assert.NoError(t, err) {
						return
					}
					wsCtx, nc := codersdk.WebsocketNetConn(ctx, sws, websocket.MessageBinary)
					serveErrCh <- svc.ServeConnV2(wsCtx, nc, tailnet.StreamID{
						Name: "client",
						ID:   peerID,
						// Auth can be nil as we use a mock update provider
						Auth: tailnet.ClientUserCoordinateeAuth{
							Auth: nil,
						},
					})
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			svrURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			connErrCh := make(chan error)
			connCh := make(chan vpn.Conn)
			go func() {
				conn, err := vpn.NewClient().NewConn(ctx, svrURL, "fakeToken", &vpn.Options{
					UpdateHandler: updateHandler(func(wu tailnet.WorkspaceUpdate) error {
						inUpdateCh <- wu
						return nil
					}),
					DNSConfigurator: &noopConfigurator{},
				})
				connErrCh <- err
				connCh <- conn
			}()
			testutil.RequireReceive(ctx, t, user)
			testutil.RequireReceive(ctx, t, connInfo)
			err = testutil.RequireReceive(ctx, t, connErrCh)
			require.NoError(t, err)
			conn := testutil.RequireReceive(ctx, t, connCh)

			// Send a workspace update
			update := &proto.WorkspaceUpdate{
				UpsertedWorkspaces: []*proto.Workspace{
					{
						Id:   wsID[:],
						Name: "wrk",
					},
				},
				UpsertedAgents: []*proto.Agent{
					{
						Id:          agentID[:],
						Name:        "agnt",
						WorkspaceId: wsID[:],
					},
				},
			}
			testutil.RequireSend(ctx, t, outUpdateCh, update)

			// It'll be received by the update handler
			recvUpdate := testutil.RequireReceive(ctx, t, inUpdateCh)
			require.Len(t, recvUpdate.UpsertedWorkspaces, 1)
			require.Equal(t, wsID, recvUpdate.UpsertedWorkspaces[0].ID)
			require.Len(t, recvUpdate.UpsertedAgents, 1)

			expectedHosts := map[dnsname.FQDN][]netip.Addr{}
			for _, name := range tc.hostnames {
				expectedHosts[dnsname.FQDN(name)] = []netip.Addr{tailnet.CoderServicePrefix.AddrFromUUID(agentID)}
			}

			// And be reflected on the Conn's state
			state, err := conn.CurrentWorkspaceState()
			require.NoError(t, err)
			require.Equal(t, tailnet.WorkspaceUpdate{
				UpsertedWorkspaces: []*tailnet.Workspace{
					{
						ID:   wsID,
						Name: "wrk",
					},
				},
				UpsertedAgents: []*tailnet.Agent{
					{
						ID:          agentID,
						Name:        "agnt",
						WorkspaceID: wsID,
						Hosts:       expectedHosts,
					},
				},
				DeletedWorkspaces: []*tailnet.Workspace{},
				DeletedAgents:     []*tailnet.Agent{},
			}, state)

			// Close the conn
			conn.Close()
			err = testutil.RequireReceive(ctx, t, serveErrCh)
			require.NoError(t, err)
		})
	}
}

type updateHandler func(tailnet.WorkspaceUpdate) error

func (h updateHandler) Update(u tailnet.WorkspaceUpdate) error {
	return h(u)
}

type noopConfigurator struct{}

func (*noopConfigurator) Close() error {
	return nil
}

func (*noopConfigurator) GetBaseConfig() (dns.OSConfig, error) {
	return dns.OSConfig{}, nil
}

func (*noopConfigurator) SetDNS(dns.OSConfig) error {
	return nil
}

func (*noopConfigurator) SupportsSplitDNS() bool {
	return true
}

var _ dns.OSConfigurator = (*noopConfigurator)(nil)
