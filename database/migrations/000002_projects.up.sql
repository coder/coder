CREATE TYPE provisioner_type AS ENUM ('terraform', 'cdr-basic');

-- Project defines infrastructure that your software project
-- requires for development.
CREATE TABLE project (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    -- Projects must be scoped to an organization.
    organization_id text NOT NULL,
    name varchar(64) NOT NULL,
    provisioner provisioner_type NOT NULL,
    -- Target's a Project Version to use for Workspaces.
    -- If a Workspace doesn't match this version, it will be prompted to rebuild.
    active_version_id uuid,
    -- Disallow projects to have the same name under
    -- the same organization.
    UNIQUE(organization_id, name)
);

CREATE TYPE project_storage_method AS ENUM ('inline-archive');

-- Project Versions store Project history. When a Project Version is imported,
-- an "import" job is queued to parse parameters. A Project Version
-- can only be used if the import job succeeds.
CREATE TABLE project_history (
    id uuid NOT NULL UNIQUE,
    -- This should be indexed.
    project_id uuid NOT NULL REFERENCES project (id),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    -- Name is generated for ease of differentiation.
    -- eg. TheCozyRabbit16
    name varchar(64) NOT NULL,
    -- Extracted from a README.md on import.
    -- Maximum of 1MB.
    description varchar(1048576) NOT NULL,
    storage_method project_storage_method NOT NULL,
    storage_source bytea NOT NULL,
    -- The import job for a Project Version. This is used
    -- to detect if an import was successful.
    import_job_id uuid NOT NULL,
    -- Disallow projects to have the same build name
    -- multiple times.
    UNIQUE(project_id, name)
);

-- Types of parameters the automator supports.
CREATE TYPE parameter_type_system AS ENUM ('hcl');

-- Supported schemes for a parameter source.
CREATE TYPE parameter_source_scheme AS ENUM('data');

-- Supported schemes for a parameter destination.
CREATE TYPE parameter_destination_scheme AS ENUM('environment_variable', 'provisioner_variable');

-- Stores project version parameters parsed on import.
-- No secrets are stored here.
-- 
-- All parameter validation occurs server-side to process
-- complex validations.
-- 
-- Parameter types, description, and validation will produce
-- a UI for users to enter values.
-- Needs to be made consistent with the examples below.
CREATE TABLE project_parameter (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    project_history_id uuid NOT NULL REFERENCES project_history(id) ON DELETE CASCADE,
    name varchar(64) NOT NULL,
    -- 8KB limit
    description varchar(8192) NOT NULL DEFAULT '',
    -- eg. data://inlinevalue
    default_source_scheme parameter_source_scheme,
    default_source_value text,
    -- Allows the user to override the source.
    allow_override_source boolean NOT null,
    -- eg. env://SOME_VARIABLE, tfvars://example
    default_destination_scheme parameter_destination_scheme,
    default_destination_value text,
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
    UNIQUE(project_history_id, name)
);
