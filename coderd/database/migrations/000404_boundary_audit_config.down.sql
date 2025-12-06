ALTER TABLE workspace_agents
    DROP COLUMN boundary_audit_otel_endpoint,
    DROP COLUMN boundary_audit_otel_headers,
    DROP COLUMN boundary_audit_send_to_coderd;
