package authzquery

import "context"

// These are methods that should only be called by a system user.

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
