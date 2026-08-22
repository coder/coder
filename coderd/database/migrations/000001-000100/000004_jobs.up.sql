CREATE TABLE IF NOT EXISTS provisioner_daemons (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz,
    organization_id uuid,
    -- Name is generated for ease of differentiation.
    -- eg. WowBananas16
    name varchar(64) NOT NULL UNIQUE,
    provisioners provisioner_type [ ] NOT NULL,
    PRIMARY KEY (id)
);

CREATE TYPE provisioner_job_type AS ENUM (
    'template_version_import',
    'workspace_build'
);

CREATE TYPE provisioner_storage_method AS ENUM ('file');

CREATE TABLE IF NOT EXISTS provisioner_jobs (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    started_at timestamptz,
    canceled_at timestamptz,
    completed_at timestamptz,
    error text,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- foreign key?
    initiator_id uuid NOT NULL,
    provisioner provisioner_type NOT NULL,
    storage_method provisioner_storage_method NOT NULL,
    storage_source text NOT NULL,
    type provisioner_job_type NOT NULL,
    input jsonb NOT NULL,
    worker_id uuid,
    PRIMARY KEY (id)
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

CREATE TABLE IF NOT EXISTS provisioner_job_logs (
    id uuid NOT NULL,
    job_id uuid NOT NULL REFERENCES provisioner_jobs (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    source log_source NOT NULL,
    level log_level NOT NULL,
    stage varchar(128) NOT NULL,
    output varchar(1024) NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE workspace_resources (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    job_id uuid NOT NULL REFERENCES provisioner_jobs (id) ON DELETE CASCADE,
    transition workspace_transition NOT NULL,
    type varchar(192) NOT NULL,
    name varchar(64) NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE workspace_agents (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    name varchar(64) NOT NULL,
    first_connected_at timestamptz,
    last_connected_at timestamptz,
    disconnected_at timestamptz,
    resource_id uuid NOT NULL REFERENCES workspace_resources (id) ON DELETE CASCADE,
    auth_token uuid NOT NULL,
    auth_instance_id varchar(64),
    architecture varchar(64) NOT NULL,
    environment_variables jsonb,
    operating_system varchar(64) NOT NULL,
    startup_script varchar(65534),
    instance_metadata jsonb,
    resource_metadata jsonb,
    PRIMARY KEY (id)
);

CREATE TYPE parameter_scope AS ENUM (
     'organization',
     'template',
     'import_job',
     'user',
     'workspace'
);

-- Types of parameters the automator supports.
CREATE TYPE parameter_type_system AS ENUM ('none', 'hcl');

-- Supported schemes for a parameter source.
CREATE TYPE parameter_source_scheme AS ENUM('none', 'data');

-- Supported schemes for a parameter destination.
CREATE TYPE parameter_destination_scheme AS ENUM('none', 'environment_variable', 'provisioner_variable');

-- Stores template version parameters parsed on import.
-- No secrets are stored here.
-- 
-- All parameter validation occurs server-side to process
-- complex validations.
-- 
-- Parameter types, description, and validation will produce
-- a UI for users to enter values.
-- Needs to be made consistent with the examples below.
CREATE TABLE parameter_schemas (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    job_id uuid NOT NULL REFERENCES provisioner_jobs (id) ON DELETE CASCADE,
    name varchar(64) NOT NULL,
    description varchar(8192) NOT NULL DEFAULT '',
    default_source_scheme parameter_source_scheme,
    default_source_value text NOT NULL,
    -- Allows the user to override the source.
    allow_override_source boolean NOT null,
    default_destination_scheme parameter_destination_scheme,
    -- Allows the user to override the destination.
    allow_override_destination boolean NOT null,
    default_refresh text NOT NULL,
    -- Whether the consumer can view the source and destinations.
    redisplay_value boolean NOT null,
    -- This error would appear in the UI if the condition is not met.
    validation_error varchar(256) NOT NULL,
    validation_condition varchar(512) NOT NULL,
    validation_type_system parameter_type_system NOT NULL,
    validation_value_type varchar(64) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE(job_id, name)
);

-- Parameters are provided to jobs for provisioning and to workspaces.
CREATE TABLE parameter_values (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    scope parameter_scope NOT NULL,
    scope_id uuid NOT NULL,
    name varchar(64) NOT NULL,
    source_scheme parameter_source_scheme NOT NULL,
    source_value text NOT NULL,
    destination_scheme parameter_destination_scheme NOT NULL,
    PRIMARY KEY (id),
    -- Prevents duplicates for parameters in the same scope.
    UNIQUE(scope_id, name)
);

CREATE TABLE workspace_builds (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    template_version_id uuid NOT NULL REFERENCES template_versions (id) ON DELETE CASCADE,
    name varchar(64) NOT NULL,
    build_number integer NOT NULL,
    transition workspace_transition NOT NULL,
    initiator_id uuid NOT NULL,
    -- State stored by the provisioner
    provisioner_state bytea,
    -- Job ID of the action
    job_id uuid NOT NULL UNIQUE REFERENCES provisioner_jobs (id) ON DELETE CASCADE,
    PRIMARY KEY (id),
    UNIQUE(workspace_id, name),
    UNIQUE(workspace_id, build_number)
);
