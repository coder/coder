package coderd

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

func TestWorkspaceAgentACPRelay(t *testing.T) {
	t.Parallel()

	t.Run("BinaryBidirectionalAndAudit", func(t *testing.T) {
		t.Parallel()
		url, agent, audited := newACPRelayTest(t, true)
		upstream := newACPWebsocketTest(t, agent)
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := dialACPRelayTest(ctx, t, url)
		remote := testutil.TryReceive(ctx, t, upstream)
		defer remote.CloseNow()
		// Frame boundaries need not coincide with NDJSON boundaries, including
		// messages larger than the websocket library's default read limit.
		payload := []byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n" +
			"{\"params\":\"" + strings.Repeat("x", 64*1024) + "\"}\n")
		for _, chunk := range [][]byte{payload[:17], payload[17 : len(payload)-3], payload[len(payload)-3:]} {
			require.NoError(t, ws.Write(ctx, websocket.MessageBinary, chunk))
		}
		requireACPRelayBytes(ctx, t, remote, payload)
		for _, chunk := range [][]byte{payload[:7], payload[7:]} {
			require.NoError(t, remote.Write(ctx, websocket.MessageBinary, chunk))
		}
		requireACPRelayBytes(ctx, t, ws, payload)
		testutil.TryReceive(ctx, t, audited)
		require.NoError(t, remote.Close(websocket.StatusNormalClosure, ""))
		_, _, err := ws.Read(ctx)
		require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
	})

	t.Run("CrossOriginRejectedBeforeAgentWork", func(t *testing.T) {
		t.Parallel()
		url, _, _ := newACPRelayTest(t, false)
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:bodyclose // websocket.Dial closes the response body.
		ws, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": []string{"https://other.example.com"}},
		})
		require.Error(t, err)
		require.Nil(t, ws)
		require.Equal(t, http.StatusForbidden, res.StatusCode)
	})

	t.Run("UnavailableHasNoSSHFallback", func(t *testing.T) {
		t.Parallel()
		url, agent, _ := newACPRelayTest(t, true)
		agent.EXPECT().ConnectNativeAgentACP(gomock.Any()).Return(nil, xerrors.New("private agent error"))
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := dialACPRelayTest(ctx, t, url)
		_, _, err := ws.Read(ctx)
		require.Equal(t, websocket.StatusInternalError, websocket.CloseStatus(err))
		require.NotContains(t, err.Error(), "private agent error")
	})

	t.Run("DisconnectClosesOnlyTransportAndAllowsReconnect", func(t *testing.T) {
		t.Parallel()
		url, agent, audited := newACPRelayTest(t, true)
		upstream := newACPWebsocketTest(t, agent)
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := dialACPRelayTest(ctx, t, url)
		remote := testutil.TryReceive(ctx, t, upstream)
		defer remote.CloseNow()
		require.NoError(t, ws.Close(websocket.StatusNormalClosure, "disconnect"))
		_, _, err := remote.Read(ctx)
		require.Error(t, err)

		// No Close or SSH method is expected on the cached AgentConn. A new
		// browser connects to the same native service through a new transport.
		second := dialACPRelayTest(ctx, t, url)
		secondRemote := testutil.TryReceive(ctx, t, upstream)
		defer secondRemote.CloseNow()
		payload := []byte("{\"method\":\"session/update\"}\n")
		require.NoError(t, secondRemote.Write(ctx, websocket.MessageBinary, payload))
		requireACPRelayBytes(ctx, t, second, payload)
		testutil.TryReceive(ctx, t, audited)
		testutil.TryReceive(ctx, t, audited)
	})

	t.Run("DisconnectCancelsPendingConnection", func(t *testing.T) {
		t.Parallel()
		url, agent, _ := newACPRelayTest(t, true)
		started, stopped := make(chan struct{}), make(chan struct{})
		agent.EXPECT().ConnectNativeAgentACP(gomock.Any()).DoAndReturn(func(ctx context.Context) (net.Conn, error) {
			close(started)
			<-ctx.Done()
			close(stopped)
			return nil, ctx.Err()
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		ws := dialACPRelayTest(ctx, t, url)
		testutil.TryReceive(ctx, t, started)
		require.NoError(t, ws.Close(websocket.StatusNormalClosure, "disconnect"))
		testutil.TryReceive(ctx, t, stopped)
	})
}

func newACPRelayTest(t *testing.T, allowAgent bool) (string, *agentconnmock.MockAgentConn, <-chan struct{}) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	agent := agentconnmock.NewMockAgentConn(ctrl)
	agentID, userID := uuid.New(), uuid.New()
	db.EXPECT().GetWorkspaceAgentAndWorkspaceByID(gomock.Any(), agentID).Return(database.GetWorkspaceAgentAndWorkspaceByIDRow{
		WorkspaceAgent: database.WorkspaceAgent{ID: agentID, OperatingSystem: "linux"},
		WorkspaceTable: database.WorkspaceTable{ID: uuid.New(), OwnerID: userID, OrganizationID: uuid.New()},
	}, nil).AnyTimes()
	audited := make(chan struct{}, 10)
	db.EXPECT().UpsertWorkspaceAppAuditSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, p database.UpsertWorkspaceAppAuditSessionParams) (bool, error) {
		require.Equal(t, agentID, p.AgentID)
		require.Equal(t, userID, p.UserID)
		require.EqualValues(t, http.StatusSwitchingProtocols, p.StatusCode)
		audited <- struct{}{}
		return false, nil
	}).AnyTimes()
	api := &API{
		ctx:         ctx,
		Options:     &Options{Logger: logger, Database: db},
		Experiments: codersdk.Experiments{codersdk.ExperimentNativeAgents},
		HTTPAuth:    &HTTPAuthorizer{Authorizer: &mockAuthorizer{}, Logger: logger},
		wsWatcher:   httpapi.NewWSWatcher(quartz.NewMock(t), nil),
		agentProvider: fakeAgentProvider{agentConn: func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.True(t, allowAgent, "must reject the request before contacting the agent")
			require.Equal(t, agentID, id)
			return agent, func() {}, nil
		}},
	}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := rbac.Subject{ID: userID.String(), Type: rbac.SubjectTypeUser}
			next.ServeHTTP(w, r.WithContext(dbauthz.As(r.Context(), actor)))
		})
	})
	router.With(httpmw.ExtractWorkspaceAgentAndWorkspaceParam(db)).Get("/{workspaceagent}", api.workspaceAgentACP)
	server := httptest.NewServer(router)
	t.Cleanup(func() {
		server.Close()
		api.WebsocketWaitGroup.Wait()
	})
	return server.URL + "/" + agentID.String(), agent, audited
}

func newACPWebsocketTest(t *testing.T, agent *agentconnmock.MockAgentConn) <-chan *websocket.Conn {
	t.Helper()
	connections := make(chan *websocket.Conn, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ws.SetReadLimit(4 << 20)
		connections <- ws
	}))
	t.Cleanup(server.Close)
	agent.EXPECT().ConnectNativeAgentACP(gomock.Any()).DoAndReturn(func(ctx context.Context) (net.Conn, error) {
		//nolint:bodyclose // websocket.Dial closes the response body.
		ws, _, err := websocket.Dial(ctx, server.URL, nil)
		if err != nil {
			return nil, err
		}
		ws.SetReadLimit(4 << 20)
		return websocket.NetConn(ctx, ws, websocket.MessageBinary), nil
	}).AnyTimes()
	return connections
}

func dialACPRelayTest(ctx context.Context, t *testing.T, url string) *websocket.Conn {
	t.Helper()
	//nolint:bodyclose // websocket.Dial closes the response body.
	ws, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)
	ws.SetReadLimit(4 << 20)
	t.Cleanup(func() { _ = ws.CloseNow() })
	return ws
}

func requireACPRelayBytes(ctx context.Context, t *testing.T, conn *websocket.Conn, want []byte) {
	t.Helper()
	got := make([]byte, len(want))
	_, err := io.ReadFull(websocket.NetConn(ctx, conn, websocket.MessageBinary), got)
	require.NoError(t, err)
	require.Equal(t, want, got)
}
