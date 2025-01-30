package agent

//go:generate mockgen -destination ./containers_mock.go -package agent . ContainerLister

import (
	"context"
	"net/http"
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

type containersHandler struct {
	cacheDuration time.Duration
	cl            ContainerLister
	clock         quartz.Clock

	mu         sync.Mutex // protects the below
	containers []codersdk.WorkspaceAgentContainer
	mtime      time.Time
}

func (ch *containersHandler) handler(rw http.ResponseWriter, r *http.Request) {
	ct, err := ch.getContainers(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not get containers.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, ct)
}

func (ch *containersHandler) getContainers(ctx context.Context) ([]codersdk.WorkspaceAgentContainer, error) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// make zero-value usable
	if ch.cacheDuration == 0 {
		ch.cacheDuration = defaultGetContainersCacheDuration
	}
	if ch.cl == nil {
		// TODO(cian): we may need some way to select the desired
		// implementation, but for now there is only one.
		ch.cl = &dockerCLIContainerLister{}
	}
	if ch.containers == nil {
		ch.containers = make([]codersdk.WorkspaceAgentContainer, 0)
	}
	if ch.clock == nil {
		ch.clock = quartz.NewReal()
	}

	now := ch.clock.Now()
	if now.Sub(ch.mtime) < ch.cacheDuration {
		cpy := make([]codersdk.WorkspaceAgentContainer, len(ch.containers))
		copy(cpy, ch.containers)
		return cpy, nil
	}

	cancelCtx, cancelFunc := context.WithTimeout(ctx, getContainersTimeout)
	defer cancelFunc()
	updated, err := ch.cl.List(cancelCtx)
	if err != nil {
		return nil, xerrors.Errorf("get containers: %w", err)
	}
	ch.containers = updated
	ch.mtime = now

	// return a copy
	cpy := make([]codersdk.WorkspaceAgentContainer, len(ch.containers))
	copy(cpy, ch.containers)
	return cpy, nil
}

type ContainerLister interface {
	List(ctx context.Context) ([]codersdk.WorkspaceAgentContainer, error)
}
