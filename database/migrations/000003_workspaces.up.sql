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

