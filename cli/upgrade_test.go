package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestUpgrade(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skipf("Detected Windows OS, skipping upgrade tests")
		return
	}

	// startFakeDeployment starts and returns a fake deployment
	// that returns the specified version when the upgrade command
	// requests build info.
	startFakeDeployment := func(t *testing.T, version string) string {
		t.Helper()
		s := httptest.NewServer(
			http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					err := json.NewEncoder(w).Encode(codersdk.BuildInfoResponse{
						Version: version,
					})
					require.NoError(t, err)
				},
			),
		)
		t.Cleanup(s.Close)
		return s.URL
	}

	// startFakeInstallServer starts and returns a fake server
	// that returns the specified script that is used to "install"
	// coder.
	startFakeInstallServer := func(t *testing.T, script string) string {
		t.Helper()
		s := httptest.NewServer(
			http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					_, err := w.Write([]byte(script))
					require.NoError(t, err)
				}),
		)
		t.Cleanup(s.Close)
		return s.URL
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			serverVersion   = "v1.2.3-devel+f72d8e95"
			expectedVersion = "1.2.3"
			script          = fmt.Sprintf(`#!/usr/bin/env bash
		echo "testing 123"
		echo %s`, expectedVersion)
		)

		serverURL := startFakeDeployment(t, serverVersion)
		installURL := startFakeInstallServer(t, script)

		inv, _ := clitest.New(t, "upgrade", "--url", serverURL, "--install-script-url", installURL)
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitMedium)

		done := make(chan any)
		go func() {
			errC := inv.WithContext(ctx).Run()
			assert.NoError(t, errC)
			close(done)
		}()
		pty.ExpectMatch(fmt.Sprintf("Detected server version %q, downloading version %q from %s", serverVersion, expectedVersion, installURL))
		pty.ExpectMatch("testing 123")
		pty.ExpectMatch(expectedVersion)
		<-done
	})

	t.Run("NoServerURL", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "upgrade")
		pty := ptytest.New(t).Attach(inv)

		ctx := testutil.Context(t, testutil.WaitMedium)
		done := make(chan any)
		go func() {
			errC := inv.WithContext(ctx).Run()
			assert.Error(t, errC)
			close(done)
		}()

		pty.ExpectMatch("No deployment URL provided. You must either login using")
		<-done
	})
}
