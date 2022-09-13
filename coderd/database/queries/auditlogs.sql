-- GetAuditLogsBefore retrieves `row_limit` number of audit logs before the provided
-- ID.
-- name: GetAuditLogsOffset :many
SELECT
	audit_logs.*,
    users.username AS user_username,
    users.email AS user_email,
    users.created_at AS user_created_at,
    users.status AS user_status,
    users.rbac_roles AS user_roles,
    users.avatar_url AS user_avatar_url
FROM
	audit_logs
LEFT JOIN
    users ON audit_logs.user_id = users.id
ORDER BY
    "time" DESC
LIMIT
    $1
OFFSET
    $2;

-- name: GetAuditLogCount :one
SELECT
    COUNT(*) as count
FROM
    audit_logs;

-- name: InsertAuditLog :one
INSERT INTO
	audit_logs (
        id,
        "time",
        user_id,
        organization_id,
        ip,
        user_agent,
        resource_type,
        resource_id,
        resource_target,
        action,
        diff,
        status_code,
        additional_fields,
        request_id,
        resource_icon
    )
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING *;
