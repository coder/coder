-- name: InsertBoundaryActiveUser :exec
INSERT INTO boundary_active_users (user_id, recorded_at)
VALUES ($1, $2);

-- name: InsertBoundaryActiveWorkspace :exec
INSERT INTO boundary_active_workspaces (workspace_id, template_id, recorded_at)
VALUES ($1, $2, $3);

-- name: GetBoundaryActiveUsersSince :many
SELECT DISTINCT user_id FROM boundary_active_users
WHERE recorded_at > $1;

-- name: GetBoundaryActiveWorkspacesSince :many
SELECT DISTINCT workspace_id, template_id FROM boundary_active_workspaces
WHERE recorded_at > $1;

-- name: DeleteBoundaryActiveUsersBefore :exec
DELETE FROM boundary_active_users WHERE recorded_at < $1;

-- name: DeleteBoundaryActiveWorkspacesBefore :exec
DELETE FROM boundary_active_workspaces WHERE recorded_at < $1;
