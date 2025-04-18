-- name: InsertTemplateVersionParameter :one
INSERT INTO
    template_version_parameters (
        template_version_id,
        name,
        description,
        type,
        form_type,
        mutable,
        default_value,
        icon,
        options,
        validation_regex,
        validation_min,
        validation_max,
        validation_error,
        validation_monotonic,
        required,
        display_name,
        display_order,
        ephemeral
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
        $13,
        $14,
        $15,
        $16,
        $17,
        $18
    ) RETURNING *;

-- name: GetTemplateVersionParameters :many
SELECT * FROM template_version_parameters WHERE template_version_id = $1 ORDER BY display_order ASC, LOWER(name) ASC;
