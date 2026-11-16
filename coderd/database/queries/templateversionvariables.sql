-- name: InsertTemplateVersionVariable :one
INSERT INTO
    template_version_variables (
        template_version_id,
        name,
        description,
        type,
        value,
        value_key_id,
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
        sqlc.narg('value_key_id'),
        $6,
        $7,
        $8
    ) RETURNING *;

-- name: GetTemplateVersionVariables :many
SELECT * FROM template_version_variables WHERE template_version_id = $1 ORDER BY name;

-- name: DeleteTemplateVersionVariablesByTemplateVersionID :exec
-- Deletes all variables of a template version. Import jobs use this to
-- replace the placeholder rows that carried user-provided values (kept out
-- of the plaintext job input; SEC-1491) with the variables parsed from the
-- template.
DELETE FROM template_version_variables WHERE template_version_id = @template_version_id;

-- name: UpdateTemplateVersionVariableValue :exec
-- Rewrites a variable value in place. Used by `coder server dbcrypt`
-- maintenance commands to re-encrypt or decrypt sensitive values at rest.
UPDATE
    template_version_variables
SET
    value = @value,
    value_key_id = sqlc.narg('value_key_id')
WHERE
    template_version_id = @template_version_id
    AND name = @name;
