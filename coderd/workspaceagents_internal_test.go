package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

type fakeAgentProvider struct {
	agentConn func(ctx context.Context, agentID uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error)
}

func (fakeAgentProvider) ReverseProxy(targetURL, dashboardURL *url.URL, agentID uuid.UUID, app appurl.ApplicationURL, wildcardHost string) *httputil.ReverseProxy {
	panic("unimplemented")
}

func (f fakeAgentProvider) AgentConn(ctx context.Context, agentID uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error) {
	if f.agentConn != nil {
		return f.agentConn(ctx, agentID)
	}

	panic("unimplemented")
}

func (fakeAgentProvider) ServeHTTPDebug(w http.ResponseWriter, r *http.Request) {
	panic("unimplemented")
}

func (fakeAgentProvider) Close() error {
	return nil
}

type channelCloser struct {
	closeFn func()
}

func (c *channelCloser) Close() error {
	c.closeFn()
	return nil
}

func TestWatchAgentContainers(t *testing.T) {
	t.Parallel()

	t.Run("CoderdWebSocketCanHandleClientClosing", func(t *testing.T) {
		t.Parallel()

		// This test ensures that the agent containers `/watch` websocket can gracefully
		// handle the client websocket closing. This test was created in
		// response to this issue: https://github.com/coder/coder/issues/19449

		var (
			ctx    = testutil.Context(t, testutil.WaitLong)
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug).Named("coderd")

			mCtrl        = gomock.NewController(t)
			mDB          = dbmock.NewMockStore(mCtrl)
			mCoordinator = tailnettest.NewMockCoordinator(mCtrl)
			mAgentConn   = agentconnmock.NewMockAgentConn(mCtrl)

			fAgentProvider = fakeAgentProvider{
				agentConn: func(ctx context.Context, agentID uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error) {
					return mAgentConn, func() {}, nil
				},
			}

			workspaceID = uuid.New()
			agentID     = uuid.New()
			resourceID  = uuid.New()

			containersCh = make(chan codersdk.WorkspaceAgentListContainersResponse)

			r = chi.NewMux()

			api = API{
				ctx: ctx,
				Options: &Options{
					AgentInactiveDisconnectTimeout: testutil.WaitShort,
					Database:                       mDB,
					Logger:                         logger,
					DeploymentValues:               &codersdk.DeploymentValues{},
					TailnetCoordinator:             tailnettest.NewFakeCoordinator(),
				},
			}
		)

		var tailnetCoordinator tailnet.Coordinator = mCoordinator
		api.TailnetCoordinator.Store(&tailnetCoordinator)
		api.agentProvider = fAgentProvider

		// Setup: Allow `ExtractWorkspaceAgentParams` to complete.
		mDB.EXPECT().GetWorkspaceAgentByIDWithWorkspace(gomock.Any(), agentID).Return(database.GetWorkspaceAgentByIDWithWorkspaceRow{
			WorkspaceAgent: database.WorkspaceAgent{
				ID:               agentID,
				ResourceID:       resourceID,
				LifecycleState:   database.WorkspaceAgentLifecycleStateReady,
				FirstConnectedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
				LastConnectedAt:  sql.NullTime{Valid: true, Time: dbtime.Now()},
			},
			Workspace: database.Workspace{
				ID: workspaceID,
			},
		}, nil)

		// And: Allow `db2dsk.WorkspaceAgent` to complete.
		mCoordinator.EXPECT().Node(gomock.Any()).Return(nil)

		// And: Allow `WatchContainers` to be called, returing our `containersCh` channel.
		mAgentConn.EXPECT().WatchContainers(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ slog.Logger) (<-chan codersdk.WorkspaceAgentListContainersResponse, io.Closer, error) {
				return containersCh, &channelCloser{closeFn: func() {
					close(containersCh)
				}}, nil
			})

		// And: We mount the HTTP Handler
		r.With(httpmw.ExtractWorkspaceAgentParam(mDB)).
			Get("/workspaceagents/{workspaceagent}/containers/watch", api.watchWorkspaceAgentContainers)

		// Given: We create the HTTP server
		srv := httptest.NewServer(r)
		defer srv.Close()

		// And: Dial the WebSocket
		wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)
		conn, resp, err := websocket.Dial(ctx, fmt.Sprintf("%s/workspaceagents/%s/containers/watch", wsURL, agentID), nil)
		require.NoError(t, err)
		if resp.Body != nil {
			defer resp.Body.Close()
		}

		// And: Create a streaming decoder
		decoder := wsjson.NewDecoder[codersdk.WorkspaceAgentListContainersResponse](conn, websocket.MessageText, logger)
		defer decoder.Close()
		decodeCh := decoder.Chan()

		// And: We can successfully send through the channel.
		testutil.RequireSend(ctx, t, containersCh, codersdk.WorkspaceAgentListContainersResponse{
			Containers: []codersdk.WorkspaceAgentContainer{{
				ID: "test-container-id",
			}},
		})

		// And: Receive the data.
		containerResp := testutil.RequireReceive(ctx, t, decodeCh)
		require.Len(t, containerResp.Containers, 1)
		require.Equal(t, "test-container-id", containerResp.Containers[0].ID)

		// When: We close the WebSocket
		conn.Close(websocket.StatusNormalClosure, "test closing connection")

		// Then: We expect `containersCh` to be closed.
		select {
		case <-ctx.Done():
			t.Fail()

		case _, ok := <-containersCh:
			require.False(t, ok, "channel is expected to be closed")
		}
	})

	t.Run("CoderdWebSocketCanHandleAgentClosing", func(t *testing.T) {
		t.Parallel()

		// This test ensures that the agent containers `/watch` websocket can gracefully
		// handle the underlying websocket unexpectedly closing. This test was created in
		// response to this issue: https://github.com/coder/coder/issues/19372

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug).Named("coderd")

			mCtrl        = gomock.NewController(t)
			mDB          = dbmock.NewMockStore(mCtrl)
			mCoordinator = tailnettest.NewMockCoordinator(mCtrl)
			mAgentConn   = agentconnmock.NewMockAgentConn(mCtrl)

			fAgentProvider = fakeAgentProvider{
				agentConn: func(ctx context.Context, agentID uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error) {
					return mAgentConn, func() {}, nil
				},
			}

			workspaceID = uuid.New()
			agentID     = uuid.New()
			resourceID  = uuid.New()

			containersCh = make(chan codersdk.WorkspaceAgentListContainersResponse)

			r = chi.NewMux()

			api = API{
				ctx: ctx,
				Options: &Options{
					AgentInactiveDisconnectTimeout: testutil.WaitShort,
					Database:                       mDB,
					Logger:                         logger,
					DeploymentValues:               &codersdk.DeploymentValues{},
					TailnetCoordinator:             tailnettest.NewFakeCoordinator(),
				},
			}
		)

		var tailnetCoordinator tailnet.Coordinator = mCoordinator
		api.TailnetCoordinator.Store(&tailnetCoordinator)
		api.agentProvider = fAgentProvider

		// Setup: Allow `ExtractWorkspaceAgentParams` to complete.
		mDB.EXPECT().GetWorkspaceAgentByIDWithWorkspace(gomock.Any(), agentID).Return(database.GetWorkspaceAgentByIDWithWorkspaceRow{
			WorkspaceAgent: database.WorkspaceAgent{
				ID:               agentID,
				ResourceID:       resourceID,
				LifecycleState:   database.WorkspaceAgentLifecycleStateReady,
				FirstConnectedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
				LastConnectedAt:  sql.NullTime{Valid: true, Time: dbtime.Now()},
			},
			Workspace: database.Workspace{
				ID: workspaceID,
			},
		}, nil)

		// And: Allow `db2dsk.WorkspaceAgent` to complete.
		mCoordinator.EXPECT().Node(gomock.Any()).Return(nil)

		// And: Allow `WatchContainers` to be called, returing our `containersCh` channel.
		mAgentConn.EXPECT().WatchContainers(gomock.Any(), gomock.Any()).
			Return(containersCh, io.NopCloser(&bytes.Buffer{}), nil)

		// And: We mount the HTTP Handler
		r.With(httpmw.ExtractWorkspaceAgentParam(mDB)).
			Get("/workspaceagents/{workspaceagent}/containers/watch", api.watchWorkspaceAgentContainers)

		// Given: We create the HTTP server
		srv := httptest.NewServer(r)
		defer srv.Close()

		// And: Dial the WebSocket
		wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)
		conn, resp, err := websocket.Dial(ctx, fmt.Sprintf("%s/workspaceagents/%s/containers/watch", wsURL, agentID), nil)
		require.NoError(t, err)
		if resp.Body != nil {
			defer resp.Body.Close()
		}

		// And: Create a streaming decoder
		decoder := wsjson.NewDecoder[codersdk.WorkspaceAgentListContainersResponse](conn, websocket.MessageText, logger)
		defer decoder.Close()
		decodeCh := decoder.Chan()

		// And: We can successfully send through the channel.
		testutil.RequireSend(ctx, t, containersCh, codersdk.WorkspaceAgentListContainersResponse{
			Containers: []codersdk.WorkspaceAgentContainer{{
				ID: "test-container-id",
			}},
		})

		// And: Receive the data.
		containerResp := testutil.RequireReceive(ctx, t, decodeCh)
		require.Len(t, containerResp.Containers, 1)
		require.Equal(t, "test-container-id", containerResp.Containers[0].ID)

		// When: We close the `containersCh`
		close(containersCh)

		// Then: We expect `decodeCh` to be closed.
		select {
		case <-ctx.Done():
			t.Fail()

		case _, ok := <-decodeCh:
			require.False(t, ok, "channel is expected to be closed")
		}
	})
}
