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

func Test_sessionTokenDestinationTrusted(t *testing.T) {
	t.Parallel()

	// Mirrors the isSessionTokenDestinationTrusted cases in
	// site/src/modules/apps/apps.test.ts.
	const (
		origin = "https://coder.example.com"
		token  = externalSessionTokenPlaceholder
	)
	origins := []string{origin}

	trusted := []struct {
		name string
		url  string
	}{
		{"VSCode", "vscode://coder.coder-remote/open?owner=a&workspace=b&url=" + origin + "&token=" + token},
		{"TrailingSlash", "cursor://coder.coder-remote/open?url=" + origin + "/&token=" + token},
		{"JetBrainsToolbox", "jetbrains://gateway/coder?&workspace=b&url=" + origin + "&token=" + token + "&ide_product_code=GO"},
		{"JetBrainsGateway", "jetbrains-gateway://connect#type=coder&workspace=b&url=" + origin + "&token=" + token},
		{"DevContainer", "cursor://coder.coder-remote/openDevContainer?url=" + origin + "&token=" + token},
		{"JetBrainsToolboxDocumentedPath", "jetbrains://gateway/com.coder.toolbox?url=" + origin + "&token=" + token},
		{"UppercaseSchemeAndAuthority", "VSCode://Coder.Coder-Remote/open?url=" + origin + "&token=" + token},
		{"UppercaseURLHost", "vscode://coder.coder-remote/open?url=https://CODER.example.com&token=" + token},
		{"DefaultPort", "vscode://coder.coder-remote/open?url=https://coder.example.com:443&token=" + token},
	}
	for _, tc := range trusted {
		t.Run("Allows/"+tc.name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, sessionTokenDestinationTrusted(tc.url, origins))
		})
	}

	untrusted := []struct {
		name string
		url  string
	}{
		{"AnotherOrigin", "vscode://coder.coder-remote/open?url=https://attacker.example&token=" + token},
		{"AnotherPort", "vscode://coder.coder-remote/open?url=https://coder.example.com:1&token=" + token},
		{"AnotherScheme", "vscode://coder.coder-remote/open?url=http://coder.example.com&token=" + token},
		{"EncodedOtherOrigin", "vscode://coder.coder-remote/open?url=https%3A%2F%2Fattacker.example&token=" + token},
		{"NoURL", "vscode://coder.coder-remote/open?token=" + token},
		{"UnparsableURL", "vscode://coder.coder-remote/open?url=not-a-url&token=" + token},
		{"DuplicateURL", "vscode://coder.coder-remote/open?url=" + origin + "&url=https://attacker.example&token=" + token},
		{"URLInQueryAndFragment", "jetbrains-gateway://connect?url=https://attacker.example#url=" + origin + "&token=" + token},
		{"TokenOutsideTokenParam", "jetbrains://gateway/coder?url=" + origin + "&token=x&ide_download_link=https://attacker.example/" + token},
		{"TokenTwice", "vscode://coder.coder-remote/open?url=" + origin + "&token=" + token + "&t=" + token},
		{"TokenInVSCodeFragment", "vscode://coder.coder-remote/open?url=" + origin + "#token=" + token},
		{"URLInFragment", "vscode://coder.coder-remote/open?url=" + origin + "&token=" + token + "#url=https://attacker.example"},
		{"AnotherExtension", "vscode://attacker.ext/open?url=" + origin + "&token=" + token},
		{"UnknownScheme", "coder://open?url=" + origin + "&token=" + token},
		{"WorkspaceAppPath", "vscode://coder.coder-remote/open?url=" + origin + "/@victim/ws.main/apps/evil/&token=" + token},
		{"DoubleEncodedHost", "vscode://coder.coder-remote/open?url=https://attacker.example%252Fx@coder.example.com/&token=" + token},
		{"DoubleEncodedURLParam", "vscode://coder.coder-remote/open?owner=x%26url%3Dhttps%253A%252F%252Fattacker.example&url=" + origin + "&token=" + token},
		{"TripleEncodedURLParam", "vscode://coder.coder-remote/open?x=%252526url%25253Dhttps%25253A%25252F%25252Fattacker.example&url=" + origin + "&token=" + token},
		{"OpaqueHandler", "vscode:coder.coder-remote/open?url=" + origin + "&token=" + token},
		{"UserinfoInHandler", "vscode://x@coder.coder-remote/open?url=" + origin + "&token=" + token},
		{"DotSegmentsInHandler", "vscode://coder.coder-remote/open/../open?url=" + origin + "&token=" + token},
		{"ParamsInVSCodeFragment", "vscode://coder.coder-remote/open#url=" + origin + "&token=" + token},
		{"ParamsInJetBrainsGatewayQuery", "jetbrains-gateway://connect?url=" + origin + "&token=" + token},
		{"PlaceholderOutsideRawTokenEntry", "jetbrains-gateway://connect#type=coder&url=" + origin + "&token=%24SESSION_TOKEN&ide_download_link=https://attacker.example/" + token},
		{"EncodedURLName", "jetbrains-gateway://connect#type=coder&%75rl=" + origin + "&token=" + token},
		{"NonCoderJetBrainsGatewayProvider", "jetbrains-gateway://connect#type=ssh&host=attacker.example&url=" + origin + "&token=" + token},
		{"DuplicateJetBrainsGatewayType", "jetbrains-gateway://connect#type=coder&url=" + origin + "&token=" + token + "&type=ssh"},
		{"ParamsAfterFragment", "vscode://coder.coder-remote/open?x=1#&url=" + origin + "&token=" + token},
		{"NonASCIIScheme", "Kiro://coder.coder-remote/open?url=" + origin + "&token=" + token},
		{"TabInParamName", "jetbrains-gateway://connect#type=coder&url=" + origin + "&ur\tl=https://attacker.example&token=" + token},
		{"NewlineInParamName", "jetbrains-gateway://connect#type=coder&url=" + origin + "&ur\nl=https://attacker.example&token=" + token},
		{"CarriageReturnInParamName", "jetbrains-gateway://connect#type=coder&url=" + origin + "&ur\rl=https://attacker.example&token=" + token},
		{"HTTPSLink", "https://attacker.example/?token=" + token},
		{"TruncatedHandler", "vscode://coder"},
	}
	for _, tc := range untrusted {
		t.Run("Rejects/"+tc.name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, sessionTokenDestinationTrusted(tc.url, origins))
		})
	}

	t.Run("AccessURLOnAnotherHost", func(t *testing.T) {
		t.Parallel()

		u := "vscode://coder.coder-remote/open?url=https://access.example.com&token=" + token
		assert.True(t, sessionTokenDestinationTrusted(u, []string{origin, "https://access.example.com"}))
		assert.False(t, sessionTokenDestinationTrusted(u, origins))
	})
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
