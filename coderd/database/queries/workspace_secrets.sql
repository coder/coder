-- name: ListActiveWorkspaceSecrets :many
-- Returns the live rows (value not yet cleared) for a workspace, which are
-- always the rows of its latest build. Includes decrypted values, so this is
-- used only by the agent manifest and by the build transaction that copies
-- secrets forward; there is no REST endpoint that reads workspace secrets.
SELECT *
FROM workspace_secrets
WHERE workspace_id = @workspace_id
  AND cleared_at IS NULL
ORDER BY name ASC;

-- name: InsertWorkspaceSecret :one
INSERT INTO workspace_secrets (
    id,
    workspace_id,
    workspace_build_id,
    name,
    value,
    value_key_id,
    env_name,
    file_path,
    ephemeral
) VALUES (
    @id,
    @workspace_id,
    @workspace_build_id,
    @name,
    @value,
    @value_key_id,
    @env_name,
    @file_path,
    @ephemeral
)
RETURNING *;

-- name: ClearWorkspaceSecretsBeforeBuild :exec
-- Drops the values of every live row that does not belong to the given
-- build, keeping the rows so the history of which secrets earlier builds
-- received stays inspectable.
UPDATE workspace_secrets
SET
    value        = NULL,
    value_key_id = NULL,
    cleared_at   = CURRENT_TIMESTAMP
WHERE workspace_id = @workspace_id
  AND workspace_build_id <> @workspace_build_id
  AND cleared_at IS NULL;

-- name: GetWorkspaceSecrets :many
-- Returns every workspace secret that still holds a value across the
-- deployment. Used only by the dbcrypt key rotation utility.
SELECT *
FROM workspace_secrets
WHERE value IS NOT NULL
ORDER BY workspace_id, workspace_build_id, name;

-- name: UpdateEncryptedWorkspaceSecretValue :one
-- Updates only the encrypted columns on a row. Used by the dbcrypt key
-- rotation utility to re-encrypt or decrypt rows in place.
UPDATE workspace_secrets
SET
    value        = @value,
    value_key_id = @value_key_id
WHERE id = @id
RETURNING *;

-- name: GetWorkspaceSecretsHistory :many
-- Returns every workspace secret row for a workspace, including cleared
-- rows, so the secrets each build received can be inspected.
SELECT *
FROM workspace_secrets
WHERE workspace_id = @workspace_id
ORDER BY created_at ASC, name ASC;
