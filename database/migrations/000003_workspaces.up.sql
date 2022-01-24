CREATE TABLE workspace (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    owner_id text NOT NULL,
    project_id uuid NOT NULL REFERENCES project (id),
    name varchar(64) NOT NULL
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
    completed_at timestamptz,
    workspace_id uuid NOT NULL REFERENCES workspace (id) ON DELETE CASCADE,
    project_history_id uuid NOT NULL REFERENCES project_history (id) ON DELETE CASCADE,
    before_id uuid,
    after_id uuid,
    transition workspace_transition NOT NULL,
    initiator varchar(255) NOT NULL,
    -- State stored by the provisioner
    provisioner_state bytea,
    -- Job ID of the action
    provision_job_id uuid NOT NULL
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

CREATE TYPE log_level AS ENUM (
    'trace',
    'debug',
    'info',
    'warn',
    'error',
    'fatal'
);

CREATE TABLE workspace_log (
    workspace_id uuid NOT NULL REFERENCES workspace (id) ON DELETE CASCADE,
    -- workspace_history_id can be NULL because some events are not going to be part of a
    -- deliberate transition, e.g. an infrastructure failure that kills the workspace
    workspace_history_id uuid NOT NULL REFERENCES workspace_history (id) ON DELETE CASCADE,
    created timestamptz NOT NULL,
-- not sure this is necessary, also not sure it's necessary separate from the log column
    logged_by varchar(255),
    level log_level NOT NULL,
    log jsonb NOT NULL
);

CREATE INDEX workspace_log_index ON workspace_log (
    workspace_id,
    workspace_history_id
);