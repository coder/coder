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
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/enterprise/tailnet"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	agpl "github.com/coder/coder/v2/tailnet"
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
			ctx, cancel      = context.WithTimeout(context.Background(), testutil.WaitShort)
			logger           = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			agentID          = uuid.New()
			serverMultiAgent = tailnettest.NewMockMultiAgentConn(gomock.NewController(t))
			r                = chi.NewRouter()
			srv              = httptest.NewServer(r)
		)
		defer cancel()

		r.Get("/api/v2/workspaceproxies/me/coordinate", func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			require.NoError(t, err)
			nc := websocket.NetConn(r.Context(), conn, websocket.MessageText)
			defer serverMultiAgent.Close()

			err = tailnet.ServeWorkspaceProxy(ctx, nc, serverMultiAgent)
			if !xerrors.Is(err, io.EOF) {
				assert.NoError(t, err)
			}
		})
		r.Get("/api/v2/workspaceagents/{workspaceagent}/legacy", func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(ctx, w, http.StatusOK, wsproxysdk.AgentIsLegacyResponse{
				Found:  true,
				Legacy: true,
			})
		})

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := wsproxysdk.New(u)
		client.SDKClient.SetLogger(logger)

		expected := []*agpl.Node{{
			ID:            55,
			AsOf:          time.Unix(1689653252, 0),
			Key:           key.NewNode().Public(),
			DiscoKey:      key.NewDisco().Public(),
			PreferredDERP: 0,
			DERPLatency: map[string]float64{
				"0": 1.0,
			},
			DERPForcedWebsocket: map[int]string{},
			Addresses:           []netip.Prefix{netip.PrefixFrom(netip.AddrFrom16([16]byte{1, 2, 3, 4}), 128)},
			AllowedIPs:          []netip.Prefix{netip.PrefixFrom(netip.AddrFrom16([16]byte{1, 2, 3, 4}), 128)},
			Endpoints:           []string{"192.168.1.1:18842"},
		}}
		sendNode := make(chan struct{})

		serverMultiAgent.EXPECT().NextUpdate(gomock.Any()).AnyTimes().
			DoAndReturn(func(ctx context.Context) ([]*agpl.Node, bool) {
				select {
				case <-sendNode:
					return expected, true
				case <-ctx.Done():
					return nil, false
				}
			})

		rma, err := client.DialCoordinator(ctx)
		require.NoError(t, err)

		// Subscribe
		{
			ch := make(chan struct{})
			serverMultiAgent.EXPECT().SubscribeAgent(agentID).Do(func(uuid.UUID) {
				close(ch)
			})
			require.NoError(t, rma.SubscribeAgent(agentID))
			waitOrCancel(ctx, t, ch)
		}
		// Read updated agent node
		{
			sendNode <- struct{}{}
			got, ok := rma.NextUpdate(ctx)
			assert.True(t, ok)
			got[0].AsOf = got[0].AsOf.In(time.Local)
			assert.Equal(t, *expected[0], *got[0])
		}
		// Check legacy
		{
			isLegacy := rma.AgentIsLegacy(agentID)
			assert.True(t, isLegacy)
		}
		// UpdateSelf
		{
			ch := make(chan struct{})
			serverMultiAgent.EXPECT().UpdateSelf(gomock.Any()).Do(func(node *agpl.Node) {
				node.AsOf = node.AsOf.In(time.Local)
				assert.Equal(t, expected[0], node)
				close(ch)
			})
			require.NoError(t, rma.UpdateSelf(expected[0]))
			waitOrCancel(ctx, t, ch)
		}
		// Unsubscribe
		{
			ch := make(chan struct{})
			serverMultiAgent.EXPECT().UnsubscribeAgent(agentID).Do(func(uuid.UUID) {
				close(ch)
			})
			require.NoError(t, rma.UnsubscribeAgent(agentID))
			waitOrCancel(ctx, t, ch)
		}
		// Close
		{
			ch := make(chan struct{})
			serverMultiAgent.EXPECT().Close().Do(func() {
				close(ch)
			})
			require.NoError(t, rma.Close())
			waitOrCancel(ctx, t, ch)
		}
	})
}

func waitOrCancel(ctx context.Context, t testing.TB, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("timed out waiting for channel")
	}
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
