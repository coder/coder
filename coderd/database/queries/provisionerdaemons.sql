-- name: GetProvisionerDaemons :many
SELECT
	*
FROM
	provisioner_daemons;

-- name: InsertProvisionerDaemon :one
INSERT INTO
	provisioner_daemons (
		id,
		created_at,
		"name",
		provisioners,
		tags,
		updated_at
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: DeleteOldProvisionerDaemons :exec
-- Delete provisioner daemons that have been created at least a week ago
-- and have not connected to coderd since a week.
-- A provisioner daemon with "zeroed" updated_at column indicates possible
-- connectivity issues (no provisioner daemon activity since registration).
DELETE FROM provisioner_daemons WHERE (
	(created_at < (NOW() - INTERVAL '7 days') AND updated_at IS NULL) OR
	(updated_at IS NOT NULL AND updated_at < (NOW() - INTERVAL '7 days'))
);
