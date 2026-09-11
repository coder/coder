package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestUpdateUserEmail(t *testing.T) {
	t.Parallel()

	t.Run("CommandReachable", func(t *testing.T) {
		t.Parallel()

		root := getRoot(t)
		var found *serpent.Command
		root.Walk(func(cmd *serpent.Command) {
			if cmd.Name() == "update-user-email" {
				found = cmd
			}
		})
		require.NotNil(t, found, "update-user-email command not found under exp")
		require.True(t, found.Hidden, "command should be hidden")
	})

	t.Run("MissingOldEmail", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "update-user-email", "--new-email", "new@example.com")
		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "old-email")
	})

	t.Run("MissingNewEmail", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "update-user-email", "--old-email", "old@example.com")
		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "new-email")
	})

	t.Run("DeclinePrompt", func(t *testing.T) {
		t.Parallel()

		// Track whether any API call is made.
		apiCalled := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiCalled = true
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(srv.Close)

		client := codersdk.New(must(url.Parse(srv.URL)))
		inv, root := clitest.New(t, "exp", "update-user-email",
			"--old-email", "old@example.com",
			"--new-email", "new@example.com",
		)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("no\n")

		err := inv.Run()
		require.ErrorIs(t, err, cliui.ErrCanceled)
		require.False(t, apiCalled, "API should not be called when prompt is declined")
	})

	t.Run("AcceptPrompt", func(t *testing.T) {
		t.Parallel()

		var gotBody codersdk.UpdateUserEmailRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, "/api/experimental/users/email", r.URL.Path)
			err := json.NewDecoder(r.Body).Decode(&gotBody)
			assert.NoError(t, err)
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(srv.Close)

		client := codersdk.New(must(url.Parse(srv.URL)))
		inv, root := clitest.New(t, "exp", "update-user-email",
			"--old-email", "old@example.com",
			"--new-email", "new@example.com",
		)
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("yes\n")

		var outBuf bytes.Buffer
		inv.Stdout = &outBuf

		require.NoError(t, inv.Run())

		out := outBuf.String()
		require.Contains(t, out, "old@example.com")
		require.Contains(t, out, "new@example.com")
		require.Contains(t, out, "sessions and API tokens")
		require.Contains(t, out, "external identity provider")
		require.Contains(t, out, "Updated user email from old@example.com to new@example.com.")

		require.Equal(t, "old@example.com", gotBody.OldEmail)
		require.Equal(t, "new@example.com", gotBody.NewEmail)
	})

	t.Run("YesSkipsPrompt", func(t *testing.T) {
		t.Parallel()

		var gotBody codersdk.UpdateUserEmailRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := json.NewDecoder(r.Body).Decode(&gotBody)
			assert.NoError(t, err)
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(srv.Close)

		client := codersdk.New(must(url.Parse(srv.URL)))
		inv, root := clitest.New(t, "exp", "update-user-email",
			"--old-email", "old@example.com",
			"--new-email", "new@example.com",
			"--yes",
		)
		clitest.SetupConfig(t, client, root)

		var outBuf bytes.Buffer
		inv.Stdout = &outBuf

		require.NoError(t, inv.Run())
		require.Contains(t, outBuf.String(), "Updated user email from old@example.com to new@example.com.")
		require.Equal(t, "old@example.com", gotBody.OldEmail)
		require.Equal(t, "new@example.com", gotBody.NewEmail)
	})

	t.Run("APIError", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"user not found"}`))
		}))
		t.Cleanup(srv.Close)

		client := codersdk.New(must(url.Parse(srv.URL)))
		inv, root := clitest.New(t, "exp", "update-user-email",
			"--old-email", "old@example.com",
			"--new-email", "new@example.com",
			"--yes",
		)
		clitest.SetupConfig(t, client, root)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "update user email")
	})
}
