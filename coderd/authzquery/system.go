package authzquery

import (
	"context"
	"time"

	"github.com/coder/coder/coderd/database"
)

// TODO: @emyrk should we name system functions differently to indicate a user
// cannot call them? Maybe we should have a separate interface for system functions?
// So you'd do `authzQ.System().GetDERPMeshKey(ctx)` or something like that?

func (q *AuthzQuerier) GetDERPMeshKey(ctx context.Context) (string, error) {
	//TODO Implement authz check for system user.
	return q.database.GetDERPMeshKey(ctx)
}

func (q *AuthzQuerier) InsertDERPMeshKey(ctx context.Context, value string) error {
	//TODO Implement authz check for system user.
	return q.InsertDERPMeshKey(ctx, value)
}

func (q *AuthzQuerier) InsertDeploymentID(ctx context.Context, value string) error {
	//TODO Implement authz check for system user.
	return q.InsertDeploymentID(ctx, value)
}

func (q *AuthzQuerier) InsertReplica(ctx context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	//TODO Implement authz check for system user.
	return q.InsertReplica(ctx, arg)
}

func (q *AuthzQuerier) UpdateReplica(ctx context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
	//TODO Implement authz check for system user.
	return q.UpdateReplica(ctx, arg)
}

func (q *AuthzQuerier) DeleteReplicasUpdatedBefore(ctx context.Context, updatedAt time.Time) error {
	//TODO Implement authz check for system user.
	return q.DeleteReplicasUpdatedBefore(ctx, updatedAt)
}

func (q *AuthzQuerier) GetReplicasUpdatedAfter(ctx context.Context, updatedAt time.Time) ([]database.Replica, error) {
	//TODO Implement authz check for system user.
	return q.GetReplicasUpdatedAfter(ctx, updatedAt)
}
