package coderd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/quartz"
)

func TestChatMCPAppResource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		disabled    bool
		server      string
		uri         string
		noWorkspace bool
		noAgent     bool
		mime        string
		text        string
		blob        string
		readError   error
		status      int
	}{
		{name: "Disabled", disabled: true, status: http.StatusNotFound},
		{name: "BadScheme", server: "app", uri: "https://example.com", status: http.StatusBadRequest},
		{name: "MissingServer", uri: "ui://app/view", status: http.StatusBadRequest},
		{name: "NoWorkspace", server: "app", uri: "ui://app/view", noWorkspace: true, status: http.StatusNotFound},
		{name: "UnknownServer", server: "unknown", uri: "ui://app/view", status: http.StatusNotFound},
		{name: "UndeclaredURI", server: "app", uri: "ui://app/other", status: http.StatusNotFound},
		{name: "NoAgent", server: "app", uri: "ui://app/view", noAgent: true, status: http.StatusNotFound},
		{name: "WrongMIME", server: "app", uri: "ui://app/view", mime: "text/plain", status: http.StatusUnsupportedMediaType},
		{name: "MissingProfile", server: "app", uri: "ui://app/view", mime: "text/html", status: http.StatusUnsupportedMediaType},
		{name: "Success", server: "app", uri: "ui://app/view", mime: "text/html;profile=mcp-app", text: "<p>Hello</p>", status: http.StatusOK},
		{name: "CaseAndWhitespaceMIME", server: "app", uri: "ui://app/view", mime: "Text/HTML; PROFILE = MCP-APP", text: "<p>Hello</p>", status: http.StatusOK},
		{name: "LegacyMIME", server: "app", uri: "ui://app/view", mime: "TEXT/HTML+SKYBRIDGE", text: "<p>Hello</p>", status: http.StatusOK},
		{name: "Blob", server: "app", uri: "ui://app/view", mime: "text/html;profile=mcp-app", blob: base64.StdEncoding.EncodeToString([]byte("<p>Hello</p>")), status: http.StatusOK},
		{name: "InvalidBlob", server: "app", uri: "ui://app/view", mime: "text/html;profile=mcp-app", blob: "!!!", status: http.StatusBadGateway},
		{name: "AtLimit", server: "app", uri: "ui://app/view", mime: "text/html;profile=mcp-app", text: strings.Repeat("x", maxMCPAppHTMLBytes), status: http.StatusOK},
		{name: "LargeHTML", server: "app", uri: "ui://app/view", mime: "text/html;profile=mcp-app", text: strings.Repeat("x", maxMCPAppHTMLBytes+1), status: http.StatusRequestEntityTooLarge},
		{name: "LargeBlob", server: "app", uri: "ui://app/view", mime: "text/html;profile=mcp-app", blob: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", maxMCPAppHTMLBytes+1))), status: http.StatusRequestEntityTooLarge},
		{name: "ResponseTooLarge", server: "app", uri: "ui://app/view", readError: xerrors.Errorf("agent: %w", workspacesdk.ErrMCPResourceTooLarge), status: http.StatusRequestEntityTooLarge},
		{name: "BlobAtLimit", server: "app", uri: "ui://app/view", mime: "text/html;profile=mcp-app", blob: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", maxMCPAppHTMLBytes))), status: http.StatusOK},
		{name: "OverEncodedLimit", server: "app", uri: "ui://app/view", mime: "text/html;profile=mcp-app", blob: strings.Repeat("!", base64.StdEncoding.EncodedLen(maxMCPAppHTMLBytes)+1), status: http.StatusRequestEntityTooLarge},
		{name: "ReadError", server: "app", uri: "ui://app/view", readError: xerrors.New("resource read failed"), status: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			chat := database.Chat{ID: uuid.New(), WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: !tc.noWorkspace}, AgentID: uuid.NullUUID{UUID: uuid.New(), Valid: !tc.noAgent}}
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)
			experiments := codersdk.Experiments{codersdk.ExperimentChatMCPApps}
			if tc.disabled {
				experiments = nil
			}
			daemon := chatd.New(pubsub.NewInMemory(), chatd.Config{Database: db, Logger: slogtest.Make(t, nil), Experiments: experiments, Clock: quartz.NewMock(t)})
			t.Cleanup(func() { require.NoError(t, daemon.Close()) })
			api := &API{Options: &Options{Database: db}, Experiments: experiments, chatDaemon: daemon}
			if !tc.disabled && tc.server != "" && strings.HasPrefix(tc.uri, "ui://") && !tc.noWorkspace && !tc.noAgent {
				db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chat.ID).Return([]database.ChatContextResource{{
					BodyKind: database.WorkspaceAgentContextBodyKindMcpServer,
					Status:   database.WorkspaceAgentContextResourceStatusOk,
					Body:     json.RawMessage(`{"serverName":"app","tools":[{"name":"render","meta":{"ui":{"resourceUri":"ui://app/view"}}}]}`),
				}}, nil)
				if tc.server == "app" && tc.uri == "ui://app/view" {
					conn := agentconnmock.NewMockAgentConn(ctrl)
					api.agentProvider = fakeAgentProvider{agentConn: func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
						require.Equal(t, chat.AgentID.UUID, id)
						return conn, func() {}, nil
					}}
					conn.EXPECT().ReadMCPResource(gomock.Any(), workspacesdk.ReadMCPResourceRequest{ServerName: "app", URI: "ui://app/view"}).Return(workspacesdk.ReadMCPResourceResponse{
						URI: "ui://app/view", MimeType: tc.mime, Text: tc.text, Blob: tc.blob,
						Meta: json.RawMessage(`{"ui":{"csp":{"resourceDomains":["https://cdn.example.com"],"connectDomains":["https://api.example.com"]}}}`),
					}, tc.readError)
				}
			}
			rtr := chi.NewRouter()
			rtr.Route("/api/v2", func(r chi.Router) {
				api.registerChatAPIRoutes(r, func(h http.Handler) http.Handler { return h }, chatAPIPrefixV2)
			})
			path := "/api/v2/chats/" + chat.ID.String() + "/mcp-apps/resource?" + url.Values{"server": {tc.server}, "uri": {tc.uri}}.Encode()
			rec := httptest.NewRecorder()
			rtr.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			if tc.status == http.StatusOK {
				expectedHTML := tc.text
				if expectedHTML == "" {
					decoded, err := base64.StdEncoding.DecodeString(tc.blob)
					require.NoError(t, err)
					expectedHTML = string(decoded)
				}
				require.Equal(t, expectedHTML, rec.Body.String())
				require.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
				require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
				require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
				require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
				require.Len(t, rec.Header().Values("Content-Security-Policy"), 1)
				policy := rec.Header().Get("Content-Security-Policy")
				require.Contains(t, policy, "sandbox allow-scripts;")
				require.NotContains(t, policy, "allow-same-origin")
				require.Contains(t, policy, "connect-src https://api.example.com;")
			}
		})
	}
}

func TestMCPAppCSP(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		meta      string
		resources string
		connect   string
	}{
		{name: "Missing", connect: "'none'"},
		{name: "InvalidJSON", meta: "{", connect: "'none'"},
		{name: "Origins", meta: `{"ui":{"csp":{"resourceDomains":["https://cdn.example.com/","https://images.example.com:8443"],"connectDomains":["https://api.example.com"]}}}`, resources: "https://cdn.example.com https://images.example.com:8443", connect: "https://api.example.com"},
		{name: "RejectNonOrigins", meta: `{"ui":{"csp":{"resourceDomains":["http://example.com","https://*.example.com","https://example.com/path","https://user@example.com","https://example.com?q=x","https://example.com#x","https://example.com;script-src","https://example.com 'unsafe-eval'","data:","//example.com"],"connectDomains":["wss://example.com"]}}}`, connect: "'none'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy := mcpAppCSP(json.RawMessage(tc.meta))
			require.Equal(t, "sandbox allow-scripts; default-src 'none'; "+
				"script-src 'unsafe-inline' "+tc.resources+"; "+
				"style-src 'unsafe-inline' "+tc.resources+"; "+
				"img-src data: blob: "+tc.resources+"; "+
				"font-src data: "+tc.resources+"; "+
				"media-src data: blob: "+tc.resources+"; "+
				"connect-src "+tc.connect+"; "+
				"frame-ancestors 'self'; base-uri 'none'; form-action 'none'; frame-src 'none'; object-src 'none'", policy)
		})
	}
}

func TestMCPAppOriginsLimits(t *testing.T) {
	t.Parallel()
	longest := "https://" + strings.Repeat("a", 245)
	for _, tc := range []struct {
		name          string
		domains, want []string
	}{
		{name: "AtLengthLimit", domains: []string{longest}, want: []string{longest}},
		{name: "OverLengthLimit", domains: []string{longest + "a", "https://example.com"}, want: []string{"https://example.com"}},
		{name: "AtCountLimit", domains: strings.Fields(strings.Repeat("https://example.com ", 16)), want: strings.Fields(strings.Repeat("https://example.com ", 16))},
		{name: "OverCountLimit", domains: strings.Fields(strings.Repeat("https://example.com ", 17)), want: strings.Fields(strings.Repeat("https://example.com ", 16))},
		{name: "SkipInvalid", domains: append(strings.Fields(strings.Repeat("invalid ", 16)), "https://example.com"), want: []string{"https://example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, mcpAppOrigins(tc.domains))
		})
	}
}
