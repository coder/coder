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

-- name: GetTemplateVersionIDsWithSensitiveVariables :many
-- Returns a page of template versions that have at least one sensitive
-- variable with a value across the deployment, ordered by id. Pass the last
-- returned id as after_id to fetch the next page. Used only by the dbcrypt
-- key rotation utility.
SELECT DISTINCT
    template_version_id
FROM
    template_version_variables
WHERE
    sensitive
    AND value != ''
    AND template_version_id > @after_id::uuid
ORDER BY
    template_version_id
LIMIT
    @limit_count::int;
