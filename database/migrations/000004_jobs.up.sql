CREATE TABLE IF NOT EXISTS provisioner_daemon (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz,
    -- Name is generated for ease of differentiation.
    -- eg. WowBananas16
    name varchar(64) NOT NULL UNIQUE,
    provisioners provisioner_type [ ] NOT NULL
);

CREATE TYPE provisioner_job_type AS ENUM (
    'project_import',
    'workspace_provision'
);

CREATE TABLE IF NOT EXISTS provisioner_job (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    started_at timestamptz,
    cancelled_at timestamptz,
    completed_at timestamptz,
    error text,
    initiator_id text NOT NULL,
    provisioner provisioner_type NOT NULL,
    type provisioner_job_type NOT NULL,
    input jsonb NOT NULL,
    worker_id uuid
);

CREATE TYPE log_level AS ENUM (
    'trace',
    'debug',
    'info',
    'warn',
    'error'
);

CREATE TYPE log_source AS ENUM (
    'provisioner_daemon',
    'provisioner'
);

CREATE TABLE IF NOT EXISTS provisioner_job_log (
    id uuid NOT NULL UNIQUE,
    job_id uuid NOT NULL REFERENCES provisioner_job (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    source log_source NOT NULL,
    level log_level NOT NULL,
    output varchar(1024) NOT NULL
);

CREATE TYPE parameter_scope AS ENUM (
     'organization',
     'project',
     'user',
     'workspace'
);

-- Parameters are provided to jobs for provisioning and to workspaces.
CREATE TABLE parameter_value (
    id uuid NOT NULL UNIQUE,
    name varchar(64) NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    scope parameter_scope NOT NULL,
    scope_id text NOT NULL,
    source_scheme parameter_source_scheme NOT NULL,
    source_value text NOT NULL,
    destination_scheme parameter_destination_scheme NOT NULL,
    destination_value text NOT NULL,
    -- Prevents duplicates for parameters in the same scope.
    UNIQUE(name, scope, scope_id)
);