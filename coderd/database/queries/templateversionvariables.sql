-- name: InsertTemplateVersionVariable :one
INSERT INTO
    template_version_variables (
        template_version_id,
        name,
        description,
        type,
        value,
        default_value,
        required,
        sensitive
    )
VALUES
    (
        $1,
        $2,
        $3,
        $4,
        $5,
        $6,
        $7,
        $8
    ) RETURNING *;

-- name: GetTemplateVersionVariables :many
SELECT * FROM template_version_variables WHERE template_version_id = $1;
