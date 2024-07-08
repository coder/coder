-- name: InsertTemplateVersionWorkspaceTag :one
INSERT INTO
    template_version_workspace_tags (
        template_version_id,
        key,
        value
    )
VALUES
    (
        $1,
        $2,
        $3
    ) RETURNING *;

-- name: GetTemplateVersionWorkspaceTags :many
SELECT * FROM template_version_workspace_tags WHERE template_version_id = $1 ORDER BY LOWER(key) ASC;
