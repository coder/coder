-- name: GetProvisionerDaemonByID :one
SELECT
	*
FROM
	provisioner_daemons
WHERE
	id = $1;

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
		auth_token
	)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- name: UpdateProvisionerDaemonByID :exec
UPDATE
	provisioner_daemons
SET
	updated_at = $2,
	provisioners = $3
WHERE
	id = $1;

-- name: GetProvisionerDaemonByAuthToken :one
SELECT
	*
FROM
	provisioner_daemons
WHERE
	auth_token = $1;
