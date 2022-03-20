CREATE TABLE workspaces (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    owner_id text NOT NULL,
    project_id uuid NOT NULL REFERENCES projects (id),
    deleted boolean NOT NULL DEFAULT FALSE,
    name varchar(64) NOT NULL
);

-- Enforces no active workspaces have the same name.
CREATE UNIQUE INDEX ON workspaces (owner_id, name) WHERE deleted = FALSE;

CREATE TYPE workspace_transition AS ENUM (
    'start',
    'stop',
    'delete'
);

