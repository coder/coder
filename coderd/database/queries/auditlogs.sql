-- GetAuditLogsBefore retrieves `limit` number of audit logs before the provided
-- ID.
-- name: GetAuditLogsBefore :many
SELECT
	*
FROM
	audit_logs
WHERE
	"time" < (SELECT "time" FROM audit_logs a WHERE a.id = sqlc.arg(id))
ORDER BY
    "time" DESC
LIMIT
	sqlc.arg(row_limit);

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
        status_code
    )
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;
