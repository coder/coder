package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestGitAskpass(t *testing.T) {
	t.Parallel()
	t.Run("UsernameAndPassword", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				Username: "something",
				Password: "bananas",
			})
		}))
		t.Cleanup(srv.Close)
		url := srv.URL
		inv, _ := clitest.New(t, "--agent-url", url, "Username for 'https://github.com':")
		inv.Environ.Set("GIT_PREFIX", "/")
		inv.Environ.Set("CODER_AGENT_TOKEN", "fake-token")
		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.Start(t, inv)
		pty.ExpectMatch("something")

		inv, _ = clitest.New(t, "--agent-url", url, "Password for 'https://potato@github.com':")
		inv.Environ.Set("GIT_PREFIX", "/")
		inv.Environ.Set("CODER_AGENT_TOKEN", "fake-token")
		pty = ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.Start(t, inv)
		pty.ExpectMatch("bananas")
	})

	t.Run("NoHost", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusNotFound, codersdk.Response{
				Message: "Nope!",
			})
		}))
		t.Cleanup(srv.Close)
		url := srv.URL
		inv, _ := clitest.New(t, "--agent-url", url, "--no-open", "Username for 'https://github.com':")
		inv.Environ.Set("GIT_PREFIX", "/")
		inv.Environ.Set("CODER_AGENT_TOKEN", "fake-token")
		pty := ptytest.New(t)
		inv.Stderr = pty.Output()
		err := inv.Run()
		require.ErrorIs(t, err, cliui.ErrCanceled)
		pty.ExpectMatch("Nope!")
	})

	t.Run("Poll", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		resp := atomic.Pointer[agentsdk.ExternalAuthResponse]{}
		resp.Store(&agentsdk.ExternalAuthResponse{
			URL: "https://something.org",
		})
		poll := make(chan struct{}, 10)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			val := resp.Load()
			if r.URL.Query().Has("listen") {
				poll <- struct{}{}
				if val.URL != "" {
					httpapi.Write(context.Background(), w, http.StatusInternalServerError, val)
					return
				}
			}
			httpapi.Write(context.Background(), w, http.StatusOK, val)
		}))
		t.Cleanup(srv.Close)
		url := srv.URL

		inv, _ := clitest.New(t, "--agent-url", url, "--no-open", "Username for 'https://github.com':")
		inv.Environ.Set("GIT_PREFIX", "/")
		inv.Environ.Set("CODER_AGENT_TOKEN", "fake-token")
		stdout := ptytest.New(t)
		inv.Stdout = stdout.Output()
		stderr := ptytest.New(t)
		inv.Stderr = stderr.Output()
		go func() {
			err := inv.Run()
			assert.NoError(t, err)
		}()
		testutil.RequireReceive(ctx, t, poll)
		stderr.ExpectMatch("Open the following URL to authenticate")
		resp.Store(&agentsdk.ExternalAuthResponse{
			Username: "username",
			Password: "password",
		})
		stdout.ExpectMatch("username")
	})

	t.Run("ChatAgentAuthRequired", func(t *testing.T) {
		t.Parallel()

		listenCalls := atomic.Int64{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Has("listen") {
				listenCalls.Add(1)
			}
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				URL:  "https://coder.example.com/external-auth/github",
				Type: codersdk.EnhancedExternalAuthProviderGitHub.String(),
			})
		}))
		t.Cleanup(srv.Close)

		inv, _ := clitest.New(
			t,
			"--agent-url",
			srv.URL,
			"--no-open",
			"Username for 'https://github.com':",
		)
		inv.Environ.Set("GIT_PREFIX", "/")
		inv.Environ.Set("CODER_AGENT_TOKEN", "fake-token")
		inv.Environ.Set("CODER_CHAT_AGENT", "true")

		var stderr bytes.Buffer
		inv.Stderr = &stderr

		err := inv.Run()
		require.Error(t, err)
		require.Condition(t, func() bool {
			message := err.Error()
			return strings.Contains(message, "exit code") || strings.Contains(message, "exit status")
		}, "expected exit status error, got %q", err.Error())
		require.Zero(t, listenCalls.Load())

		output := stderr.String()
		require.Contains(t, output, "CODER_GITAUTH_REQUIRED:")
		require.NotContains(t, output, "Open the following URL to authenticate")
		require.NotContains(t, output, "Your browser has been opened")

		_, markerRaw, found := strings.Cut(output, "CODER_GITAUTH_REQUIRED:")
		require.True(t, found)
		var marker struct {
			ProviderID          string `json:"provider_id"`
			ProviderType        string `json:"provider_type"`
			ProviderDisplayName string `json:"provider_display_name"`
			AuthenticateURL     string `json:"authenticate_url"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(markerRaw)), &marker))
		require.Equal(t, "github", marker.ProviderID)
		require.Equal(t, codersdk.EnhancedExternalAuthProviderGitHub.String(), marker.ProviderType)
		require.Equal(t, "GitHub", marker.ProviderDisplayName)
		require.Equal(t, "https://coder.example.com/external-auth/github", marker.AuthenticateURL)
	})
}
