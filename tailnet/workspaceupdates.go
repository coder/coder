package tailnet

import (
	"context"
	"io"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet/proto"
)

type WorkspaceUpdatesProvider interface {
	io.Closer
	Subscribe(ctx context.Context, userID uuid.UUID, db UpdateQuerier) (Subscription, error)
}

type Subscription interface {
	io.Closer
	Updates() <-chan *proto.WorkspaceUpdate
}

type UpdateQuerier interface {
	GetWorkspacesAndAgents(ctx context.Context, ownerID uuid.UUID) ([]WorkspacesAndAgents, error)
	AuthorizeTunnel(ctx context.Context, agentID uuid.UUID) error
}

type WorkspacesAndAgents struct {
	ID         uuid.UUID
	Name       string
	JobStatus  codersdk.ProvisionerJobStatus
	Transition codersdk.WorkspaceTransition
	Agents     []AgentIDNamePair
}

type AgentIDNamePair struct {
	ID   uuid.UUID
	Name string
}
