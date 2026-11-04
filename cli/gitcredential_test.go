package cli_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestGitCredential(t *testing.T) {
	t.Parallel()

	// run invokes "coder gitcredential <op>" the way Git does, with the
	// request on stdin, and returns stdout and stderr.
	run := func(t *testing.T, url, op, request string) (string, string) {
		t.Helper()
		inv, _ := clitest.New(t, "gitcredential", op, "--agent-url", url)
		inv.Environ.Set("CODER_AGENT_TOKEN", "fake-token")
		inv.Stdin = strings.NewReader(request)
		var stdout, stderr bytes.Buffer
		inv.Stdout = &stdout
		inv.Stderr = &stderr
		require.NoError(t, inv.Run())
		return stdout.String(), stderr.String()
	}

	t.Run("TokenAsUsername", func(t *testing.T) {
		t.Parallel()
		expiry := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
		var match atomic.Value
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			match.Store(r.URL.Query().Get("match"))
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				Username:  "gho_token",
				ExpiresAt: expiry,
			})
		}))
		t.Cleanup(srv.Close)

		stdout, _ := run(t, srv.URL, "get", "protocol=https\nhost=github.com\n\n")
		require.Equal(t, "https://github.com", match.Load())
		require.Equal(t, "username=gho_token\npassword=gho_token\npassword_expiry_utc=1893553445\n", stdout)
	})

	t.Run("UsernameAndPassword", func(t *testing.T) {
		t.Parallel()
		var match atomic.Value
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			match.Store(r.URL.Query().Get("match"))
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				Username: "oauth2",
				Password: "glpat",
			})
		}))
		t.Cleanup(srv.Close)

		stdout, _ := run(t, srv.URL, "get", "protocol=https\nhost=gitlab.com\npath=group/repo.git\n\n")
		require.Equal(t, "https://gitlab.com/group/repo.git", match.Load())
		require.Equal(t, "username=oauth2\npassword=glpat\n", stdout)
	})

	t.Run("NoProvider", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusNotFound, codersdk.Response{
				Message: "Nope!",
			})
		}))
		t.Cleanup(srv.Close)

		stdout, stderr := run(t, srv.URL, "get", "protocol=https\nhost=example.com\n\n")
		require.Empty(t, stdout)
		require.Empty(t, stderr)
	})

	t.Run("NotAuthenticated", func(t *testing.T) {
		t.Parallel()
		var requests atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			// The helper must never long-poll for a new token.
			assert.Empty(t, r.URL.Query().Get("listen"))
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				URL: "https://example.com/external-auth/github",
			})
		}))
		t.Cleanup(srv.Close)

		stdout, stderr := run(t, srv.URL, "get", "protocol=https\nhost=github.com\n\n")
		require.Empty(t, stdout)
		require.Contains(t, stderr, "https://example.com/external-auth/github")
		require.EqualValues(t, 1, requests.Load())
	})

	t.Run("IgnoredRequests", func(t *testing.T) {
		t.Parallel()
		var requests atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		for _, tc := range []struct{ op, request string }{
			{"store", "protocol=https\nhost=github.com\nusername=u\npassword=p\n\n"},
			{"erase", "protocol=https\nhost=github.com\n\n"},
			{"get", "protocol=ssh\nhost=github.com\n\n"},
		} {
			stdout, stderr := run(t, srv.URL, tc.op, tc.request)
			require.Empty(t, stdout, tc.op)
			require.Empty(t, stderr, tc.op)
		}
		require.Zero(t, requests.Load())
	})
}
