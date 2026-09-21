package coderd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

func TestACPProxyWorkspaceIsolation(t *testing.T) {
	t.Parallel()
	for _, sameWorkspace := range []bool{true, false} {
		t.Run(fmt.Sprint(sameWorkspace), func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			parent, org, workspaceID, agentID, sessionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
			chat := database.Chat{ID: parent, OrganizationID: org, WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true}}
			workspace := database.Workspace{ID: workspaceID, OrganizationID: org}
			db.EXPECT().GetChatByID(gomock.Any(), parent).Return(chat, nil)
			db.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(workspace, nil)
			target := workspace
			if !sameWorkspace {
				target.ID = uuid.New()
			}
			db.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agentID).Return(target, nil)
			logger := slogtest.Make(t, nil)
			api := &API{Options: &Options{Database: db, Logger: logger}, HTTPAuth: &HTTPAuthorizer{Authorizer: &mockAuthorizer{}, Logger: logger}}
			if sameWorkspace {
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, workspacesdk.ACPPath(org, parent)+sessionID.String()+"/messages", r.URL.Path)
					require.Empty(t, r.Header.Get("Cookie"))
					require.Empty(t, r.Header.Get("Authorization"))
					_, _ = w.Write([]byte(`{"status":"running"}`))
				}))
				defer backend.Close()
				conn := agentconnmock.NewMockAgentConn(ctrl)
				conn.EXPECT().DialContext(gomock.Any(), "tcp", gomock.Any()).DoAndReturn(func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(backend.URL, "http://"))
				})
				api.agentProvider = fakeAgentProvider{agentConn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) { return conn, func() {}, nil }}
			}
			router := chi.NewRouter()
			router.With(injectSystemActor, httpmw.ExtractChatParam(db)).HandleFunc("/chats/{chat}/acp/agents/{workspaceagent}/sessions/*", api.proxyACP)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, fmt.Sprintf("/chats/%s/acp/agents/%s/sessions/%s/messages", parent, agentID, sessionID), strings.NewReader(`{"message":"hello"}`))
			req.Header.Set("Cookie", "secret")
			req.Header.Set("Authorization", "secret")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if sameWorkspace {
				require.Equal(t, 200, response.Code)
				require.JSONEq(t, `{"status":"running"}`, response.Body.String())
			} else {
				require.Equal(t, 404, response.Code)
			}
		})
	}
}

func TestACPProxyRejectsCrossOriginWebSocket(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	parent := uuid.New()
	db.EXPECT().GetChatByID(gomock.Any(), parent).Return(database.Chat{ID: parent}, nil)
	api := &API{Options: &Options{Database: db}}
	router := chi.NewRouter()
	router.With(httpmw.ExtractChatParam(db)).HandleFunc("/chats/{chat}/acp/agents/{workspaceagent}/sessions/*", api.proxyACP)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://coder.test/chats/%s/acp/agents/%s/sessions/%s/stream", parent, uuid.New(), uuid.New()), nil)
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Origin", "https://unrelated.test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
}
