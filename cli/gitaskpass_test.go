package cli_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/pty/ptytest"
)

// nolint:paralleltest
func TestGitAskpass(t *testing.T) {
	t.Setenv("GIT_PREFIX", "/")
	t.Run("UsernameAndPassword", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.GitAuthResponse{
				Username: "something",
				Password: "bananas",
			})
		}))
		t.Cleanup(srv.Close)
		url := srv.URL
		cmd, _ := clitest.New(t, "--agent-url", url, "Username for 'https://github.com':")
		pty := ptytest.New(t)
		cmd.SetOutput(pty.Output())
		err := cmd.Execute()
		require.NoError(t, err)
		pty.ExpectMatch("something")

		cmd, _ = clitest.New(t, "--agent-url", url, "Password for 'https://potato@github.com':")
		pty = ptytest.New(t)
		cmd.SetOutput(pty.Output())
		err = cmd.Execute()
		require.NoError(t, err)
		pty.ExpectMatch("bananas")
	})

	t.Run("NoHost", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusNotFound, codersdk.Response{
				Message: "Nope!",
			})
		}))
		t.Cleanup(srv.Close)
		url := srv.URL
		cmd, _ := clitest.New(t, "--agent-url", url, "--no-open", "Username for 'https://github.com':")
		pty := ptytest.New(t)
		cmd.SetOutput(pty.Output())
		err := cmd.Execute()
		require.ErrorIs(t, err, cliui.Canceled)
		pty.ExpectMatch("Nope!")
	})

	t.Run("Poll", func(t *testing.T) {
		resp := atomic.Pointer[agentsdk.GitAuthResponse]{}
		resp.Store(&agentsdk.GitAuthResponse{
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

		cmd, _ := clitest.New(t, "--agent-url", url, "--no-open", "Username for 'https://github.com':")
		pty := ptytest.New(t)
		cmd.SetOutput(pty.Output())
		go func() {
			err := cmd.Execute()
			assert.NoError(t, err)
		}()
		<-poll
		resp.Store(&agentsdk.GitAuthResponse{
			Username: "username",
			Password: "password",
		})
		pty.ExpectMatch("username")
	})
}
