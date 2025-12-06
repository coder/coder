-- Add boundary audit configuration columns to workspace_agents table.
-- These control how boundary network audit logs are exported.

ALTER TABLE workspace_agents
    ADD COLUMN boundary_audit_otel_endpoint TEXT NOT NULL DEFAULT '',
    ADD COLUMN boundary_audit_otel_headers JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN boundary_audit_send_to_coderd BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN workspace_agents.boundary_audit_otel_endpoint IS 'OTEL endpoint URL for sending boundary network audit logs via OTLP/HTTP. If empty, logs are sent to coderd.';
COMMENT ON COLUMN workspace_agents.boundary_audit_otel_headers IS 'Optional headers for OTEL endpoint authentication as JSON object.';
COMMENT ON COLUMN workspace_agents.boundary_audit_send_to_coderd IS 'Whether to also send boundary network audit logs to coderd when OTEL is enabled.';
