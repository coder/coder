-- name: InsertMCPGatewayEscalation :one
INSERT INTO mcp_gateway_escalations (
    id,
    mcp_server_config_id,
    server_slug,
    server_url,
    tool,
    input,
    ai_agent_id,
    sponsor_user_id,
    workspace_name,
    status,
    created_at,
    expires_at,
    resolved_at,
    resolved_by
) VALUES (
    @id,
    @mcp_server_config_id,
    @server_slug,
    @server_url,
    @tool,
    @input,
    @ai_agent_id,
    @sponsor_user_id,
    @workspace_name,
    @status,
    @created_at,
    @expires_at,
    @resolved_at,
    @resolved_by
)
RETURNING *;

-- name: GetMCPGatewayEscalationByID :one
SELECT * FROM mcp_gateway_escalations WHERE id = $1;

-- name: ListMCPGatewayEscalationsBySponsor :many
-- Lists escalations for a sponsor, optionally windowed on creation or
-- resolution time for the sponsor timeline. Zero bounds disable the window;
-- a zero limit returns all rows (management API behavior).
SELECT * FROM mcp_gateway_escalations
WHERE sponsor_user_id = @sponsor_user_id
  AND (@status::text = '' OR status = @status::text)
  AND CASE
    WHEN @ai_agent_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN ai_agent_id = @ai_agent_id::uuid
    ELSE true
  END
  AND (
    (
      (@after_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR created_at > @after_time::timestamptz)
      AND (@before_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR created_at < @before_time::timestamptz)
    )
    OR (
      resolved_at IS NOT NULL
      AND (@after_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR resolved_at > @after_time::timestamptz)
      AND (@before_time::timestamptz = '0001-01-01 00:00:00+00'::timestamptz OR resolved_at < @before_time::timestamptz)
    )
  )
ORDER BY GREATEST(created_at, COALESCE(resolved_at, created_at)) DESC
LIMIT NULLIF(@limit_::integer, 0);

-- name: ResolveMCPGatewayEscalation :one
UPDATE mcp_gateway_escalations
SET status = $2, resolved_at = $3, resolved_by = $4
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: ExpireMCPGatewayEscalations :execrows
UPDATE mcp_gateway_escalations
SET status = 'expired', resolved_at = $1
WHERE status = 'pending' AND expires_at < $1;
