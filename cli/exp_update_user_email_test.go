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
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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

		// Use a channel closed by the handler to detect whether the API is called.
		apiCalled := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(apiCalled)
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

		ctx := testutil.Context(t, testutil.WaitShort)
		done := make(chan struct{})
		var runErr error
		go func() {
			defer close(done)
			runErr = inv.Run()
		}()

		testutil.TryReceive(ctx, t, done)
		require.ErrorIs(t, runErr, cliui.ErrCanceled)

		// Verify the API was not called after the command returned.
		select {
		case <-apiCalled:
			t.Fatal("API should not be called when prompt is declined")
		default:
		}
	})

	// AcceptPrompt runs a full end-to-end test against a real Coder server: it
	// creates a second user, invokes the CLI as an admin, confirms the prompt,
	// and asserts both the command output and the actual stored email.
	t.Run("AcceptPrompt", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitShort)

		oldEmail := member.Email
		newEmail := "updated-" + oldEmail

		inv, root := clitest.New(t, "exp", "update-user-email",
			"--old-email", oldEmail,
			"--new-email", newEmail,
		)
		//nolint:gocritic // This break-glass command is restricted to deployment owners.
		clitest.SetupConfig(t, client, root)
		inv.Stdin = strings.NewReader("yes\n")

		var outBuf bytes.Buffer
		inv.Stdout = &outBuf

		require.NoError(t, inv.Run())

		out := outBuf.String()
		require.Contains(t, out, oldEmail)
		require.Contains(t, out, newEmail)
		require.Contains(t, out, "sessions and API tokens")
		require.Contains(t, out, "external identity provider")
		require.Contains(t, out, "Updated user email from "+oldEmail+" to "+newEmail+".")

		// Verify the email was actually persisted.
		updated, err := client.User(ctx, member.ID.String())
		require.NoError(t, err)
		require.Equal(t, newEmail, updated.Email)
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
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"internal server error"}`))
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

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "internal server error")
	})
}
