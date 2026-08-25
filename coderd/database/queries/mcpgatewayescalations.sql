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
SELECT * FROM mcp_gateway_escalations
WHERE sponsor_user_id = @sponsor_user_id
  AND (@status::text = '' OR status = @status::text)
ORDER BY created_at DESC;

-- name: ResolveMCPGatewayEscalation :one
UPDATE mcp_gateway_escalations
SET status = $2, resolved_at = $3, resolved_by = $4
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: ExpireMCPGatewayEscalations :execrows
UPDATE mcp_gateway_escalations
SET status = 'expired', resolved_at = $1
WHERE status = 'pending' AND expires_at < $1;
