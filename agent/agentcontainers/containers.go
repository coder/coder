package agentcontainers

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	defaultGetContainersCacheDuration = 10 * time.Second
	dockerCreatedAtTimeFormat         = "2006-01-02 15:04:05 -0700 MST"
	getContainersTimeout              = 5 * time.Second
)

type devcontainersHandler struct {
	cacheDuration time.Duration
	cl            Lister
	clock         quartz.Clock

	initLockOnce sync.Once // ensures we don't get a race when initializing lockCh
	// lockCh protects the below fields. We use a channel instead of a mutex so we
	// can handle cancellation properly.
	lockCh     chan struct{}
	containers *codersdk.WorkspaceAgentListContainersResponse
	mtime      time.Time
}

// Option is a functional option for devcontainersHandler.
type Option func(*devcontainersHandler)

// WithLister sets the agentcontainers.Lister implementation to use.
// The default implementation uses the Docker CLI to list containers.
func WithLister(cl Lister) Option {
	return func(ch *devcontainersHandler) {
		ch.cl = cl
	}
}

// New returns a new devcontainersHandler with the given options applied.
func New(options ...Option) http.Handler {
	ch := &devcontainersHandler{}
	for _, opt := range options {
		opt(ch)
	}
	return ch
}

func (ch *devcontainersHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
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

func (ch *devcontainersHandler) getContainers(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	ch.initLockOnce.Do(func() {
		if ch.lockCh == nil {
			ch.lockCh = make(chan struct{}, 1)
		}
	})
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
