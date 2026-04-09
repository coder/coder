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
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		// Don't run goleak on windows tests, they're super flaky right now.
		// See: https://github.com/coder/coder/issues/8954
		os.Exit(m.Run())
	}
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func Test_formatExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		examples    []Example
		wantMatches []string
	}{
		{
			name:        "No examples",
			examples:    nil,
			wantMatches: nil,
		},
		{
			name: "Output examples",
			examples: []Example{
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
			examples: []Example{
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FormatExamples(tt.examples...)
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

func Test_wrapTransportWithVersionCheck(t *testing.T) {
	t.Parallel()

	t.Run("NoOutput", func(t *testing.T) {
		t.Parallel()
		r := &RootCmd{}
		cmd, err := r.Command(nil)
		require.NoError(t, err)
		var buf bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &buf
		rt := wrapTransportWithVersionCheck(roundTripper(func(req *http.Request) (*http.Response, error) {
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
		rt := wrapTransportWithVersionCheck(roundTripper(func(req *http.Request) (*http.Response, error) {
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

	t.Run("ServerStableVersion", func(t *testing.T) {
		t.Parallel()
		r := &RootCmd{}
		cmd, err := r.Command(nil)
		require.NoError(t, err)
		var buf bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &buf
		rt := wrapTransportWithVersionCheck(roundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					codersdk.BuildVersionHeader: []string{"v2.31.0"},
				},
				Body: io.NopCloser(nil),
			}, nil
		}), inv, "v2.31.0", nil)
		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		res, err := rt.RoundTrip(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Empty(t, buf.String())
	})
}

func Test_serverVersionMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		version  string
		expected string
	}{
		{"Stable", "v2.31.0", ""},
		{"Dev", "v0.0.0-devel+abc123", "the server is running a development version of Coder (v0.0.0-devel+abc123)"},
		{"RC", "v2.31.0-rc.1", "the server is running a release candidate of Coder (v2.31.0-rc.1)"},
		{"RCDevel", "v2.33.0-rc.1-devel+727ec00f7", "the server is running a release candidate of Coder (v2.33.0-rc.1-devel+727ec00f7)"},
		{"Empty", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, c.expected, serverVersionMessage(c.version))
		})
	}
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

//nolint:tparallel,paralleltest // This test modifies environment variables.
func TestPrintDeprecatedOptions(t *testing.T) {
	newValue := serpent.StringOf(new(string))

	// Both the "new" option and the deprecated option point at the
	// same Value, mirroring how codersdk/deployment.go wires the
	// CODER_EMAIL_* / CODER_NOTIFICATIONS_EMAIL_* pairs.
	newOpt := serpent.Option{
		Name:  "new-option",
		Flag:  "new-option",
		Env:   "CODER_TEST_NEW_OPTION",
		Value: newValue,
	}
	deprecatedOpt := serpent.Option{
		Name:       "old-option",
		Flag:       "old-option",
		Env:        "CODER_TEST_OLD_OPTION",
		Value:      newValue, // same pointer
		UseInstead: serpent.OptionSet{newOpt},
	}

	makeCmd := func(opts serpent.OptionSet) *serpent.Command {
		return &serpent.Command{
			Use:        "test",
			Options:    opts,
			Middleware: PrintDeprecatedOptions(),
			Handler: func(_ *serpent.Invocation) error {
				return nil
			},
		}
	}

	t.Run("EnvOnlyNew_NoWarning", func(t *testing.T) {
		t.Setenv("CODER_TEST_NEW_OPTION", "val")

		cmd := makeCmd(serpent.OptionSet{newOpt, deprecatedOpt})
		var stderr bytes.Buffer
		inv := cmd.Invoke()
		inv.Environ = serpent.ParseEnviron(os.Environ(), "")
		inv.Stderr = &stderr
		err := inv.Run()
		require.NoError(t, err)
		require.Empty(t, stderr.String(),
			"setting only the new env var should not produce a deprecation warning")
	})

	t.Run("EnvOnlyOld_Warning", func(t *testing.T) {
		t.Setenv("CODER_TEST_OLD_OPTION", "val")

		cmd := makeCmd(serpent.OptionSet{newOpt, deprecatedOpt})
		var stderr bytes.Buffer
		inv := cmd.Invoke()
		inv.Environ = serpent.ParseEnviron(os.Environ(), "")
		inv.Stderr = &stderr
		err := inv.Run()
		require.NoError(t, err)
		require.Contains(t, stderr.String(), "is deprecated",
			"setting the deprecated env var should produce a warning")
	})

	t.Run("EnvBothSet_Warning", func(t *testing.T) {
		t.Setenv("CODER_TEST_NEW_OPTION", "new")
		t.Setenv("CODER_TEST_OLD_OPTION", "old")

		cmd := makeCmd(serpent.OptionSet{newOpt, deprecatedOpt})
		var stderr bytes.Buffer
		inv := cmd.Invoke()
		inv.Environ = serpent.ParseEnviron(os.Environ(), "")
		inv.Stderr = &stderr
		err := inv.Run()
		require.NoError(t, err)
		require.Contains(t, stderr.String(), "is deprecated",
			"setting both env vars should still warn about the deprecated one")
	})

	t.Run("DeprecatedEnvAndNewFlag_Warning", func(t *testing.T) {
		t.Setenv("CODER_TEST_OLD_OPTION", "val")

		cmd := makeCmd(serpent.OptionSet{newOpt, deprecatedOpt})
		var stderr bytes.Buffer
		inv := cmd.Invoke("--new-option", "val")
		inv.Environ = serpent.ParseEnviron(os.Environ(), "")
		inv.Stderr = &stderr
		err := inv.Run()
		require.NoError(t, err)
		require.Contains(t, stderr.String(), "`CODER_TEST_OLD_OPTION` is deprecated",
			"setting the deprecated env var should still warn even if the replacement flag overrides the value")
		require.NotContains(t, stderr.String(), "`--old-option` is deprecated",
			"the deprecated environment variable should not be misreported as a deprecated flag")
	})

	t.Run("FlagOnlyNew_NoWarning", func(t *testing.T) {
		cmd := makeCmd(serpent.OptionSet{newOpt, deprecatedOpt})
		var stderr bytes.Buffer
		inv := cmd.Invoke("--new-option", "val")
		inv.Stderr = &stderr
		err := inv.Run()
		require.NoError(t, err)
		require.Empty(t, stderr.String(),
			"passing only the new flag should not produce a deprecation warning")
	})

	t.Run("FlagOnlyOld_Warning", func(t *testing.T) {
		cmd := makeCmd(serpent.OptionSet{newOpt, deprecatedOpt})
		var stderr bytes.Buffer
		inv := cmd.Invoke("--old-option", "val")
		inv.Stderr = &stderr
		err := inv.Run()
		require.NoError(t, err)
		require.Contains(t, stderr.String(), "is deprecated",
			"passing the deprecated flag should produce a warning")
	})

	t.Run("CODER_EMAIL_FROM_NoWarning", func(t *testing.T) {
		t.Setenv("CODER_EMAIL_FROM", "noreply@example.com")

		deploymentValues := new(codersdk.DeploymentValues)
		cmd := makeCmd(deploymentValues.Options())
		var stderr bytes.Buffer
		inv := cmd.Invoke()
		inv.Environ = serpent.ParseEnviron([]string{"CODER_EMAIL_FROM=noreply@example.com"}, "")
		inv.Stderr = &stderr
		err := inv.Run()
		require.NoError(t, err)
		require.NotContains(t, stderr.String(), "is deprecated",
			"setting only CODER_EMAIL_FROM should not produce any deprecation warning")
	})

	t.Run("NothingSet_NoWarning", func(t *testing.T) {
		t.Parallel()

		cmd := makeCmd(serpent.OptionSet{newOpt, deprecatedOpt})
		var stderr bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &stderr
		err := inv.Run()
		require.NoError(t, err)
		require.Empty(t, stderr.String(),
			"setting nothing should not produce a deprecation warning")
	})
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
