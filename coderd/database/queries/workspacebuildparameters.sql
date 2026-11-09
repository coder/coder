-- name: InsertWorkspaceBuildParameters :exec
INSERT INTO
    workspace_build_parameters (workspace_build_id, name, value)
SELECT
    @workspace_build_id :: uuid AS workspace_build_id,
    unnest(@name :: text[]) AS name,
    unnest(@value :: text[]) AS value
RETURNING *;

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
