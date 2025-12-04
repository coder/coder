CREATE TYPE boundary_audit_decision AS ENUM ('allow', 'deny');

CREATE TABLE boundary_audit_logs (
    id uuid PRIMARY KEY,
    time timestamp with time zone NOT NULL,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    workspace_owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_name text NOT NULL,
    agent_id uuid NOT NULL,
    agent_name text NOT NULL,
    resource_type text NOT NULL,
    resource text NOT NULL,
    operation text NOT NULL,
    decision boundary_audit_decision NOT NULL
);

COMMENT ON TABLE boundary_audit_logs IS 'Audit logs for resource access allowed or denied by Boundary in workspaces.';
COMMENT ON COLUMN boundary_audit_logs.time IS 'The timestamp when the resource access was requested.';
COMMENT ON COLUMN boundary_audit_logs.resource_type IS 'The type of resource being accessed (e.g., network, file).';
COMMENT ON COLUMN boundary_audit_logs.resource IS 'The resource being accessed (e.g., URL, file path).';
COMMENT ON COLUMN boundary_audit_logs.operation IS 'The operation being performed (e.g., GET, POST, read, write).';
COMMENT ON COLUMN boundary_audit_logs.decision IS 'Whether the access was allowed or denied by Boundary.';

CREATE INDEX idx_boundary_audit_logs_time ON boundary_audit_logs (time DESC);
CREATE INDEX idx_boundary_audit_logs_workspace_id ON boundary_audit_logs (workspace_id);
CREATE INDEX idx_boundary_audit_logs_org_id ON boundary_audit_logs (organization_id);
