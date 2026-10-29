package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenPreview(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		input        string
		path         string
		daemonID     string
		daemonPath   string
		daemonError  error
		resolveError error
		wantPort     int
		wantTitle    string
		wantPath     string
		wantURLPath  string
		wantError    string
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
			name:       "PublishDirectory",
			input:      `{"title":"Docs site","path":" /home/coder/output "}`,
			path:       "/home/coder/output",
			daemonID:   "fedcba9876543210",
			daemonPath: "/srv/output",
			wantTitle:  "Docs site",
			// The catalog shows the path the daemon resolved, not the input.
			wantPath: "/srv/output",
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
			name:      "MultilineTitle",
			input:     `{"title":"Docs\nsite","port":3000}`,
			wantError: "control or invisible",
		},
		{
			name:      "BidiTitle",
			input:     `{"title":"Docs\u202esite","port":3000}`,
			wantError: "control or invisible",
		},
		{
			name:      "NeitherTarget",
			input:     `{"title":"App"}`,
			wantError: "exactly one",
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
		{
			name:      "PortOverflow",
			input:     `{"title":"App","port":65536}`,
			wantError: "between 1000 and 65535",
		},
		{
			name:         "WorkspaceUnavailable",
			input:        `{"title":"App","port":3000}`,
			resolveError: xerrors.New("no workspace is attached"),
			wantError:    "no workspace is attached",
		},
		{
			name:        "PublishError",
			input:       `{"title":"Docs","path":"/home/coder/output"}`,
			path:        "/home/coder/output",
			daemonError: xerrors.New("directory requires index.html"),
			wantError:   "directory requires index.html",
		},
		{
			name:      "InvalidDaemonID",
			input:     `{"title":"Docs","path":"/home/coder/output"}`,
			path:      "/home/coder/output",
			daemonID:  "0123456789ABCDEF",
			wantError: "invalid preview ID",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
			workspaceID := uuid.New()
			if tt.path != "" {
				conn.EXPECT().OpenPreview(gomock.Any(), workspacesdk.OpenPreviewRequest{Path: tt.path}).
					Return(workspacesdk.OpenPreviewResponse{ID: tt.daemonID, Path: tt.daemonPath}, tt.daemonError)
			}
			tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
				GetWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
					return chattool.PreviewWorkspace{
						Conn:        conn,
						WorkspaceID: workspaceID,
						AgentName:   "main",
					}, tt.resolveError
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
				want["preview_id"] = tt.daemonID
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

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	build := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
	}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
		agents[0].Apps = []*sdkproto.App{
			{Slug: "code-server", Url: "http://localhost:8080"},
			{Slug: "hidden", Url: "http://localhost:8081", Hidden: true},
			{Slug: "docs", Url: "https://coder.com", External: true},
			{Slug: "shell", Command: "bash"},
		}
		return agents
	}).Do()

	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, nil),
		testAccessControlStorePointer(),
	)
	tool := chattool.OpenPreview(chattool.OpenPreviewOptions{
		GetWorkspace: func(context.Context) (chattool.PreviewWorkspace, error) {
			return chattool.PreviewWorkspace{
				WorkspaceID: build.Workspace.ID,
				AgentID:     build.Agents[0].ID,
				AgentName:   build.Agents[0].Name,
			}, nil
		},
		DB:      authzDB,
		OwnerID: user.ID,
	})

	for _, tt := range []struct {
		name      string
		app       string
		wantError string
	}{
		{name: "Embeddable", app: "code-server"},
		// Only apps the right panel can embed are listed as alternatives.
		{name: "Hidden", app: "hidden", wantError: `app "hidden" not found; apps that can be shown: code-server`},
		{name: "Command", app: "shell", wantError: `apps that can be shown: code-server`},
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
