package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (q *querier) InsertWorkspaceExecutionArtifact(ctx context.Context, arg database.InsertWorkspaceExecutionArtifactParams) (int64, error) {
	session, err := q.db.GetWorkspaceExecutionSessionByID(ctx, arg.SessionID)
	if err != nil {
		return 0, err
	}
	object := rbac.ResourceWorkspaceExecution.WithID(session.ID).WithOwner(session.OwnerID.String()).InOrg(session.OrganizationID)
	if err := q.authorizeContext(ctx, policy.ActionSSH, object); err != nil {
		return 0, err
	}
	return q.db.InsertWorkspaceExecutionArtifact(ctx, arg)
}

func (q *querier) GetWorkspaceExecutionArtifactsBySessionID(ctx context.Context, id uuid.UUID) ([]database.GetWorkspaceExecutionArtifactsBySessionIDRow, error) {
	return fetchWithPostFilter(q.auth, policy.ActionRead, q.db.GetWorkspaceExecutionArtifactsBySessionID)(ctx, id)
}

func (q *querier) ReadWorkspaceExecutionArtifact(ctx context.Context, arg database.ReadWorkspaceExecutionArtifactParams) (database.ReadWorkspaceExecutionArtifactRow, error) {
	return fetchWithAction(q.log, q.auth, policy.ActionSSH, q.db.ReadWorkspaceExecutionArtifact)(ctx, arg)
}
