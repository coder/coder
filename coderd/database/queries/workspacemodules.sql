-- name: InsertWorkspaceModule :one
INSERT INTO
	workspace_modules (id, job_id, transition, source, version, key, created_at)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;
