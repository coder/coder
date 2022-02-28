CREATE TABLE workspace (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    owner_id text NOT NULL,
    project_id uuid NOT NULL REFERENCES project (id),
    name varchar(64) NOT NULL,
    UNIQUE(owner_id, name)
);

CREATE TYPE workspace_transition AS ENUM (
    'start',
    'stop',
    'delete'
);

-- Workspace transition represents a change in workspace state.
CREATE TABLE workspace_history (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    workspace_id uuid NOT NULL REFERENCES workspace (id) ON DELETE CASCADE,
    project_version_id uuid NOT NULL REFERENCES project_version (id) ON DELETE CASCADE,
    name varchar(64) NOT NULL,
    before_id uuid,
    after_id uuid,
    transition workspace_transition NOT NULL,
    initiator varchar(255) NOT NULL,
    -- State stored by the provisioner
    provisioner_state bytea,
    -- Job ID of the action
    provision_job_id uuid NOT NULL,
    UNIQUE(workspace_id, name)
);
