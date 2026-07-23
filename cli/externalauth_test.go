package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestExternalAuth(t *testing.T) {
	t.Parallel()
	t.Run("CanceledWithURL", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				URL: "https://github.com",
			})
		}))
		t.Cleanup(srv.Close)
		url := srv.URL
		inv, _ := clitest.New(t, "--agent-url", url, "--agent-token", "foo", "external-auth", "access-token", "github")
		stdout := expecter.NewAttachedToInvocation(t, inv)
		waiter := clitest.StartWithWaiter(t, inv)
		stdout.ExpectMatch(ctx, "https://github.com")
		waiter.RequireIs(cliui.ErrCanceled)
	})
	t.Run("SuccessWithToken", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				AccessToken: "bananas",
			})
		}))
		t.Cleanup(srv.Close)
		url := srv.URL
		inv, _ := clitest.New(t, "--agent-url", url, "--agent-token", "foo", "external-auth", "access-token", "github")
		stdout := expecter.NewAttachedToInvocation(t, inv)
		clitest.Start(t, inv)
		stdout.ExpectMatch(ctx, "bananas")
	})
	t.Run("NoArgs", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				AccessToken: "bananas",
			})
		}))
		t.Cleanup(srv.Close)
		url := srv.URL
		inv, _ := clitest.New(t, "--agent-url", url, "--agent-token", "foo", "external-auth", "access-token")
		watier := clitest.StartWithWaiter(t, inv)
		watier.RequireContains("wanted 1 args but got 0")
	})
	t.Run("SuccessWithExtra", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(context.Background(), w, http.StatusOK, agentsdk.ExternalAuthResponse{
				AccessToken: "bananas",
				TokenExtra: map[string]any{
					"hey": "there",
				},
			})
		}))
		t.Cleanup(srv.Close)
		url := srv.URL
		inv, _ := clitest.New(t, "--agent-url", url, "--agent-token", "foo", "external-auth", "access-token", "github", "--extra", "hey")
		stdout := expecter.NewAttachedToInvocation(t, inv)
		clitest.Start(t, inv)
		stdout.ExpectMatch(ctx, "there")
	})
	t.Run("JSONOutput", func(t *testing.T) {
		t.Parallel()
		expiry := time.Now().Add(8 * time.Hour).UTC().Truncate(time.Second)

		tests := []struct {
			name    string
			resp    agentsdk.ExternalAuthResponse
			wantErr error
		}{
			{
				name: "WithExpiry",
				resp: agentsdk.ExternalAuthResponse{AccessToken: "bananas", ExpiresAt: expiry},
			},
			{
				name:    "WithURL",
				resp:    agentsdk.ExternalAuthResponse{URL: "https://github.com/login"},
				wantErr: cliui.ErrCanceled,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					httpapi.Write(context.Background(), w, http.StatusOK, tt.resp)
				}))
				t.Cleanup(srv.Close)
				inv, _ := clitest.New(t, "--agent-url", srv.URL, "--agent-token", "foo", "external-auth", "access-token", "github", "--output", "json")
				buf := new(bytes.Buffer)
				inv.Stdout = buf
				waiter := clitest.StartWithWaiter(t, inv)
				if tt.wantErr != nil {
					waiter.RequireIs(tt.wantErr)
				} else {
					waiter.RequireSuccess()
				}

				var resp agentsdk.ExternalAuthResponse
				require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
				require.Equal(t, tt.resp.AccessToken, resp.AccessToken)
				require.Equal(t, tt.resp.URL, resp.URL)
				require.Equal(t, tt.resp.ExpiresAt.UTC(), resp.ExpiresAt.UTC())
			})
		}
	})
}
