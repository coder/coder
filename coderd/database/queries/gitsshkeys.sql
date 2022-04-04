-- name: InsertGitSSHKey :one
INSERT INTO
	git_ssh_keys (
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
	git_ssh_keys
WHERE
	user_id = $1;

-- name: UpdateGitSSHKey :exec
UPDATE
	git_ssh_keys
SET
	updated_at = $2,
	private_key = $3,
	public_key = $4
WHERE
	user_id = $1;

-- name: DeleteGitSSHKey :exec
DELETE FROM
	git_ssh_keys
WHERE
	user_id = $1;
