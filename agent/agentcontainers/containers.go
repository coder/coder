package agentcontainers

import (
	"context"

	"github.com/coder/coder/v2/codersdk"
)

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
