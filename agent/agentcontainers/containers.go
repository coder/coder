package agentcontainers

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"golang.org/x/xerrors"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	defaultGetContainersCacheDuration = 10 * time.Second
	dockerCreatedAtTimeFormat         = "2006-01-02 15:04:05 -0700 MST"
	getContainersTimeout              = 5 * time.Second
)

type Handler struct {
	cacheDuration time.Duration
	cl            Lister
	dccli         DevcontainerCLI
	clock         quartz.Clock

	// lockCh protects the below fields. We use a channel instead of a mutex so we
	// can handle cancellation properly.
	lockCh     chan struct{}
	containers *codersdk.WorkspaceAgentListContainersResponse
	mtime      time.Time
}

// Option is a functional option for Handler.
type Option func(*Handler)

// WithLister sets the agentcontainers.Lister implementation to use.
// The default implementation uses the Docker CLI to list containers.
func WithLister(cl Lister) Option {
	return func(ch *Handler) {
		ch.cl = cl
	}
}

func WithDevcontainerCLI(dccli DevcontainerCLI) Option {
	return func(ch *Handler) {
		ch.dccli = dccli
	}
}

// New returns a new Handler with the given options applied.
func New(options ...Option) *Handler {
	ch := &Handler{
		lockCh: make(chan struct{}, 1),
	}
	for _, opt := range options {
		opt(ch)
	}
	return ch
}

func (ch *Handler) List(rw http.ResponseWriter, r *http.Request) {
	select {
	case <-r.Context().Done():
		// Client went away.
		return
	default:
		ct, err := ch.getContainers(r.Context())
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

func (ch *Handler) getContainers(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	select {
	case <-ctx.Done():
		return codersdk.WorkspaceAgentListContainersResponse{}, ctx.Err()
	default:
		ch.lockCh <- struct{}{}
	}
	defer func() {
		<-ch.lockCh
	}()

	// make zero-value usable
	if ch.cacheDuration == 0 {
		ch.cacheDuration = defaultGetContainersCacheDuration
	}
	if ch.cl == nil {
		ch.cl = &DockerCLILister{}
	}
	if ch.containers == nil {
		ch.containers = &codersdk.WorkspaceAgentListContainersResponse{}
	}
	if ch.clock == nil {
		ch.clock = quartz.NewReal()
	}

	now := ch.clock.Now()
	if now.Sub(ch.mtime) < ch.cacheDuration {
		// Return a copy of the cached data to avoid accidental modification by the caller.
		cpy := codersdk.WorkspaceAgentListContainersResponse{
			Containers: slices.Clone(ch.containers.Containers),
			Warnings:   slices.Clone(ch.containers.Warnings),
		}
		return cpy, nil
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, getContainersTimeout)
	defer timeoutCancel()
	updated, err := ch.cl.List(timeoutCtx)
	if err != nil {
		return codersdk.WorkspaceAgentListContainersResponse{}, xerrors.Errorf("get containers: %w", err)
	}
	ch.containers = &updated
	ch.mtime = now

	// Return a copy of the cached data to avoid accidental modification by the
	// caller.
	cpy := codersdk.WorkspaceAgentListContainersResponse{
		Containers: slices.Clone(ch.containers.Containers),
		Warnings:   slices.Clone(ch.containers.Warnings),
	}
	return cpy, nil
}

// Lister is an interface for listing containers visible to the
// workspace agent.
type Lister interface {
	// List returns a list of containers visible to the workspace agent.
	// This should include running and stopped containers.
	List(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error)
}

// NoopLister is a Lister interface that never returns any containers.
type NoopLister struct{}

var _ Lister = NoopLister{}

func (NoopLister) List(_ context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	return codersdk.WorkspaceAgentListContainersResponse{}, nil
}

func (ch *Handler) Recreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if id == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing container ID or name",
			Detail:  "Container ID or name is required to recreate a devcontainer.",
		})
		return
	}

	containers, err := ch.cl.List(ctx)
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

	_, err = ch.dccli.Up(ctx, workspaceFolder, configPath, WithRemoveExistingContainer())
	if err != nil {
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not recreate devcontainer",
			Detail:  err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
