package agentcontainers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/codersdk"
)

// fakeLister implements the agentcontainers.Lister interface for
// testing.
type fakeLister struct {
	containers codersdk.WorkspaceAgentListContainersResponse
	err        error
}

func (f *fakeLister) List(_ context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	return f.containers, f.err
}

// fakeDevcontainerCLI implements the agentcontainers.DevcontainerCLI
// interface for testing.
type fakeDevcontainerCLI struct {
	id  string
	err error
}

func (f *fakeDevcontainerCLI) Up(_ context.Context, _, _ string, _ ...agentcontainers.DevcontainerCLIUpOptions) (string, error) {
	return f.id, f.err
}

func TestAPI(t *testing.T) {
	t.Parallel()

	t.Run("Recreate", func(t *testing.T) {
		t.Parallel()

		validContainer := codersdk.WorkspaceAgentContainer{
			ID:           "container-id",
			FriendlyName: "container-name",
			Labels: map[string]string{
				agentcontainers.DevcontainerLocalFolderLabel: "/workspace",
				agentcontainers.DevcontainerConfigFileLabel:  "/workspace/.devcontainer/devcontainer.json",
			},
		}

		missingFolderContainer := codersdk.WorkspaceAgentContainer{
			ID:           "missing-folder-container",
			FriendlyName: "missing-folder-container",
			Labels:       map[string]string{},
		}

		tests := []struct {
			name            string
			containerID     string
			lister          *fakeLister
			devcontainerCLI *fakeDevcontainerCLI
			wantStatus      int
			wantBody        string
		}{
			{
				name:            "Missing ID",
				containerID:     "",
				lister:          &fakeLister{},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusBadRequest,
				wantBody:        "Missing container ID or name",
			},
			{
				name:        "List error",
				containerID: "container-id",
				lister: &fakeLister{
					err: xerrors.New("list error"),
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusInternalServerError,
				wantBody:        "Could not list containers",
			},
			{
				name:        "Container not found",
				containerID: "nonexistent-container",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusNotFound,
				wantBody:        "Container not found",
			},
			{
				name:        "Missing workspace folder label",
				containerID: "missing-folder-container",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{missingFolderContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusBadRequest,
				wantBody:        "Missing workspace folder label",
			},
			{
				name:        "Devcontainer CLI error",
				containerID: "container-id",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{
					err: xerrors.New("devcontainer CLI error"),
				},
				wantStatus: http.StatusInternalServerError,
				wantBody:   "Could not recreate devcontainer",
			},
			{
				name:        "OK",
				containerID: "container-id",
				lister: &fakeLister{
					containers: codersdk.WorkspaceAgentListContainersResponse{
						Containers: []codersdk.WorkspaceAgentContainer{validContainer},
					},
				},
				devcontainerCLI: &fakeDevcontainerCLI{},
				wantStatus:      http.StatusNoContent,
				wantBody:        "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

				// Setup router with the handler under test.
				r := chi.NewRouter()
				api := agentcontainers.NewAPI(
					logger,
					agentcontainers.WithLister(tt.lister),
					agentcontainers.WithDevcontainerCLI(tt.devcontainerCLI),
				)
				r.Mount("/containers", api.Routes())

				// Simulate HTTP request to the recreate endpoint.
				req := httptest.NewRequest(http.MethodPost, "/containers/"+tt.containerID+"/recreate", nil)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				// Check the response status code and body.
				require.Equal(t, tt.wantStatus, rec.Code, "status code mismatch")
				if tt.wantBody != "" {
					assert.Contains(t, rec.Body.String(), tt.wantBody, "response body mismatch")
				} else if tt.wantStatus == http.StatusNoContent {
					assert.Empty(t, rec.Body.String(), "expected empty response body")
				}
			})
		}
	})
}
