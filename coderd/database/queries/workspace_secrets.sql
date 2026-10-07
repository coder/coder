-- name: ListActiveWorkspaceSecrets :many
-- Returns the live rows (value not yet cleared) linked to a build. Only the
-- latest build of a workspace has live rows, so an older build returns none.
-- Includes decrypted values, so this is used only by the agent manifest and
-- by the build transaction that copies secrets forward; there is no REST
-- endpoint that reads workspace secrets.
SELECT *
FROM workspace_secrets
WHERE workspace_build_id = @workspace_build_id
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
-- Drops the values of every live row that belongs to an earlier build of the
-- workspace (lower build number than the given build), keeping the rows so
-- the history of which secrets earlier builds received stays inspectable.
-- Rows of the given build and of any later build are left untouched, so a
-- build that clears out of order cannot wipe a newer build's secrets.
UPDATE workspace_secrets
SET
    value        = NULL,
    value_key_id = NULL,
    cleared_at   = CURRENT_TIMESTAMP
FROM workspace_builds
WHERE workspace_secrets.workspace_build_id = workspace_builds.id
  AND workspace_secrets.workspace_id = @workspace_id
  AND workspace_secrets.cleared_at IS NULL
  AND workspace_builds.build_number < (
      SELECT current_build.build_number
      FROM workspace_builds AS current_build
      WHERE current_build.id = @workspace_build_id
  );

-- name: ClearWorkspaceSecretsByWorkspaceID :exec
-- Drops the values of every live row of the workspace, keeping the rows as
-- history. Used when the workspace is deleted.
UPDATE workspace_secrets
SET
    value        = NULL,
    value_key_id = NULL,
    cleared_at   = CURRENT_TIMESTAMP
WHERE workspace_id = @workspace_id
  AND cleared_at IS NULL;

-- name: GetWorkspaceSecrets :many
-- Returns a page of workspace secrets that still hold a value across the
-- deployment, ordered by id. Pass the last returned id as after_id to fetch
-- the next page. Used only by the dbcrypt key rotation utility.
SELECT *
FROM workspace_secrets
WHERE value IS NOT NULL
  AND id > @after_id::uuid
ORDER BY id
LIMIT @limit_count::int;

-- name: UpdateEncryptedWorkspaceSecretValue :one
-- Updates only the encrypted columns on a row. Used by the dbcrypt key
-- rotation utility to re-encrypt or decrypt rows in place.
-- Cleared rows are skipped: they hold no value, and a row cleared between
-- the rotation's list and this update must not have a value written back.
UPDATE workspace_secrets
SET
    value        = @value,
    value_key_id = @value_key_id
WHERE id = @id
  AND cleared_at IS NULL
RETURNING *;

-- name: GetWorkspaceSecretsHistory :many
-- Returns metadata for every workspace secret row of a workspace, including
-- cleared rows, so the secrets each build received can be inspected. Values
-- are never selected. The workspace owner and organization are included for
-- authorization.
SELECT
    ws.id, ws.workspace_id, ws.workspace_build_id, ws.name,
    ws.env_name, ws.file_path, ws.ephemeral, ws.created_at, ws.cleared_at,
    w.owner_id AS workspace_owner_id,
    w.organization_id AS workspace_organization_id
FROM workspace_secrets ws
JOIN workspaces w ON w.id = ws.workspace_id
WHERE ws.workspace_id = @workspace_id
ORDER BY ws.created_at ASC, ws.name ASC;
