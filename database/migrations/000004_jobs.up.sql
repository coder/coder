CREATE TABLE IF NOT EXISTS provisioner_daemon (
    id uuid NOT NULL UNIQUE,
    created timestamptz NOT NULL,
    updated timestamptz,
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
    created timestamptz NOT NULL,
    updated timestamptz NOT NULL,
    started timestamptz,
    cancelled timestamptz,
    completed timestamptz,
    initiator uuid NOT NULL,
    worker uuid,
    provisioner provisioner_type NOT NULL,
    project uuid NOT NULL,
    type provisioner_job_type NOT NULL,
    input jsonb NOT NULL
);
