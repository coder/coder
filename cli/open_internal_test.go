package cli

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
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

func Test_isSessionTokenDestinationTrusted(t *testing.T) {
	t.Parallel()

	const (
		origin = "https://coder.example.com"
		vs     = "vscode://coder.coder-remote/open?"
		gw     = "jetbrains-gateway://connect#type=coder&"
		u      = "url=" + origin
		tok    = "&token=" + externalSessionTokenPlaceholder
		evil   = "https://attacker.example"
	)
	deployment, err := url.Parse(origin)
	require.NoError(t, err)

	cases := map[string]bool{
		"cursor://coder.coder-remote/open?" + u + "/" + tok:                          true,
		"jetbrains://gateway/coder?&workspace=b&" + u + tok + "&ide_product_code=GO": true,
		gw + "workspace=b&" + u + tok:                                                true,
		"VSCode://Coder.Coder-Remote/open?owner=a&" + u + tok:                        true,

		"https://attacker.example/?t=$SESSION_TOKEN":              false,
		"http://attacker.example/?t=$SESSION_TOKEN":               false,
		"myapp://open?" + u + tok:                                 false,
		vs + "url=" + evil + tok:                                  false,
		vs + "url=https://coder.example.com:1" + tok:              false,
		vs + "url=http://coder.example.com" + tok:                 false,
		vs + "url=https%3A%2F%2Fattacker.example" + tok:           false,
		vs + "token=$SESSION_TOKEN":                               false,
		vs + "url=not-a-url" + tok:                                false,
		vs + u + "&url=" + evil + tok:                             false,
		"jetbrains-gateway://connect?url=" + evil + "#" + u + tok: false,
		"jetbrains://gateway/coder?" + u + "&token=x&ide_download_link=" + evil + "/$SESSION_TOKEN": false,
		vs + u + tok + "&t=$SESSION_TOKEN":                                               false,
		vs + u + "#token=$SESSION_TOKEN":                                                 false,
		vs + u + tok + "#url=" + evil:                                                    false,
		"vscode://attacker.ext/open?" + u + tok:                                          false,
		vs + u + "/@victim/ws.main/apps/evil/" + tok:                                     false,
		vs + "url=https://attacker.example%252Fx@coder.example.com/" + tok:               false,
		vs + "owner=x%26url%3Dhttps%253A%252F%252Fattacker.example&" + u + tok:           false,
		vs + "x=%252526url%25253Dhttps%25253A%25252F%25252Fattacker.example&" + u + tok:  false,
		vs + "x=%FF&" + u + tok:                                                          false,
		"vscode:coder.coder-remote/open?" + u + tok:                                      false,
		"vscode://x@coder.coder-remote/open?" + u + tok:                                  false,
		"vscode://coder.coder-remote/open/../open?" + u + tok:                            false,
		"vscode://coder.coder-remote/open#" + u + tok:                                    false,
		"jetbrains-gateway://connect?" + u + tok:                                         false,
		gw + u + "&token=%24SESSION_TOKEN&ide_download_link=" + evil + "/$SESSION_TOKEN": false,
		gw + "%75rl=" + origin + tok:                                                     false,
		"jetbrains-gateway://connect#type=ssh&host=attacker.example&" + u + tok:          false,
		gw + u + tok + "&type=ssh":                                                       false,
		vs + "x=1#&" + u + tok:                                                           false,
		"\u212Airo://coder.coder-remote/open?" + u + tok:                                 false,
		gw + u + "&ur\tl=" + evil + tok:                                                  false,
		gw + u + "&ur\nl=" + evil + tok:                                                  false,
		gw + u + "&ur\rl=" + evil + tok:                                                  false,
		vs + "url=https://attacker.example@coder.example.com" + tok:                      false,
		vs + "url=https://coder.example.com%20" + tok:                                    false,
	}
	for _, prefix := range sessionTokenHandlerPrefixes {
		cases[prefix+u+tok] = true
	}
	for link, want := range cases {
		assert.Equal(t, want, isSessionTokenDestinationTrusted(link, deployment), "%q", link)
	}
}
