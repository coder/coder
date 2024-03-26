package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

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

func Test_wrapTransportWithVersionMismatchCheck(t *testing.T) {
	t.Parallel()

	t.Run("NoOutput", func(t *testing.T) {
		t.Parallel()
		r := &RootCmd{}
		cmd, err := r.Command(nil)
		require.NoError(t, err)
		var buf bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &buf
		rt := wrapTransportWithVersionMismatchCheck(roundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					// Provider a version that will not match!
					codersdk.BuildVersionHeader: []string{"v2.0.0"},
				},
				Body: io.NopCloser(nil),
			}, nil
		}), inv, "v2.0.0", nil)
		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		res, err := rt.RoundTrip(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, "", buf.String())
	})

	t.Run("CustomUpgradeMessage", func(t *testing.T) {
		t.Parallel()

		r := &RootCmd{}

		cmd, err := r.Command(nil)
		require.NoError(t, err)

		var buf bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &buf
		expectedUpgradeMessage := "My custom upgrade message"
		rt := wrapTransportWithVersionMismatchCheck(roundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					// Provider a version that will not match!
					codersdk.BuildVersionHeader: []string{"v1.0.0"},
				},
				Body: io.NopCloser(nil),
			}, nil
		}), inv, "v2.0.0", func(ctx context.Context) (codersdk.BuildInfoResponse, error) {
			return codersdk.BuildInfoResponse{
				UpgradeMessage: expectedUpgradeMessage,
			}, nil
		})
		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		res, err := rt.RoundTrip(req)
		require.NoError(t, err)
		defer res.Body.Close()

		// Run this twice to ensure the upgrade message is only printed once.
		res, err = rt.RoundTrip(req)
		require.NoError(t, err)
		defer res.Body.Close()

		fmtOutput := fmt.Sprintf("version mismatch: client v2.0.0, server v1.0.0\n%s", expectedUpgradeMessage)
		expectedOutput := fmt.Sprintln(pretty.Sprint(cliui.DefaultStyles.Warn, fmtOutput))
		require.Equal(t, expectedOutput, buf.String())
	})
}

func Test_wrapTransportWithTelemetryHeader(t *testing.T) {
	t.Parallel()

	rt := wrapTransportWithTelemetryHeader(roundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Body: io.NopCloser(nil),
		}, nil
	}), &serpent.Invocation{
		Command: &serpent.Command{
			Use: "test",
			Options: serpent.OptionSet{{
				Name:        "bananas",
				Description: "hey",
			}},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	res, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer res.Body.Close()
	resp := req.Header.Get(codersdk.CLITelemetryHeader)
	require.NotEmpty(t, resp)
	data, err := base64.StdEncoding.DecodeString(resp)
	require.NoError(t, err)
	var ti telemetry.Invocation
	err = json.Unmarshal(data, &ti)
	require.NoError(t, err)
	require.Equal(t, ti.Command, "test")
}

func Test_wrapTransportWithEntitlementsCheck(t *testing.T) {
	t.Parallel()

	lines := []string{"First Warning", "Second Warning"}
	var buf bytes.Buffer
	rt := wrapTransportWithEntitlementsCheck(roundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				codersdk.EntitlementsWarningHeader: lines,
			},
			Body: io.NopCloser(nil),
		}, nil
	}), &buf)
	res, err := rt.RoundTrip(httptest.NewRequest(http.MethodGet, "http://example.com", nil))
	require.NoError(t, err)
	defer res.Body.Close()
	expectedOutput := fmt.Sprintf("%s\n%s\n", pretty.Sprint(cliui.DefaultStyles.Warn, lines[0]),
		pretty.Sprint(cliui.DefaultStyles.Warn, lines[1]))
	require.Equal(t, expectedOutput, buf.String())
}
