package cli_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
)

func TestWhoami(t *testing.T) {
	t.Parallel()

	t.Run("InitialUserNoTTY", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		root, _ := clitest.New(t, "login", client.URL.String())
		err := root.Run()
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		inv, root := clitest.New(t, "whoami")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.Run()
		require.NoError(t, err)
		whoami := buf.String()
		require.NotEmpty(t, whoami)
	})

	t.Run("HTMLResponse", func(t *testing.T) {
		t.Parallel()
		// Simulate an SSO portal or misconfigured reverse proxy that
		// returns 200 OK with an HTML body instead of JSON.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>Sign in</title></head><body>Sign in</body></html>"))
		}))
		defer srv.Close()

		parsedURL, err := url.Parse(srv.URL)
		require.NoError(t, err)
		client := codersdk.New(parsedURL)
		client.SetSessionToken("test-token")

		inv, root := clitest.New(t, "whoami")
		clitest.SetupConfig(t, client, root)
		err = inv.Run()
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.True(t, errors.As(err, &sdkErr))
		require.Equal(t, http.StatusOK, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "HTML response instead of JSON")
		require.Contains(t, sdkErr.Helper, "/api/v2")
		require.NotContains(t, err.Error(), "invalid character")
		require.NotContains(t, err.Error(), "unexpected status code")
	})
}
