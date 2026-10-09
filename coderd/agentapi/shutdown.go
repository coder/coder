package agentapi

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/emptypb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

// WorkspaceShutdownAPI provides shutdown context for the authenticated agent.
type WorkspaceShutdownAPI struct {
	WorkspaceID uuid.UUID
	Database    database.Store
}

// GetWorkspaceShutdown returns the latest running or successful stop/delete
// build. An empty response means no workspace shutdown is known.
func (a *WorkspaceShutdownAPI) GetWorkspaceShutdown(ctx context.Context, _ *emptypb.Empty) (*agentproto.WorkspaceShutdown, error) {
	build, err := a.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, a.WorkspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get latest workspace build: %w", err)
	}
	if build.Transition != database.WorkspaceTransitionStop && build.Transition != database.WorkspaceTransitionDelete {
		return &agentproto.WorkspaceShutdown{}, nil
	}
	job, err := a.Database.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return nil, xerrors.Errorf("get shutdown provisioner job: %w", err)
	}
	// A successful build is usable only while the existing connection survives;
	// the next monitor check retires it. Later shutdowns use server_shutdown.
	// Pending, failed, or canceled jobs cannot establish a shutdown cause.
	if job.JobStatus != database.ProvisionerJobStatusRunning && job.JobStatus != database.ProvisionerJobStatusSucceeded {
		return &agentproto.WorkspaceShutdown{}, nil
	}
	return &agentproto.WorkspaceShutdown{
		BuildId: build.ID.String(), Transition: string(build.Transition), BuildReason: string(build.Reason),
	}, nil
}
