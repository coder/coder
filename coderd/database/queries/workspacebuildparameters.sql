-- name: InsertWorkspaceBuildParameters :exec
INSERT INTO
    workspace_build_parameters (workspace_build_id, name, value, sensitive, value_key_id)
SELECT
    @workspace_build_id :: uuid AS workspace_build_id,
    unnest(@name :: text[]) AS name,
    unnest(@value :: text[]) AS value,
    unnest(@sensitive :: boolean[]) AS sensitive,
    -- Empty string means not encrypted; the column has a foreign key to
    -- dbcrypt_keys, so it must be NULL rather than ''.
    NULLIF(unnest(@value_key_id :: text[]), '') AS value_key_id
RETURNING *;

-- name: UpdateWorkspaceBuildParameterValue :exec
-- Rewrites a sensitive parameter value in place and marks the parameter
-- sensitive. Used by `coder server dbcrypt` maintenance commands to
-- re-encrypt or decrypt sensitive values at rest.
UPDATE
    workspace_build_parameters
SET
    value = @value,
    value_key_id = sqlc.narg('value_key_id'),
    sensitive = TRUE
WHERE
    workspace_build_id = @workspace_build_id
    AND name = @name;

-- name: GetWorkspaceBuildParameters :many
SELECT
    *
FROM
    workspace_build_parameters
WHERE
    workspace_build_id = $1;

-- name: GetWorkspaceBuildParametersByBuildIDsAndNames :many
-- Matches parameters whose name is in names or starts with one of
-- name_prefixes. starts_with is used instead of LIKE so an underscore in a
-- prefix is matched literally rather than as a wildcard.
SELECT
    *
FROM
    workspace_build_parameters
WHERE
    workspace_build_id = ANY(@workspace_build_ids :: uuid[])
    AND (
        name = ANY(@names :: text[])
        OR EXISTS (
            SELECT 1 FROM unnest(@name_prefixes :: text[]) AS prefix
            WHERE starts_with(name, prefix)
        )
    );

-- name: GetSensitiveWorkspaceBuildParameters :many
-- Returns a page of workspace build parameters that hold a sensitive value
-- across the deployment, ordered by workspace build id and name. Pass the
-- workspace build id and name of the last returned row to fetch the next
-- page. A parameter is listed when it is marked sensitive, or when its
-- template version declares it ephemeral, which covers rows written before
-- the sensitive column existed. Used only by the dbcrypt key rotation
-- utility.
SELECT
    workspace_build_parameters.*
FROM
    workspace_build_parameters
WHERE
    workspace_build_parameters.value != ''
    AND (
        workspace_build_parameters.sensitive
        OR EXISTS (
            SELECT
                1
            FROM
                workspace_builds
            JOIN
                template_version_parameters
            ON
                template_version_parameters.template_version_id = workspace_builds.template_version_id
            WHERE
                workspace_builds.id = workspace_build_parameters.workspace_build_id
                AND template_version_parameters.name = workspace_build_parameters.name
                AND template_version_parameters.ephemeral
        )
    )
    AND (workspace_build_parameters.workspace_build_id, workspace_build_parameters.name) > (@after_workspace_build_id::uuid, @after_name::text)
ORDER BY
    workspace_build_parameters.workspace_build_id,
    workspace_build_parameters.name
LIMIT
    @limit_count::int;
