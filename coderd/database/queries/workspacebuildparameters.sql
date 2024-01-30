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

-- name: GetUserWorkspaceBuildParameters :many
-- name: GetUserWorkspaceBuildParameters :many
SELECT DISTINCT ON (tvp.name)
    tvp.name,
    wbp.value
FROM
    workspace_build_parameters wbp
JOIN
    workspace_builds wb ON wb.id = wbp.workspace_build_id
JOIN
    workspaces w ON w.id = wb.workspace_id
JOIN
    template_version_parameters tvp ON tvp.template_version_id = wb.template_version_id
WHERE
    w.owner_id = $1
    AND wb.transition = 'start'
    AND w.template_id = $2
    AND tvp.ephemeral = false
    AND tvp.name = wbp.name
ORDER BY
    tvp.name, wb.created_at DESC
LIMIT 100;

