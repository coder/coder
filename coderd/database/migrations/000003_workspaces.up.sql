CREATE TABLE workspaces (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    -- Use ON DELETE RESTRICT so that we can cleanup external workspace
    -- resources first.
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    project_id uuid NOT NULL REFERENCES projects (id) ON DELETE RESTRICT,
    deleted boolean NOT NULL DEFAULT FALSE,
    name varchar(64) NOT NULL,
    PRIMARY KEY (id)
);

-- Enforces no active workspaces have the same name.
CREATE UNIQUE INDEX ON workspaces (owner_id, name) WHERE deleted = FALSE;

CREATE TYPE workspace_transition AS ENUM (
    'start',
    'stop',
    'delete'
);

