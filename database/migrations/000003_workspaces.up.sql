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
    'create',
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

-- Cloud resources produced by a provision job.
CREATE TABLE workspace_resource (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    workspace_history_id uuid NOT NULL REFERENCES workspace_history (id) ON DELETE CASCADE,
    -- Resource type produced by a provisioner.
    -- eg. "google_compute_instance"
    type varchar(256) NOT NULL,
    -- Name of the resource.
    -- eg. "kyle-dev-instance"
    name varchar(64) NOT NULL,
    -- Token for an agent to connect.
    workspace_agent_token varchar(128) NOT NULL UNIQUE,
    -- If an agent has been conencted for this resource,
    -- the agent table is not null.
    workspace_agent_id uuid,

    UNIQUE(workspace_history_id, name)
);

CREATE TABLE workspace_agent (
    id uuid NOT NULL UNIQUE,
    workspace_resource_id uuid NOT NULL REFERENCES workspace_resource (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    -- Identifies instance architecture, cloud, etc.
    instance_metadata jsonb NOT NULL,
    -- Identifies resources.
    resource_metadata jsonb NOT NULL
);
