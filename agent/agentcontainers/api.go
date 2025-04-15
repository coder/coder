package agentcontainers

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	defaultGetContainersCacheDuration = 10 * time.Second
	dockerCreatedAtTimeFormat         = "2006-01-02 15:04:05 -0700 MST"
	getContainersTimeout              = 5 * time.Second
)

// API is responsible for container-related operations in the agent.
// It provides methods to list and manage containers.
type API struct {
	cacheDuration time.Duration
	cl            Lister
	dccli         DevcontainerCLI
	clock         quartz.Clock

	// lockCh protects the below fields. We use a channel instead of a mutex so we
	// can handle cancellation properly.
	lockCh     chan struct{}
	containers codersdk.WorkspaceAgentListContainersResponse
	mtime      time.Time
}

// Option is a functional option for API.
type Option func(*API)

// WithLister sets the agentcontainers.Lister implementation to use.
// The default implementation uses the Docker CLI to list containers.
func WithLister(cl Lister) Option {
	return func(api *API) {
		api.cl = cl
	}
}

func WithDevcontainerCLI(dccli DevcontainerCLI) Option {
	return func(api *API) {
		api.dccli = dccli
	}
}

// NewAPI returns a new API with the given options applied.
func NewAPI(logger slog.Logger, options ...Option) *API {
	api := &API{
		clock:         quartz.NewReal(),
		cacheDuration: defaultGetContainersCacheDuration,
		lockCh:        make(chan struct{}, 1),
	}
	for _, opt := range options {
		opt(api)
	}
	if api.cl == nil {
		api.cl = &DockerCLILister{}
	}
	if api.dccli == nil {
		api.dccli = NewDevcontainerCLI(logger, agentexec.DefaultExecer)
	}

	return api
}

// Routes returns the HTTP handler for container-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", api.handleList)
	r.Post("/{id}/recreate", api.handleRecreate)
	return r
}

// handleList handles the HTTP request to list containers.
func (api *API) handleList(rw http.ResponseWriter, r *http.Request) {
	select {
	case <-r.Context().Done():
		// Client went away.
		return
	default:
		ct, err := api.getContainers(r.Context())
		if err != nil {
			if errors.Is(err, context.Canceled) {
				httpapi.Write(r.Context(), rw, http.StatusRequestTimeout, codersdk.Response{
					Message: "Could not get containers.",
					Detail:  "Took too long to list containers.",
				})
				return
			}
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Could not get containers.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(r.Context(), rw, http.StatusOK, ct)
	}
}

func copyListContainersResponse(resp codersdk.WorkspaceAgentListContainersResponse) codersdk.WorkspaceAgentListContainersResponse {
	return codersdk.WorkspaceAgentListContainersResponse{
		Containers: slices.Clone(resp.Containers),
		Warnings:   slices.Clone(resp.Warnings),
	}
}

func (api *API) getContainers(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	select {
	case <-ctx.Done():
		return codersdk.WorkspaceAgentListContainersResponse{}, ctx.Err()
	default:
		api.lockCh <- struct{}{}
	}
	defer func() {
		<-api.lockCh
	}()

	now := api.clock.Now()
	if now.Sub(api.mtime) < api.cacheDuration {
		return copyListContainersResponse(api.containers), nil
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, getContainersTimeout)
	defer timeoutCancel()
	updated, err := api.cl.List(timeoutCtx)
	if err != nil {
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("get containers: %w", err)
	}
	api.containers = updated
	api.mtime = now

	return copyListContainersResponse(api.containers), nil
}

// handleRecreate handles the HTTP request to recreate a container.
func (api *API) handleRecreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if id == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing container ID or name",
			Detail:  "Container ID or name is required to recreate a devcontainer.",
		})
		return
	}

	containers, err := api.cl.List(ctx)
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not list containers",
			Detail:  err.Error(),
		})
		return
	}

	containerIdx := slices.IndexFunc(containers.Containers, func(c codersdk.WorkspaceAgentContainer) bool {
		return c.Match(id)
	})
	if containerIdx == -1 {
		httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
			Message: "Container not found",
			Detail:  "Container ID or name not found in the list of containers.",
		})
		return
	}

	container := containers.Containers[containerIdx]
	workspaceFolder := container.Labels[DevcontainerLocalFolderLabel]
	configPath := container.Labels[DevcontainerConfigFileLabel]

	// Workspace folder is required to recreate a container, we don't verify
	// the config path here because it's optional.
	if workspaceFolder == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing workspace folder label",
			Detail:  "The workspace folder label is required to recreate a devcontainer.",
		})
		return
	}

	_, err = api.dccli.Up(ctx, workspaceFolder, configPath, WithRemoveExistingContainer())
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not recreate devcontainer",
			Detail:  err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
