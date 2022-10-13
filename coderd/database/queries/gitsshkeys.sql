-- name: InsertGitSSHKey :one
INSERT INTO
	gitsshkeys (
		user_id,
		created_at,
		updated_at,
		private_key,
		public_key
	)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- name: GetGitSSHKey :one
SELECT
	*
FROM
	gitsshkeys
WHERE
	user_id = $1;

-- name: UpdateGitSSHKey :one
UPDATE
	gitsshkeys
SET
	updated_at = $2,
	private_key = $3,
	public_key = $4
WHERE
	user_id = $1
RETURNING
	*;

-- name: DeleteGitSSHKey :exec
DELETE FROM
	gitsshkeys
WHERE
	user_id = $1;
