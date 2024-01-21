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
    sub.value,
    sub.created_at
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
    WHERE
        w.owner_id = $1
        AND wb.transition = 'start'
) sub
WHERE
    sub.rn = 1
LIMIT 100;
