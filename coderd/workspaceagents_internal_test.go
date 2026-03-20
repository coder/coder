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
	"sync"
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

func TestWatchChatGit(t *testing.T) {
	t.Parallel()

	t.Run("ChatWithNoWorkspaceReturns400", func(t *testing.T) {
		t.Parallel()

		// This test ensures that a chat with no workspace ID
		// returns a 400 error.

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug).Named("coderd")

			mCtrl = gomock.NewController(t)
			mDB   = dbmock.NewMockStore(mCtrl)

			chatID = uuid.New()

			r = chi.NewMux()

			api = API{
				ctx: ctx,
				Options: &Options{
					AgentInactiveDisconnectTimeout: testutil.WaitShort,
					Database:                       mDB,
					Logger:                         logger,
					DeploymentValues:               &codersdk.DeploymentValues{},
				},
			}
		)

		// Setup: Return a chat with no workspace ID.
		mDB.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
			ID:          chatID,
			OwnerID:     uuid.New(),
			WorkspaceID: uuid.NullUUID{Valid: false},
		}, nil)

		// And: We mount the HTTP handler.
		r.With(httpmw.ExtractChatParam(mDB)).
			Get("/chats/{chat}/stream/git", api.watchChatGit)

		// Given: We create the HTTP server.
		srv := httptest.NewServer(r)
		defer srv.Close()

		// When: We make a request.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/chats/%s/stream/git", srv.URL, chatID), nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Then: We expect a 400 response.
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("UnauthorizedUsersCannotWatch", func(t *testing.T) {
		t.Parallel()

		// This test ensures that if the chat middleware returns
		// an error (e.g. unauthorized), the handler is not
		// reached.

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug).Named("coderd")

			mCtrl = gomock.NewController(t)
			mDB   = dbmock.NewMockStore(mCtrl)

			chatID = uuid.New()

			r = chi.NewMux()

			api = API{
				ctx: ctx,
				Options: &Options{
					AgentInactiveDisconnectTimeout: testutil.WaitShort,
					Database:                       mDB,
					Logger:                         logger,
					DeploymentValues:               &codersdk.DeploymentValues{},
				},
			}
		)

		// Setup: Return an error from the DB to simulate
		// unauthorized access.
		mDB.EXPECT().GetChatByID(gomock.Any(), chatID).Return(
			database.Chat{}, sql.ErrNoRows,
		)

		// And: We mount the HTTP handler.
		r.With(httpmw.ExtractChatParam(mDB)).
			Get("/chats/{chat}/stream/git", api.watchChatGit)

		// Given: We create the HTTP server.
		srv := httptest.NewServer(r)
		defer srv.Close()

		// When: We make a request.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/chats/%s/stream/git", srv.URL, chatID), nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Then: We expect a 404 (not found) since sql.ErrNoRows
		// is treated as a 404 by httpapi.Is404Error.
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("DisconnectedAgentRejected", func(t *testing.T) {
		t.Parallel()

		// This test ensures that a chat whose workspace agent is
		// not connected returns a 400 error.

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug).Named("coderd")

			mCtrl        = gomock.NewController(t)
			mDB          = dbmock.NewMockStore(mCtrl)
			mCoordinator = tailnettest.NewMockCoordinator(mCtrl)

			chatID      = uuid.New()
			workspaceID = uuid.New()
			agentID     = uuid.New()
			resourceID  = uuid.New()

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

		// Setup: Return a chat with a valid workspace ID.
		mDB.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
			ID:          chatID,
			OwnerID:     uuid.New(),
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)

		// And: Return an agent that is disconnected (no
		// FirstConnectedAt).
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{{
				ID:             agentID,
				ResourceID:     resourceID,
				LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
			}}, nil)

		// And: Allow db2sdk.WorkspaceAgent to complete.
		mCoordinator.EXPECT().Node(gomock.Any()).Return(nil)

		// And: We mount the HTTP handler.
		r.With(httpmw.ExtractChatParam(mDB)).
			Get("/chats/{chat}/stream/git", api.watchChatGit)

		// Given: We create the HTTP server.
		srv := httptest.NewServer(r)
		defer srv.Close()

		// When: We make a request.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/chats/%s/stream/git", srv.URL, chatID), nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Then: We expect a 400 response since the agent is
		// not connected.
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("BidirectionalProxyWorks", func(t *testing.T) {
		t.Parallel()

		// This test ensures that messages flow bidirectionally
		// between the client websocket and the agent websocket
		// through the coderd proxy.

		var (
			ctx    = testutil.Context(t, testutil.WaitLong)
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug).Named("coderd")

			mCtrl        = gomock.NewController(t)
			mDB          = dbmock.NewMockStore(mCtrl)
			mCoordinator = tailnettest.NewMockCoordinator(mCtrl)
			mAgentConn   = agentconnmock.NewMockAgentConn(mCtrl)

			chatID      = uuid.New()
			workspaceID = uuid.New()
			agentID     = uuid.New()
			resourceID  = uuid.New()

			r = chi.NewMux()

			fAgentProvider = fakeAgentProvider{
				agentConn: func(ctx context.Context, aID uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error) {
					return mAgentConn, func() {}, nil
				},
			}

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

		// Setup: Create a fake agent-side websocket server that
		// we can interact with.
		agentDone := make(chan struct{})
		closeAgentDone := sync.OnceFunc(func() { close(agentDone) })
		t.Cleanup(closeAgentDone)
		agentStreamReady := make(chan *wsjson.Stream[
			codersdk.WorkspaceAgentGitClientMessage,
			codersdk.WorkspaceAgentGitServerMessage,
		], 1)
		agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := websocket.Accept(w, r, nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Create stream typed from the agent's perspective:
			// reads client messages, writes server messages.
			s := wsjson.NewStream[
				codersdk.WorkspaceAgentGitClientMessage,
				codersdk.WorkspaceAgentGitServerMessage,
			](ws, websocket.MessageText, websocket.MessageText, logger)
			agentStreamReady <- s
			// Keep the handler alive until test signals done.
			<-agentDone
		}))
		defer agentSrv.Close()

		// And: Return a chat with a valid workspace ID.
		mDB.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
			ID:          chatID,
			OwnerID:     uuid.New(),
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)

		// And: Return a connected agent.
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{{
				ID:               agentID,
				ResourceID:       resourceID,
				LifecycleState:   database.WorkspaceAgentLifecycleStateReady,
				FirstConnectedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
				LastConnectedAt:  sql.NullTime{Valid: true, Time: dbtime.Now()},
			}}, nil)

		// And: Allow db2sdk.WorkspaceAgent to complete.
		mCoordinator.EXPECT().Node(gomock.Any()).Return(nil)

		// And: WatchGit dials our fake agent server and returns
		// the stream.
		mAgentConn.EXPECT().WatchGit(gomock.Any(), gomock.Any(), chatID).
			DoAndReturn(func(ctx context.Context, _ slog.Logger, _ uuid.UUID) (*wsjson.Stream[codersdk.WorkspaceAgentGitServerMessage, codersdk.WorkspaceAgentGitClientMessage], error) {
				agentURL := strings.Replace(agentSrv.URL, "http://", "ws://", 1)
				conn, resp, err := websocket.Dial(ctx, agentURL, nil)
				if err != nil {
					return nil, err
				}
				if resp != nil && resp.Body != nil {
					defer resp.Body.Close()
				}
				// From coderd's perspective: reads server messages
				// from agent, writes client messages to agent.
				s := wsjson.NewStream[
					codersdk.WorkspaceAgentGitServerMessage,
					codersdk.WorkspaceAgentGitClientMessage,
				](conn, websocket.MessageText, websocket.MessageText, logger)
				return s, nil
			})
		// And: We mount the HTTP handler.
		r.With(httpmw.ExtractChatParam(mDB)).
			Get("/chats/{chat}/stream/git", api.watchChatGit)

		// Given: We create the HTTP server.
		coderdSrv := httptest.NewServer(r)
		defer coderdSrv.Close()

		// And: Dial the WebSocket as a client.
		wsURL := strings.Replace(coderdSrv.URL, "http://", "ws://", 1)
		clientConn, resp, err := websocket.Dial(ctx, fmt.Sprintf("%s/chats/%s/stream/git", wsURL, chatID), nil)
		require.NoError(t, err)
		if resp.Body != nil {
			defer resp.Body.Close()
		}

		// And: Create a client stream.
		clientStream := wsjson.NewStream[
			codersdk.WorkspaceAgentGitServerMessage,
			codersdk.WorkspaceAgentGitClientMessage,
		](clientConn, websocket.MessageText, websocket.MessageText, logger)
		clientCh := clientStream.Chan()

		// And: Wait for the agent stream to be ready.
		agentStream := testutil.RequireReceive(ctx, t, agentStreamReady)

		// Test agent → client: Send a server message from the
		// agent and verify the client receives it.
		err = agentStream.Send(codersdk.WorkspaceAgentGitServerMessage{
			Type:    codersdk.WorkspaceAgentGitServerMessageTypeChanges,
			Message: "test-changes",
		})
		require.NoError(t, err)

		serverMsg := testutil.RequireReceive(ctx, t, clientCh)
		require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, serverMsg.Type)
		require.Equal(t, "test-changes", serverMsg.Message)

		// Test client → agent: Send a client message and verify
		// the agent receives it.
		agentCh := agentStream.Chan()
		err = clientStream.Send(codersdk.WorkspaceAgentGitClientMessage{
			Type: codersdk.WorkspaceAgentGitClientMessageTypeRefresh,
		})
		require.NoError(t, err)

		clientMsg := testutil.RequireReceive(ctx, t, agentCh)
		require.Equal(t, codersdk.WorkspaceAgentGitClientMessageTypeRefresh, clientMsg.Type)

		// Cleanup: Close the client connection to unwind the
		// proxy chain before closing the servers.
		_ = clientStream.Close(websocket.StatusNormalClosure)
		closeAgentDone()
		coderdSrv.Close()
		agentSrv.Close()
	})

	t.Run("ClientDisconnectTearsDown", func(t *testing.T) {
		t.Parallel()

		// This test ensures that closing the client websocket
		// causes the agent stream to be closed.

		var (
			ctx    = testutil.Context(t, testutil.WaitLong)
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug).Named("coderd")

			mCtrl        = gomock.NewController(t)
			mDB          = dbmock.NewMockStore(mCtrl)
			mCoordinator = tailnettest.NewMockCoordinator(mCtrl)
			mAgentConn   = agentconnmock.NewMockAgentConn(mCtrl)

			chatID      = uuid.New()
			workspaceID = uuid.New()
			agentID     = uuid.New()
			resourceID  = uuid.New()

			r = chi.NewMux()

			fAgentProvider = fakeAgentProvider{
				agentConn: func(ctx context.Context, aID uuid.UUID) (_ workspacesdk.AgentConn, release func(), _ error) {
					return mAgentConn, func() {}, nil
				},
			}

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

		// Setup: Create a fake agent-side websocket server.
		agentDone := make(chan struct{})
		closeAgentDone := sync.OnceFunc(func() { close(agentDone) })
		t.Cleanup(closeAgentDone)
		agentStreamReady := make(chan *wsjson.Stream[
			codersdk.WorkspaceAgentGitClientMessage,
			codersdk.WorkspaceAgentGitServerMessage,
		], 1)
		agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := websocket.Accept(w, r, nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s := wsjson.NewStream[
				codersdk.WorkspaceAgentGitClientMessage,
				codersdk.WorkspaceAgentGitServerMessage,
			](ws, websocket.MessageText, websocket.MessageText, logger)
			agentStreamReady <- s
			// Keep the handler alive until test signals done.
			<-agentDone
		}))
		defer agentSrv.Close()

		// And: Return a chat with a valid workspace ID.
		mDB.EXPECT().GetChatByID(gomock.Any(), chatID).Return(database.Chat{
			ID:          chatID,
			OwnerID:     uuid.New(),
			WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		}, nil)

		// And: Return a connected agent.
		mDB.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
			Return([]database.WorkspaceAgent{{
				ID:               agentID,
				ResourceID:       resourceID,
				LifecycleState:   database.WorkspaceAgentLifecycleStateReady,
				FirstConnectedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
				LastConnectedAt:  sql.NullTime{Valid: true, Time: dbtime.Now()},
			}}, nil)

		// And: Allow db2sdk.WorkspaceAgent to complete.
		mCoordinator.EXPECT().Node(gomock.Any()).Return(nil)

		// And: WatchGit dials our fake agent server.
		mAgentConn.EXPECT().WatchGit(gomock.Any(), gomock.Any(), chatID).
			DoAndReturn(func(ctx context.Context, _ slog.Logger, _ uuid.UUID) (*wsjson.Stream[codersdk.WorkspaceAgentGitServerMessage, codersdk.WorkspaceAgentGitClientMessage], error) {
				agentURL := strings.Replace(agentSrv.URL, "http://", "ws://", 1)
				conn, resp, err := websocket.Dial(ctx, agentURL, nil)
				if err != nil {
					return nil, err
				}
				if resp != nil && resp.Body != nil {
					defer resp.Body.Close()
				}
				s := wsjson.NewStream[
					codersdk.WorkspaceAgentGitServerMessage,
					codersdk.WorkspaceAgentGitClientMessage,
				](conn, websocket.MessageText, websocket.MessageText, logger)
				return s, nil
			})
		// And: We mount the HTTP handler.
		r.With(httpmw.ExtractChatParam(mDB)).
			Get("/chats/{chat}/stream/git", api.watchChatGit)

		// Given: We create the HTTP server.
		coderdSrv := httptest.NewServer(r)
		defer coderdSrv.Close()

		// And: Dial the WebSocket as a client.
		wsURL := strings.Replace(coderdSrv.URL, "http://", "ws://", 1)
		clientConn, resp, err := websocket.Dial(ctx, fmt.Sprintf("%s/chats/%s/stream/git", wsURL, chatID), nil)
		require.NoError(t, err)
		if resp.Body != nil {
			defer resp.Body.Close()
		}

		// And: Wait for the agent stream to be ready.
		agentStream := testutil.RequireReceive(ctx, t, agentStreamReady)
		agentCh := agentStream.Chan()

		// And: Verify the proxy is working first by sending a
		// message from agent to client.
		err = agentStream.Send(codersdk.WorkspaceAgentGitServerMessage{
			Type:    codersdk.WorkspaceAgentGitServerMessageTypeChanges,
			Message: "hello",
		})
		require.NoError(t, err)

		clientDecoder := wsjson.NewDecoder[codersdk.WorkspaceAgentGitServerMessage](clientConn, websocket.MessageText, logger)
		decodeCh := clientDecoder.Chan()
		serverMsg := testutil.RequireReceive(ctx, t, decodeCh)
		require.Equal(t, "hello", serverMsg.Message)

		// When: We close the client WebSocket.
		clientConn.Close(websocket.StatusNormalClosure, "test closing connection")

		// Then: We expect agentCh to be closed, indicating
		// teardown propagated to the agent side.
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for agent channel to close")

		case _, ok := <-agentCh:
			require.False(t, ok, "agent channel is expected to be closed")
		}

		// Cleanup: Close the servers in the correct order.
		closeAgentDone()
		coderdSrv.Close()
		agentSrv.Close()
	})
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
		mDB.EXPECT().GetWorkspaceAgentAndWorkspaceByID(gomock.Any(), agentID).Return(database.GetWorkspaceAgentAndWorkspaceByIDRow{
			WorkspaceAgent: database.WorkspaceAgent{
				ID:               agentID,
				ResourceID:       resourceID,
				LifecycleState:   database.WorkspaceAgentLifecycleStateReady,
				FirstConnectedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
				LastConnectedAt:  sql.NullTime{Valid: true, Time: dbtime.Now()},
			},
			WorkspaceTable: database.WorkspaceTable{
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
		r.With(httpmw.ExtractWorkspaceAgentAndWorkspaceParam(mDB)).
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
		mDB.EXPECT().GetWorkspaceAgentAndWorkspaceByID(gomock.Any(), agentID).Return(database.GetWorkspaceAgentAndWorkspaceByIDRow{
			WorkspaceAgent: database.WorkspaceAgent{
				ID:               agentID,
				ResourceID:       resourceID,
				LifecycleState:   database.WorkspaceAgentLifecycleStateReady,
				FirstConnectedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
				LastConnectedAt:  sql.NullTime{Valid: true, Time: dbtime.Now()},
			},
			WorkspaceTable: database.WorkspaceTable{
				ID: workspaceID,
			},
		}, nil)

		// And: Allow `db2dsk.WorkspaceAgent` to complete.
		mCoordinator.EXPECT().Node(gomock.Any()).Return(nil)

		// And: Allow `WatchContainers` to be called, returing our `containersCh` channel.
		mAgentConn.EXPECT().WatchContainers(gomock.Any(), gomock.Any()).
			Return(containersCh, io.NopCloser(&bytes.Buffer{}), nil)

		// And: We mount the HTTP Handler
		r.With(httpmw.ExtractWorkspaceAgentAndWorkspaceParam(mDB)).
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
