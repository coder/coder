CREATE TYPE boundary_network_action AS ENUM ('allow', 'deny');

CREATE TABLE boundary_network_audit_logs (
    id uuid PRIMARY KEY,
    time timestamp with time zone NOT NULL,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    workspace_owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_name text NOT NULL,
    agent_id uuid NOT NULL,
    agent_name text NOT NULL,
    domain text NOT NULL,
    action boundary_network_action NOT NULL
);

COMMENT ON TABLE boundary_network_audit_logs IS 'Audit logs for network requests allowed or denied by Boundary in workspaces.';
COMMENT ON COLUMN boundary_network_audit_logs.time IS 'The timestamp when the network request was made.';
COMMENT ON COLUMN boundary_network_audit_logs.domain IS 'The domain that was requested (e.g., github.com).';
COMMENT ON COLUMN boundary_network_audit_logs.action IS 'Whether the request was allowed or denied by Boundary.';

CREATE INDEX idx_boundary_network_audit_logs_time ON boundary_network_audit_logs (time DESC);
CREATE INDEX idx_boundary_network_audit_logs_workspace_id ON boundary_network_audit_logs (workspace_id);
CREATE INDEX idx_boundary_network_audit_logs_org_id ON boundary_network_audit_logs (organization_id);
