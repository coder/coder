package agentcontainers

import (
	"context"

	"github.com/coder/coder/v2/codersdk"
)

// ContainerCLI is an interface for interacting with containers in a workspace.
type ContainerCLI interface {
	// List returns a list of containers visible to the workspace agent.
	// This should include running and stopped containers.
	List(ctx context.Context) (codersdk.WorkspaceAgentListContainersResponse, error)
	// DetectArchitecture detects the architecture of a container.
	DetectArchitecture(ctx context.Context, containerName string) (string, error)
	// Copy copies a file from the host to a container.
	Copy(ctx context.Context, containerName, src, dst string) error
	// ExecAs executes a command in a container as a specific user.
	ExecAs(ctx context.Context, containerName, user string, args ...string) ([]byte, error)
	// Stop terminates the container
	Stop(ctx context.Context, containerName string) error
	// Remove removes the container
	Remove(ctx context.Context, containerName string) error
}

// noopContainerCLI is a ContainerCLI that does nothing.
type noopContainerCLI struct{}

var _ ContainerCLI = noopContainerCLI{}

func (noopContainerCLI) List(_ context.Context) (codersdk.WorkspaceAgentListContainersResponse, error) {
	return codersdk.WorkspaceAgentListContainersResponse{}, nil
}

func (noopContainerCLI) DetectArchitecture(_ context.Context, _ string) (string, error) {
	return "<none>", nil
}
func (noopContainerCLI) Copy(_ context.Context, _ string, _ string, _ string) error { return nil }
func (noopContainerCLI) ExecAs(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}
func (noopContainerCLI) Stop(_ context.Context, _ string) error   { return nil }
func (noopContainerCLI) Remove(_ context.Context, _ string) error { return nil }
