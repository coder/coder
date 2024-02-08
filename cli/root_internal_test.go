package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
)

func Test_formatExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		examples    []example
		wantMatches []string
	}{
		{
			name:        "No examples",
			examples:    nil,
			wantMatches: nil,
		},
		{
			name: "Output examples",
			examples: []example{
				{
					Description: "Hello world.",
					Command:     "echo hello",
				},
				{
					Description: "Bye bye.",
					Command:     "echo bye",
				},
			},
			wantMatches: []string{
				"Hello world", "echo hello",
				"Bye bye", "echo bye",
			},
		},
		{
			name: "No description outputs commands",
			examples: []example{
				{
					Command: "echo hello",
				},
			},
			wantMatches: []string{
				"echo hello",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatExamples(tt.examples...)
			if len(tt.wantMatches) == 0 {
				require.Empty(t, got)
			} else {
				for _, want := range tt.wantMatches {
					require.Contains(t, got, want)
				}
			}
		})
	}
}

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		// Don't run goleak on windows tests, they're super flaky right now.
		// See: https://github.com/coder/coder/issues/8954
		os.Exit(m.Run())
	}
	goleak.VerifyTestMain(m,
		// The lumberjack library is used by by agent and seems to leave
		// goroutines after Close(), fails TestGitSSH tests.
		// https://github.com/natefinch/lumberjack/pull/100
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).mill.func1"),
		// The pq library appears to leave around a goroutine after Close().
		goleak.IgnoreTopFunction("github.com/lib/pq.NewDialListener"),
	)
}

func Test_checkVersions(t *testing.T) {
	t.Parallel()

	t.Run("CustomUpgradeMessage", func(t *testing.T) {
		t.Parallel()

		expectedUpgradeMessage := "My custom upgrade message"

		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.BuildInfoResponse{
				ExternalURL: buildinfo.ExternalURL(),
				// Provide a version that will not match
				Version:         "v1.0.0",
				AgentAPIVersion: coderd.AgentAPIVersionREST,
				// does not matter what the url is
				DashboardURL:   "https://example.com",
				WorkspaceProxy: false,
				UpgradeMessage: expectedUpgradeMessage,
			})
		}))
		defer srv.Close()
		surl, err := url.Parse(srv.URL)
		require.NoError(t, err)

		client := codersdk.New(surl)

		r := &RootCmd{}

		cmd, err := r.Command(nil)
		require.NoError(t, err)

		var buf bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &buf

		err = r.checkVersions(inv, client, "v2.0.0")
		require.NoError(t, err)

		fmtOutput := fmt.Sprintf("version mismatch: client v2.0.0, server v1.0.0\n%s", expectedUpgradeMessage)
		expectedOutput := fmt.Sprintln(pretty.Sprint(cliui.DefaultStyles.Warn, fmtOutput))
		require.Equal(t, expectedOutput, buf.String())
	})

	t.Run("DefaultUpgradeMessage", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.BuildInfoResponse{
				ExternalURL: buildinfo.ExternalURL(),
				// Provide a version that will not match
				Version:         "v1.0.0",
				AgentAPIVersion: coderd.AgentAPIVersionREST,
				// does not matter what the url is
				DashboardURL:   "https://example.com",
				WorkspaceProxy: false,
				UpgradeMessage: "",
			})
		}))
		defer srv.Close()
		surl, err := url.Parse(srv.URL)
		require.NoError(t, err)

		client := codersdk.New(surl)

		r := &RootCmd{}

		cmd, err := r.Command(nil)
		require.NoError(t, err)

		var buf bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &buf

		err = r.checkVersions(inv, client, "v2.0.0")
		require.NoError(t, err)

		fmtOutput := fmt.Sprintf("version mismatch: client v2.0.0, server v1.0.0\n%s", defaultUpgradeMessage("v1.0.0"))
		expectedOutput := fmt.Sprintln(pretty.Sprint(cliui.DefaultStyles.Warn, fmtOutput))
		require.Equal(t, expectedOutput, buf.String())
	})
}
