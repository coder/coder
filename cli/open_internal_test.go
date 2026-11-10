package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_resolveAgentAbsPath(t *testing.T) {
	t.Parallel()

	type args struct {
		workingDirectory string
		relOrAbsPath     string
		agentOS          string
		local            bool
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"ok no args", args{}, "", false},
		{"ok only working directory", args{workingDirectory: "/workdir"}, "/workdir", false},
		{"ok with working directory and rel path", args{workingDirectory: "/workdir", relOrAbsPath: "my/path"}, "/workdir/my/path", false},
		{"ok with working directory and abs path", args{workingDirectory: "/workdir", relOrAbsPath: "/my/path"}, "/my/path", false},
		{"ok with no working directory and abs path", args{relOrAbsPath: "/my/path"}, "/my/path", false},

		{"fail tilde", args{relOrAbsPath: "~"}, "", true},
		{"fail tilde with working directory", args{workingDirectory: "/workdir", relOrAbsPath: "~"}, "", true},
		{"fail tilde path", args{relOrAbsPath: "~/workdir"}, "", true},
		{"fail tilde path with working directory", args{workingDirectory: "/workdir", relOrAbsPath: "~/workdir"}, "", true},
		{"fail relative dot with no working directory", args{relOrAbsPath: "."}, "", true},
		{"fail relative with no working directory", args{relOrAbsPath: "workdir"}, "", true},

		{"ok with working directory and rel path on windows", args{workingDirectory: "C:\\workdir", relOrAbsPath: "my\\path", agentOS: "windows"}, "C:\\workdir\\my\\path", false},
		{"ok with working directory and abs path on windows", args{workingDirectory: "C:\\workdir", relOrAbsPath: "C:\\my\\path", agentOS: "windows"}, "C:\\my\\path", false},
		{"ok with no working directory and abs path on windows", args{relOrAbsPath: "C:\\my\\path", agentOS: "windows"}, "C:\\my\\path", false},
		{"ok abs unix path on windows", args{workingDirectory: "C:\\workdir", relOrAbsPath: "/my/path", agentOS: "windows"}, "\\my\\path", false},
		{"ok rel unix path on windows", args{workingDirectory: "C:\\workdir", relOrAbsPath: "my/path", agentOS: "windows"}, "C:\\workdir\\my\\path", false},

		{"fail with no working directory and rel path on windows", args{relOrAbsPath: "my\\path", agentOS: "windows"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveAgentAbsPath(tt.args.workingDirectory, tt.args.relOrAbsPath, tt.args.agentOS, tt.args.local)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveAgentAbsPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveAgentAbsPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_buildAppLinkURL(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		// function arguments
		baseURL           string
		workspace         codersdk.Workspace
		agent             codersdk.WorkspaceAgent
		app               codersdk.WorkspaceApp
		appsHost          string
		preferredPathBase string
		// expected results
		expectedLink string
	}{
		{
			name:    "external url",
			baseURL: "https://coder.tld",
			app: codersdk.WorkspaceApp{
				External: true,
				URL:      "https://external-url.tld",
			},
			expectedLink: "https://external-url.tld",
		},
		{
			name:    "without subdomain",
			baseURL: "https://coder.tld",
			workspace: codersdk.Workspace{
				Name:      "Test-Workspace",
				OwnerName: "username",
			},
			agent: codersdk.WorkspaceAgent{
				Name: "a-workspace-agent",
			},
			app: codersdk.WorkspaceApp{
				Slug:      "app-slug",
				Subdomain: false,
			},
			preferredPathBase: "/path-base",
			expectedLink:      "https://coder.tld/path-base/@username/Test-Workspace.a-workspace-agent/apps/app-slug/",
		},
		{
			name:    "with command",
			baseURL: "https://coder.tld",
			workspace: codersdk.Workspace{
				Name:      "Test-Workspace",
				OwnerName: "username",
			},
			agent: codersdk.WorkspaceAgent{
				Name: "a-workspace-agent",
			},
			app: codersdk.WorkspaceApp{
				Slug:    "my-terminal",
				Command: "ls -la",
			},
			expectedLink: "https://coder.tld/@username/Test-Workspace.a-workspace-agent/terminal?app=my-terminal",
		},
		{
			name:    "with subdomain",
			baseURL: "ftps://coder.tld",
			workspace: codersdk.Workspace{
				Name:      "Test-Workspace",
				OwnerName: "username",
			},
			agent: codersdk.WorkspaceAgent{
				Name: "a-workspace-agent",
			},
			app: codersdk.WorkspaceApp{
				Subdomain:     true,
				SubdomainName: "hellocoder",
			},
			preferredPathBase: "/path-base",
			appsHost:          "*.apps-host.tld",
			expectedLink:      "ftps://hellocoder.apps-host.tld/",
		},
		{
			name:    "with subdomain, but not apps host",
			baseURL: "https://coder.tld",
			workspace: codersdk.Workspace{
				Name:      "Test-Workspace",
				OwnerName: "username",
			},
			agent: codersdk.WorkspaceAgent{
				Name: "a-workspace-agent",
			},
			app: codersdk.WorkspaceApp{
				Slug:          "app-slug",
				Subdomain:     true,
				SubdomainName: "It really doesn't matter what this is without AppsHost.",
			},
			preferredPathBase: "/path-base",
			expectedLink:      "https://coder.tld/path-base/@username/Test-Workspace.a-workspace-agent/apps/app-slug/",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseURL, err := url.Parse(tt.baseURL)
			require.NoError(t, err)
			actual := buildAppLinkURL(baseURL, tt.workspace, tt.agent, tt.app, tt.appsHost, tt.preferredPathBase)
			assert.Equal(t, tt.expectedLink, actual)
		})
	}
}

func Test_sessionTokenDestinationTrusted(t *testing.T) {
	t.Parallel()

	// The dashboard's apps.test.ts runs the same cases against
	// isSessionTokenDestinationTrusted, so both clients trust the same links.
	raw, err := os.ReadFile(filepath.Join("..", "site", "src", "modules", "apps", "sessionTokenDestinationCases.json"))
	require.NoError(t, err)
	var fixture struct {
		Origin string `json:"origin"`
		Cases  []struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Trusted bool   `json:"trusted"`
			Origin  string `json:"origin"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.NotEmpty(t, fixture.Cases)

	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			origin := fixture.Origin
			if tc.Origin != "" {
				origin = tc.Origin
			}
			assert.Equal(t, tc.Trusted, sessionTokenDestinationTrusted(tc.URL, []string{origin}))
		})
	}
}

func Test_urlOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"https://Coder.Example.com", "https://coder.example.com"},
		{"https://coder.example.com:443/path", "https://coder.example.com"},
		{"http://coder.example.com:80", "http://coder.example.com"},
		{"http://coder.example.com:3000", "http://coder.example.com:3000"},
		{"https://coder.example.com:80", "https://coder.example.com:80"},
		{"http://[::1]:3000", "http://[::1]:3000"},
		{"http://[::1]", "http://[::1]"},
		{"https://COD\u0130NG.com", "https://cod\u0130ng.com"},
		{"https://coder.example.com:0443", "https://coder.example.com"},
		{"http://coder.example.com:0080", "http://coder.example.com"},
		{"http://coder.example.com:0001", "http://coder.example.com:1"},
		{"https://coder.example.com:65536", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, urlOrigin(u))
		})
	}
}

func Test_sessionTokenOrigins(t *testing.T) {
	t.Parallel()

	t.Run("IncludesDashboardURL", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(codersdk.BuildInfoResponse{
				DashboardURL: "https://Access.example.com:443/",
			})
		}))
		t.Cleanup(srv.Close)
		u, err := url.Parse(srv.URL)
		require.NoError(t, err)

		origins := sessionTokenOrigins(testutil.Context(t, testutil.WaitShort), codersdk.New(u))
		assert.Equal(t, []string{urlOrigin(u), "https://access.example.com"}, origins)
	})

	// Without build info, only the URL the CLI is logged in to is trusted.
	t.Run("BuildInfoError", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		u, err := url.Parse(srv.URL)
		require.NoError(t, err)

		origins := sessionTokenOrigins(testutil.Context(t, testutil.WaitShort), codersdk.New(u))
		assert.Equal(t, []string{urlOrigin(u)}, origins)
	})
}
