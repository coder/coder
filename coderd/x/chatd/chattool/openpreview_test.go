package chattool_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenPreview(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()

	for _, tt := range []struct {
		name        string
		input       string
		path        string
		agentPath   string
		wantPort    int
		wantTitle   string
		wantPath    string
		wantURLPath string
		openErr     error
		wantError   string
	}{
		{
			name:      "ExistingServer",
			input:     `{"title":"  Signup form  ","port":3000}`,
			wantPort:  3000,
			wantTitle: "Signup form",
		},
		{
			name:        "ExistingServerAtPath",
			input:       `{"title":"Editor","port":6006,"url_path":" /?folder=/home/coder/coder "}`,
			wantPort:    6006,
			wantTitle:   "Editor",
			wantURLPath: "/?folder=/home/coder/coder",
		},
		{
			name:      "RootURLPath",
			input:     `{"title":"Editor","port":6006,"url_path":"/"}`,
			wantPort:  6006,
			wantTitle: "Editor",
		},
		{
			name:      "URLPathWithoutPort",
			input:     `{"title":"Docs","path":"/home/coder/output","url_path":"/docs"}`,
			wantError: "url_path can only be used with port",
		},
		{
			// Browsers treat a leading // or /\ as another host.
			name:      "URLPathOtherHost",
			input:     `{"title":"Editor","port":6006,"url_path":"/\\evil.example"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathDoubleSlash",
			input:     `{"title":"Editor","port":6006,"url_path":"//evil.example"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathRelative",
			input:     `{"title":"Editor","port":6006,"url_path":"docs"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathFragment",
			input:     `{"title":"Editor","port":6006,"url_path":"/a#b"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathControlCharacter",
			input:     `{"title":"Editor","port":6006,"url_path":"/a\u0007b"}`,
			wantError: "url_path must start with a single /",
		},
		{
			name:      "URLPathTooLong",
			input:     `{"title":"Editor","port":6006,"url_path":"/` + strings.Repeat("a", 2048) + `"}`,
			wantError: "at most 2048 characters",
		},
		{
			name:      "PublishDirectory",
			input:     `{"title":"Docs site","path":" /home/coder/output/ "}`,
			path:      "/home/coder/output/",
			agentPath: "/srv/output",
			wantTitle: "Docs site",
			// The previews endpoint lists the path the agent resolved, not the
			// input.
			wantPath: "/srv/output",
		},
		{
			name:  "AgentError",
			input: `{"title":"Docs site","path":"/home/coder/missing"}`,
			path:  "/home/coder/missing",
			openErr: func() error {
				err := codersdk.NewTestError(http.StatusBadRequest, "POST", "http://[fd7a::1]:4/api/v0/previews")
				err.Message = "Could not open preview."
				err.Detail = "preview path does not exist or cannot be read"
				return err
			}(),
			wantError: "Could not open preview.: preview path does not exist or cannot be read",
		},
		{
			name:      "OutdatedAgent",
			input:     `{"title":"Docs site","path":"/home/coder/output"}`,
			path:      "/home/coder/output",
			openErr:   codersdk.NewTestError(http.StatusNotFound, "POST", "http://[fd7a::1]:4/api/v0/previews"),
			wantError: "the workspace agent is outdated",
		},
		{
			name:      "BlankTitle",
			input:     `{"title":" \t ","port":3000}`,
			wantError: "title is required",
		},
		{
			name:      "LongTitle",
			input:     `{"title":"` + strings.Repeat("é", 81) + `","port":3000}`,
			wantError: "at most 80 characters",
		},
		{
			name:      "BothTargets",
			input:     `{"title":"App","port":3000,"path":"/home/coder/output"}`,
			wantError: "exactly one",
		},
		{
			name:      "ShortPortHostname",
			input:     `{"title":"App","port":999}`,
			wantError: "between 1000 and 65535",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			if tt.path != "" {
				conn.EXPECT().OpenPreview(gomock.Any(), workspacesdk.OpenPreviewRequest{Path: tt.path}).
					Return(workspacesdk.OpenPreviewResponse{ID: "00000001", Path: tt.agentPath}, tt.openErr)
			}
			tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
				GetPreviewWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
					return chattool.PreviewWorkspace{
						Conn:        conn,
						WorkspaceID: workspaceID,
						AgentName:   "main",
					}, nil
				},
			})

			ctx := testutil.Context(t, testutil.WaitShort)
			response, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    uuid.NewString(),
				Name:  "open_preview",
				Input: tt.input,
			})
			require.NoError(t, err)
			if tt.wantError != "" {
				require.True(t, response.IsError)
				require.Contains(t, response.Content, tt.wantError)
				require.NotContains(t, response.Content, "http://")
				return
			}
			require.False(t, response.IsError)

			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
			want := map[string]any{
				"ok":           true,
				"title":        tt.wantTitle,
				"workspace_id": workspaceID.String(),
				"agent_name":   "main",
			}
			if tt.wantPath != "" {
				want["path"] = tt.wantPath
				want["preview_id"] = "00000001"
			} else {
				want["port"] = float64(tt.wantPort)
			}
			if tt.wantURLPath != "" {
				want["url_path"] = tt.wantURLPath
			}
			require.Equal(t, want, result)
		})
	}
}

func TestOpenPreviewApp(t *testing.T) {
	t.Parallel()

	db, owner, build := setupPreviewWorkspace(t,
		&sdkproto.App{Slug: "code-server", Url: "http://localhost:8080", Subdomain: true},
		&sdkproto.App{Slug: "path-app", Url: "http://localhost:8082"},
		&sdkproto.App{Slug: "hidden", Url: "http://localhost:8081", Subdomain: true, Hidden: true},
		&sdkproto.App{Slug: "docs", Url: "https://coder.com", External: true},
		&sdkproto.App{Slug: "shell", Command: "bash"},
	)
	tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
		GetPreviewWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
			return chattool.PreviewWorkspace{
				WorkspaceID: build.Workspace.ID,
				AgentID:     build.Agents[0].ID,
				AgentName:   build.Agents[0].Name,
			}, nil
		},
		DB:      db,
		OwnerID: owner.ID,
	})

	for _, tt := range []struct {
		name      string
		app       string
		wantError string
	}{
		{name: "Openable", app: "code-server"},
		{name: "Hidden", app: "hidden", wantError: `app "hidden" cannot be opened: open_preview only opens subdomain apps that are not hidden, external, or command apps; apps it can open: code-server`},
		{name: "PathBased", app: "path-app", wantError: `app "path-app" cannot be opened: open_preview only opens subdomain apps that are not hidden, external, or command apps; apps it can open: code-server`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			response, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    uuid.NewString(),
				Name:  "open_preview",
				Input: `{"title":"Editor","app":"` + tt.app + `"}`,
			})
			require.NoError(t, err)
			if tt.wantError != "" {
				require.True(t, response.IsError)
				require.Contains(t, response.Content, tt.wantError)
				return
			}
			require.False(t, response.IsError, response.Content)

			var result chattool.OpenPreviewResult
			require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
			require.Equal(t, chattool.OpenPreviewResult{
				OK:          true,
				Title:       "Editor",
				WorkspaceID: build.Workspace.ID,
				AgentName:   build.Agents[0].Name,
				AppSlug:     "code-server",
			}, result)
		})
	}
}

func setupPreviewWorkspace(t *testing.T, apps ...*sdkproto.App) (database.Store, database.User, dbfake.WorkspaceResponse) {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
	})
	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        owner.ID,
		OrganizationID: org.ID,
	}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
		agents[0].Apps = apps
		return agents
	}).Do()

	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, nil),
		testAccessControlStorePointer(),
	)
	return authzDB, owner, build
}
