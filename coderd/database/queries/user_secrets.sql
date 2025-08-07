-- GetUserSecret - Get by user_id and name
-- GetUserSecretByID - Get by ID
-- ListUserSecrets - List all secrets for a user
-- CreateUserSecret - Create new secret
-- UpdateUserSecret - Update existing secret
-- DeleteUserSecret - Delete by user_id and name
-- DeleteUserSecretByID - Delete by ID

-- name: GetUserSecret :one
SELECT * FROM user_secrets
WHERE user_id = @user_id AND name = @name;

-- name: ListUserSecrets :many
SELECT * FROM user_secrets
WHERE user_id = @user_id;

-- name: InsertUserSecret :one
INSERT INTO user_secrets (
	id,
	user_id,
	name,
	description,
	value,
	value_key_id,
	env_name,
	file_path
)
VALUES (
	@id,
	@user_id,
	@name,
	@description,
	@value,
	@value_key_id,
	@env_name,
	@file_path
) RETURNING *;
