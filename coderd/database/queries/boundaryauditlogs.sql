-- name: InsertBoundaryAuditLogs :exec
INSERT INTO boundary_audit_logs (
    id,
    time,
    organization_id,
    workspace_id,
    workspace_owner_id,
    workspace_name,
    agent_id,
    agent_name,
    resource_type,
    resource,
    operation,
    decision
)
SELECT
    unnest(@id::uuid[]),
    unnest(@time::timestamptz[]),
    unnest(@organization_id::uuid[]),
    unnest(@workspace_id::uuid[]),
    unnest(@workspace_owner_id::uuid[]),
    unnest(@workspace_name::text[]),
    unnest(@agent_id::uuid[]),
    unnest(@agent_name::text[]),
    unnest(@resource_type::text[]),
    unnest(@resource::text[]),
    unnest(@operation::text[]),
    unnest(@decision::boundary_audit_decision[]);

-- name: GetBoundaryAuditLogs :many
SELECT
    boundary_audit_logs.*,
    workspace_owner.username AS workspace_owner_username,
    organizations.name AS organization_name,
    organizations.display_name AS organization_display_name,
    organizations.icon AS organization_icon
FROM
    boundary_audit_logs
JOIN users AS workspace_owner ON
    boundary_audit_logs.workspace_owner_id = workspace_owner.id
JOIN organizations ON
    boundary_audit_logs.organization_id = organizations.id
WHERE
    -- Filter by organization_id
    CASE
        WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
            boundary_audit_logs.organization_id = @organization_id
        ELSE true
    END
    -- Filter by workspace_id
    AND CASE
        WHEN @workspace_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
            boundary_audit_logs.workspace_id = @workspace_id
        ELSE true
    END
    -- Filter by workspace_owner_id
    AND CASE
        WHEN @workspace_owner_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
            boundary_audit_logs.workspace_owner_id = @workspace_owner_id
        ELSE true
    END
    -- Filter by decision
    AND CASE
        WHEN @decision :: text != '' THEN
            boundary_audit_logs.decision = @decision :: boundary_audit_decision
        ELSE true
    END
    -- Filter by time range (after)
    AND CASE
        WHEN @time_after :: timestamptz != '0001-01-01 00:00:00'::timestamptz THEN
            boundary_audit_logs.time >= @time_after
        ELSE true
    END
    -- Filter by time range (before)
    AND CASE
        WHEN @time_before :: timestamptz != '0001-01-01 00:00:00'::timestamptz THEN
            boundary_audit_logs.time <= @time_before
        ELSE true
    END
ORDER BY
    boundary_audit_logs.time DESC
LIMIT
    @limit_opt::int
OFFSET
    @offset_opt::int;

-- name: CountBoundaryAuditLogs :one
SELECT COUNT(*) AS count
FROM
    boundary_audit_logs
WHERE
    -- Filter by organization_id
    CASE
        WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
            boundary_audit_logs.organization_id = @organization_id
        ELSE true
    END
    -- Filter by workspace_id
    AND CASE
        WHEN @workspace_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
            boundary_audit_logs.workspace_id = @workspace_id
        ELSE true
    END
    -- Filter by workspace_owner_id
    AND CASE
        WHEN @workspace_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
            boundary_audit_logs.workspace_owner_id = @workspace_owner_id
        ELSE true
    END
    -- Filter by decision
    AND CASE
        WHEN @decision :: text != '' THEN
            boundary_audit_logs.decision = @decision :: boundary_audit_decision
        ELSE true
    END
    -- Filter by time range (after)
    AND CASE
        WHEN @time_after :: timestamptz != '0001-01-01 00:00:00'::timestamptz THEN
            boundary_audit_logs.time >= @time_after
        ELSE true
    END
    -- Filter by time range (before)
    AND CASE
        WHEN @time_before :: timestamptz != '0001-01-01 00:00:00'::timestamptz THEN
            boundary_audit_logs.time <= @time_before
        ELSE true
    END;

-- name: DeleteOldBoundaryAuditLogs :execrows
DELETE FROM boundary_audit_logs
WHERE time < @before::timestamptz;
