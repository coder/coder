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
SELECT
    sub.name,
    sub.value
FROM (
    SELECT
        wbp.name,
        wbp.value,
        wb.created_at,
        ROW_NUMBER() OVER (PARTITION BY wbp.name ORDER BY wb.created_at DESC) as rn
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
) sub
WHERE
    sub.rn = 1
ORDER BY sub.created_at DESC
-- If there are many distinct parameters, 
-- we only want the most recent ones. 
LIMIT 100;
