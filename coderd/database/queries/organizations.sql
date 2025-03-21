-- name: GetDefaultOrganization :one
SELECT
    *
FROM
    organizations
WHERE
    is_default = true
LIMIT
    1;

-- name: GetOrganizations :many
SELECT
    *
FROM
    organizations
WHERE
    -- Optionally include deleted organizations
    deleted = @deleted
      -- Filter by ids
    AND CASE
        WHEN array_length(@ids :: uuid[], 1) > 0 THEN
            id = ANY(@ids)
        ELSE true
    END
    AND CASE
          WHEN @name::text != '' THEN
              LOWER("name") = LOWER(@name)
          ELSE true
    END
;

-- name: GetOrganizationByID :one
SELECT
    *
FROM
    organizations
WHERE
    id = $1;

-- name: GetOrganizationByName :one
SELECT
    *
FROM
    organizations
WHERE
    -- Optionally include deleted organizations
    deleted = @deleted AND
    LOWER("name") = LOWER(@name)
LIMIT
    1;

-- name: GetOrganizationsByUserID :many
SELECT
    *
FROM
    organizations
WHERE
    -- Optionally include deleted organizations
    deleted = @deleted AND
    id = ANY(
        SELECT
            organization_id
        FROM
            organization_members
        WHERE
            user_id = $1
    );

-- name: GetOrganizationResourcesCountById :one
SELECT
    (SELECT COUNT(*) FROM workspaces WHERE workspaces.organization_id = $1 AND deleted = false) AS workspace_count,
    (SELECT COUNT(*) FROM groups WHERE groups.organization_id = $1 AND deleted = false) AS group_count,
    (SELECT COUNT(*) FROM templates WHERE templates.organization_id = $1 AND deleted = false) AS template_count,
    (SELECT COUNT(*) FROM organization_members WHERE organization_members.organization_id = $1) AS member_count,
    (SELECT COUNT(*) FROM provisioner_keys WHERE provisioner_keys.organization_id = $1) AS provisioner_key_count;

-- name: InsertOrganization :one
INSERT INTO
    organizations (id, "name", display_name, description, icon, created_at, updated_at, is_default)
VALUES
    -- If no organizations exist, and this is the first, make it the default.
    (@id, @name, @display_name, @description, @icon, @created_at, @updated_at, (SELECT TRUE FROM organizations LIMIT 1) IS NULL) RETURNING *;

-- name: UpdateOrganization :one
UPDATE
    organizations
SET
    updated_at = @updated_at,
    name = @name,
    display_name = @display_name,
    description = @description,
    icon = @icon
WHERE
    id = @id
RETURNING *;

-- name: UpdateOrganizationDeletedByID :exec
UPDATE organizations
SET
    deleted = true,
    updated_at = @updated_at
WHERE
    id = @id AND
    is_default = false;

