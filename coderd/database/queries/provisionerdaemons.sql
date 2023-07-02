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
		tags
	)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- name: DeleteOldProvisionerDaemons :exec
DELETE FROM
	provisioner_daemons
WHERE
	updated_at < NOW() - INTERVAL '7 days';
