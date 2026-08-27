ALTER TABLE aibridge_tool_usages
    DROP COLUMN disposition,
    DROP COLUMN escalation_id;

DROP TABLE mcp_gateway_escalations;

-- No-op for the resource_type value because Postgres cannot remove enum values safely.
