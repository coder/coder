package cli_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIDEURLFormats tests the URL formats used by different IDEs
func TestIDEURLFormats(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		scheme      string
		host        string
		path        string
		workspaceID string
		directory   string
		expectedURL string
	}{
		{
			name:        "Fleet format",
			scheme:      "fleet",
			host:        "fleet.ssh",
			path:        "/coder.my-workspace.main",
			workspaceID: "my-workspace.main",
			directory:   "/home/coder/project",
			expectedURL: "fleet://fleet.ssh/coder.my-workspace.main?pwd=%2Fhome%2Fcoder%2Fproject",
		},
		{
			name:        "Zed format",
			scheme:      "zed",
			host:        "ssh",
			path:        "/coder.my-workspace.main",
			workspaceID: "my-workspace.main",
			directory:   "/home/coder/project",
			expectedURL: "zed://ssh/coder.my-workspace.main/home/coder/project",
		},
		{
			name:        "VSCode format",
			scheme:      "vscode",
			host:        "coder.coder-remote",
			path:        "/open",
			workspaceID: "my-workspace.main",
			directory:   "/home/coder/project",
			expectedURL: "vscode://coder.coder-remote/open",
		},
		{
			name:        "Cursor format",
			scheme:      "cursor",
			host:        "coder.coder-remote",
			path:        "/open",
			workspaceID: "my-workspace.main",
			directory:   "/home/coder/project",
			expectedURL: "cursor://coder.coder-remote/open",
		},
		{
			name:        "Windsurf format",
			scheme:      "windsurf",
			host:        "coder.coder-remote",
			path:        "/open",
			workspaceID: "my-workspace.main",
			directory:   "/home/coder/project",
			expectedURL: "windsurf://coder.coder-remote/open",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := &url.URL{
				Scheme: tc.scheme,
				Host:   tc.host,
				Path:   tc.path,
			}

			// Add query parameters for Fleet
			if tc.scheme == "fleet" && tc.directory != "" {
				q := url.Values{}
				q.Set("pwd", tc.directory)
				u.RawQuery = q.Encode()
			}

			// Add path for Zed
			if tc.scheme == "zed" && tc.directory != "" {
				u.Path = tc.path + tc.directory
			}

			assert.Contains(t, u.String(), tc.expectedURL, "URL should match expected format")
		})
	}
}
