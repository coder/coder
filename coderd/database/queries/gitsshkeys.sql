-- name: InsertGitSSHKey :one
INSERT INTO
	gitsshkeys (
		user_id,
		created_at,
		updated_at,
		private_key,
		private_key_key_id,
		public_key
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

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
	private_key_key_id = $4,
	public_key = $5
WHERE
	user_id = $1
RETURNING
	*;
