-- name: InsertTemplateVersionParameter :one
INSERT INTO
    template_version_parameters (
        template_version_id,
        name,
        description,
        type,
        mutable,
        default_value,
        icon,
        options,
        validation_regex,
        validation_min,
        validation_max,
        validation_error,
        validation_monotonic
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
        $8,
        $9,
        $10,
        $11,
        $12,
        $13
    ) RETURNING *;

-- name: GetTemplateVersionParameters :many
SELECT * FROM template_version_parameters WHERE template_version_id = $1;
