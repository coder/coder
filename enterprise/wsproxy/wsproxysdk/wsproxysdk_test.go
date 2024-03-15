package wsproxysdk_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/enterprise/tailnet"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func Test_IssueSignedAppTokenHTML(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			expectedProxyToken = "hi:test"
			expectedAppReq     = workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/@user/workspace/apps/slug",
				UsernameOrID:      "user",
				WorkspaceNameOrID: "workspace",
				AppSlugOrPort:     "slug",
			}
			expectedSessionToken   = "user-session-token"
			expectedSignedTokenStr = "signed-app-token"
		)
		var called int64
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&called, 1)

			assert.Equal(t, r.Method, http.MethodPost)
			assert.Equal(t, r.URL.Path, "/api/v2/workspaceproxies/me/issue-signed-app-token")
			assert.Equal(t, r.Header.Get(httpmw.WorkspaceProxyAuthTokenHeader), expectedProxyToken)

			var req workspaceapps.IssueTokenRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, req.AppRequest, expectedAppReq)
			assert.Equal(t, req.SessionToken, expectedSessionToken)

			rw.WriteHeader(http.StatusCreated)
			err = json.NewEncoder(rw).Encode(wsproxysdk.IssueSignedAppTokenResponse{
				SignedTokenStr: expectedSignedTokenStr,
			})
			assert.NoError(t, err)
		}))

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := wsproxysdk.New(u)
		client.SetSessionToken(expectedProxyToken)

		ctx := testutil.Context(t, testutil.WaitLong)

		rw := newResponseRecorder()
		tokenRes, ok := client.IssueSignedAppTokenHTML(ctx, rw, workspaceapps.IssueTokenRequest{
			AppRequest:   expectedAppReq,
			SessionToken: expectedSessionToken,
		})
		if !assert.True(t, ok) {
			t.Log("issue request failed when it should've succeeded")
			t.Log("response dump:")
			res := rw.Result()
			defer res.Body.Close()
			dump, err := httputil.DumpResponse(res, true)
			if err != nil {
				t.Logf("failed to dump response: %v", err)
			} else {
				t.Log(string(dump))
			}
			t.FailNow()
		}
		require.Equal(t, expectedSignedTokenStr, tokenRes.SignedTokenStr)
		require.False(t, rw.WasWritten())

		require.EqualValues(t, called, 1)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		var (
			expectedProxyToken     = "hi:test"
			expectedResponseStatus = http.StatusBadRequest
			expectedResponseBody   = "bad request"
		)
		var called int64
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&called, 1)

			assert.Equal(t, r.Method, http.MethodPost)
			assert.Equal(t, r.URL.Path, "/api/v2/workspaceproxies/me/issue-signed-app-token")
			assert.Equal(t, r.Header.Get(httpmw.WorkspaceProxyAuthTokenHeader), expectedProxyToken)

			rw.WriteHeader(expectedResponseStatus)
			_, _ = rw.Write([]byte(expectedResponseBody))
		}))

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := wsproxysdk.New(u)
		_ = client.SetSessionToken(expectedProxyToken)

		ctx := testutil.Context(t, testutil.WaitLong)

		rw := newResponseRecorder()
		tokenRes, ok := client.IssueSignedAppTokenHTML(ctx, rw, workspaceapps.IssueTokenRequest{
			AppRequest:   workspaceapps.Request{},
			SessionToken: "user-session-token",
		})
		require.False(t, ok)
		require.Empty(t, tokenRes)
		require.True(t, rw.WasWritten())

		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, expectedResponseStatus, res.StatusCode)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, expectedResponseBody, string(body))

		require.EqualValues(t, called, 1)
	})
}

func TestDialCoordinator(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		var (
			ctx, cancel                  = context.WithTimeout(context.Background(), testutil.WaitShort)
			logger                       = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			agentID                      = uuid.UUID{33}
			proxyID                      = uuid.UUID{44}
			mCoord                       = tailnettest.NewMockCoordinator(gomock.NewController(t))
			coord       agpl.Coordinator = mCoord
			r                            = chi.NewRouter()
			srv                          = httptest.NewServer(r)
		)
		defer cancel()
		defer srv.Close()

		coordPtr := atomic.Pointer[agpl.Coordinator]{}
		coordPtr.Store(&coord)
		cSrv, err := tailnet.NewClientService(
			logger, &coordPtr,
			time.Hour,
			func() *tailcfg.DERPMap { panic("not implemented") },
		)
		require.NoError(t, err)

		// buffer the channels here, so we don't need to read and write in goroutines to
		// avoid blocking
		reqs := make(chan *proto.CoordinateRequest, 100)
		resps := make(chan *proto.CoordinateResponse, 100)
		mCoord.EXPECT().Coordinate(gomock.Any(), proxyID, gomock.Any(), agpl.SingleTailnetCoordinateeAuth{}).
			Times(1).
			Return(reqs, resps)

		serveMACErr := make(chan error, 1)
		r.Get("/api/v2/workspaceproxies/me/coordinate", func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if !assert.NoError(t, err) {
				return
			}
			version := r.URL.Query().Get("version")
			if !assert.Equal(t, version, proto.CurrentVersion.String()) {
				return
			}
			nc := websocket.NetConn(r.Context(), conn, websocket.MessageBinary)
			err = cSrv.ServeMultiAgentClient(ctx, version, nc, proxyID)
			serveMACErr <- err
		})

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := wsproxysdk.New(u)
		client.SDKClient.SetLogger(logger)

		peerID := uuid.UUID{55}
		peerNodeKey, err := key.NewNode().Public().MarshalBinary()
		require.NoError(t, err)
		peerDiscoKey, err := key.NewDisco().Public().MarshalText()
		require.NoError(t, err)
		expected := &proto.CoordinateResponse{PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{{
			Id: peerID[:],
			Node: &proto.Node{
				Id:            55,
				AsOf:          timestamppb.New(time.Unix(1689653252, 0)),
				Key:           peerNodeKey[:],
				Disco:         string(peerDiscoKey),
				PreferredDerp: 0,
				DerpLatency: map[string]float64{
					"0": 1.0,
				},
				DerpForcedWebsocket: map[int32]string{},
				Addresses:           []string{netip.PrefixFrom(netip.AddrFrom16([16]byte{1, 2, 3, 4}), 128).String()},
				AllowedIps:          []string{netip.PrefixFrom(netip.AddrFrom16([16]byte{1, 2, 3, 4}), 128).String()},
				Endpoints:           []string{"192.168.1.1:18842"},
			},
		}}}

		rma, err := client.DialCoordinator(ctx)
		require.NoError(t, err)

		// Subscribe
		{
			require.NoError(t, rma.SubscribeAgent(agentID))

			req := testutil.RequireRecvCtx(ctx, t, reqs)
			require.Equal(t, agentID[:], req.GetAddTunnel().GetId())
		}
		// Read updated agent node
		{
			resps <- expected

			resp, ok := rma.NextUpdate(ctx)
			assert.True(t, ok)
			updates := resp.GetPeerUpdates()
			assert.Len(t, updates, 1)
			eq, err := updates[0].GetNode().Equal(expected.GetPeerUpdates()[0].GetNode())
			assert.NoError(t, err)
			assert.True(t, eq)
		}
		// UpdateSelf
		{
			require.NoError(t, rma.UpdateSelf(expected.PeerUpdates[0].GetNode()))

			req := testutil.RequireRecvCtx(ctx, t, reqs)
			eq, err := req.GetUpdateSelf().GetNode().Equal(expected.PeerUpdates[0].GetNode())
			require.NoError(t, err)
			require.True(t, eq)
		}
		// Unsubscribe
		{
			require.NoError(t, rma.UnsubscribeAgent(agentID))

			req := testutil.RequireRecvCtx(ctx, t, reqs)
			require.Equal(t, agentID[:], req.GetRemoveTunnel().GetId())
		}
		// Close
		{
			require.NoError(t, rma.Close())

			req := testutil.RequireRecvCtx(ctx, t, reqs)
			require.NotNil(t, req.Disconnect)
			close(resps)
			select {
			case <-ctx.Done():
				t.Fatal("timeout waiting for req close")
			case _, ok := <-reqs:
				require.False(t, ok, "didn't close requests")
			}
			require.Error(t, testutil.RequireRecvCtx(ctx, t, serveMACErr))
		}
	})
}

type ResponseRecorder struct {
	rw         *httptest.ResponseRecorder
	wasWritten atomic.Bool
}

var _ http.ResponseWriter = &ResponseRecorder{}

func newResponseRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		rw: httptest.NewRecorder(),
	}
}

func (r *ResponseRecorder) WasWritten() bool {
	return r.wasWritten.Load()
}

func (r *ResponseRecorder) Result() *http.Response {
	return r.rw.Result()
}

func (r *ResponseRecorder) Flush() {
	r.wasWritten.Store(true)
	r.rw.Flush()
}

func (r *ResponseRecorder) Header() http.Header {
	// Usually when retrieving the headers for the response, it means you're
	// trying to write a header.
	r.wasWritten.Store(true)
	return r.rw.Header()
}

func (r *ResponseRecorder) Write(b []byte) (int, error) {
	r.wasWritten.Store(true)
	return r.rw.Write(b)
}

func (r *ResponseRecorder) WriteHeader(statusCode int) {
	r.wasWritten.Store(true)
	r.rw.WriteHeader(statusCode)
}
