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

//nolint:paralleltest // t.Setenv is incompatible with t.Parallel.
func TestPrintDeprecatedOptions(t *testing.T) {
	//nolint:paralleltest // t.Setenv is incompatible with t.Parallel.
	t.Run("NoWarningWhenNewEnvSet", func(t *testing.T) {
		// Simulate the scenario from #23847: the new env var
		// (CODER_EMAIL_FROM) is set, but the old deprecated one
		// (CODER_NOTIFICATIONS_EMAIL_FROM) is not. Both options
		// share the same Value pointer, so serpent propagates
		// ValueSource to both. The warning should NOT fire for the
		// deprecated option because it was not directly set.
		var value serpent.String

		newOpt := serpent.Option{
			Name:        "Email From",
			Flag:        "email-from",
			Env:         "CODER_TEST_EMAIL_FROM",
			Value:       &value,
			ValueSource: serpent.ValueSourceEnv,
		}
		deprecatedOpt := serpent.Option{
			Name:  "Notifications Email From",
			Flag:  "notifications-email-from",
			Env:   "CODER_TEST_NOTIFICATIONS_EMAIL_FROM",
			Value: &value,
			// Serpent propagates ValueSource from the new option.
			ValueSource: serpent.ValueSourceEnv,
			UseInstead:  serpent.OptionSet{newOpt},
		}

		// Set only the new env var, not the deprecated one.
		t.Setenv("CODER_TEST_EMAIL_FROM", "test@example.com")

		cmd := &serpent.Command{
			Use: "test",
			Options: serpent.OptionSet{
				newOpt,
				deprecatedOpt,
			},
			Handler: func(_ *serpent.Invocation) error {
				return nil
			},
		}

		var stderr bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &stderr
		inv.Stdout = io.Discard

		mw := PrintDeprecatedOptions()
		err := mw(func(_ *serpent.Invocation) error {
			return nil
		})(inv)
		require.NoError(t, err)
		require.Empty(t, stderr.String(), "should not warn when deprecated env var is not set")
	})

	//nolint:paralleltest // t.Setenv is incompatible with t.Parallel.
	t.Run("WarningWhenDeprecatedEnvSet", func(t *testing.T) {
		var value serpent.String

		newOpt := serpent.Option{
			Name:  "Email From",
			Flag:  "email-from",
			Env:   "CODER_TEST_EMAIL_FROM_2",
			Value: &value,
		}
		deprecatedOpt := serpent.Option{
			Name:        "Notifications Email From",
			Flag:        "notifications-email-from-2",
			Env:         "CODER_TEST_NOTIFICATIONS_EMAIL_FROM_2",
			Value:       &value,
			ValueSource: serpent.ValueSourceEnv,
			UseInstead:  serpent.OptionSet{newOpt},
		}

		// Set the deprecated env var.
		t.Setenv("CODER_TEST_NOTIFICATIONS_EMAIL_FROM_2", "test@example.com")

		cmd := &serpent.Command{
			Use: "test",
			Options: serpent.OptionSet{
				newOpt,
				deprecatedOpt,
			},
			Handler: func(_ *serpent.Invocation) error {
				return nil
			},
		}

		var stderr bytes.Buffer
		inv := cmd.Invoke()
		inv.Stderr = &stderr
		inv.Stdout = io.Discard

		mw := PrintDeprecatedOptions()
		err := mw(func(_ *serpent.Invocation) error {
			return nil
		})(inv)
		require.NoError(t, err)
		require.Contains(t, stderr.String(), "CODER_TEST_NOTIFICATIONS_EMAIL_FROM_2",
			"should warn when deprecated env var is set")
		require.Contains(t, stderr.String(), "deprecated")
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
