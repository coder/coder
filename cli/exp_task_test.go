package cli_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentapi "github.com/coder/agentapi-sdk-go"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resp     *codersdk.Response
		status   *agentapi.GetStatusResponse
		expected codersdk.WorkspaceAppStatusState
	}{
		{
			name: "ReportWorking",
			resp: nil,
			status: &agentapi.GetStatusResponse{
				Status: agentapi.StatusRunning,
			},
			expected: codersdk.WorkspaceAppStatusStateWorking,
		},
		{
			name: "ReportComplete",
			resp: nil,
			status: &agentapi.GetStatusResponse{
				Status: agentapi.StatusStable,
			},
			expected: codersdk.WorkspaceAppStatusStateComplete,
		},
		{
			name: "ReportUpdateError",
			resp: &codersdk.Response{
				Message: "Failed to get workspace app.",
				Detail:  "This is a test failure.",
			},
			status: &agentapi.GetStatusResponse{
				Status: agentapi.StatusStable,
			},
			expected: codersdk.WorkspaceAppStatusStateComplete,
		},
		{
			name:     "ReportStatusError",
			resp:     nil,
			status:   nil,
			expected: codersdk.WorkspaceAppStatusStateComplete,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			done := make(chan codersdk.WorkspaceAppStatusState)

			// A mock server for coderd.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				_ = r.Body.Close()

				var req agentsdk.PatchAppStatus
				err = json.Unmarshal(body, &req)
				require.NoError(t, err)

				if test.resp != nil {
					httpapi.Write(context.Background(), w, http.StatusBadRequest, test.resp)
				} else {
					httpapi.Write(context.Background(), w, http.StatusOK, nil)
				}
				done <- req.State
			}))
			t.Cleanup(srv.Close)
			agentURL := srv.URL

			// Another mock server for the LLM agent API.
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.status != nil {
					httpapi.Write(context.Background(), w, http.StatusOK, test.status)
				} else {
					httpapi.Write(context.Background(), w, http.StatusBadRequest, nil)
				}
			}))
			t.Cleanup(srv.Close)
			agentapiURL := srv.URL

			inv, _ := clitest.New(t, "--agent-url", agentURL, "exp", "task", "report-status",
				"--app-slug", "claude-code",
				"--agentapi-url", agentapiURL)
			stdout := ptytest.New(t)
			inv.Stdout = stdout.Output()
			stderr := ptytest.New(t)
			inv.Stderr = stderr.Output()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			t.Cleanup(cancel)

			go func() {
				err := inv.WithContext(ctx).Run()
				assert.NoError(t, err)
			}()

			// Should only try to update the status if we got one.
			if test.status == nil {
				stderr.ExpectMatch("failed to fetch status")
			} else {
				got := <-done
				require.Equal(t, got, test.expected)
			}

			// Non-nil for the update means there was an error.
			if test.resp != nil {
				stderr.ExpectMatch("failed to update status")
			}
		})
	}
}
