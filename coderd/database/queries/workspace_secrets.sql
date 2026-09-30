-- name: ListWorkspaceSecretsWithValues :many
-- Returns all columns including the secret value. Used only by the agent
-- manifest for runtime injection; there is no REST endpoint that reads
-- workspace secrets back.
SELECT *
FROM workspace_secrets
WHERE workspace_id = @workspace_id
ORDER BY name ASC;

-- name: UpsertWorkspaceSecret :one
-- Sets a workspace secret by name, replacing the value and injection
-- targets if a row with the same name already exists.
INSERT INTO workspace_secrets (
    id,
    workspace_id,
    name,
    value,
    value_key_id,
    env_name,
    file_path,
    updated_by_build_id
) VALUES (
    @id,
    @workspace_id,
    @name,
    @value,
    @value_key_id,
    @env_name,
    @file_path,
    @updated_by_build_id
)
ON CONFLICT (workspace_id, name) DO UPDATE
SET
    value               = EXCLUDED.value,
    value_key_id        = EXCLUDED.value_key_id,
    env_name            = EXCLUDED.env_name,
    file_path           = EXCLUDED.file_path,
    updated_by_build_id = EXCLUDED.updated_by_build_id,
    updated_at          = CURRENT_TIMESTAMP
RETURNING *;

-- name: DeleteWorkspaceSecretByWorkspaceIDAndName :exec
DELETE FROM workspace_secrets
WHERE workspace_id = @workspace_id AND name = @name;

-- name: GetWorkspaceSecrets :many
-- Returns every workspace secret across the deployment. Used only by the
-- dbcrypt key rotation utility.
SELECT *
FROM workspace_secrets
ORDER BY workspace_id, name;

-- name: UpdateEncryptedWorkspaceSecretValue :one
-- Updates only the encrypted columns on a row. Used by the dbcrypt key
-- rotation utility to re-encrypt or decrypt rows in place.
UPDATE workspace_secrets
SET
    value        = @value,
    value_key_id = @value_key_id,
    updated_at   = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING *;
