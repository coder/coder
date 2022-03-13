-- Store arbitrary data like project source code or avatars.
CREATE TABLE file (
    hash varchar(64) NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    created_by text NOT NULL,
    mimetype varchar(64) NOT NULL,
    data bytea NOT NULL
);

CREATE TYPE provisioner_type AS ENUM ('echo', 'terraform');

-- Project defines infrastructure that your software project
-- requires for development.
CREATE TABLE project (
    id uuid NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    -- Projects must be scoped to an organization.
    organization_id text NOT NULL,
    deleted boolean NOT NULL DEFAULT FALSE,
    name varchar(64) NOT NULL,
    provisioner provisioner_type NOT NULL,
    -- Target's a Project Version to use for Workspaces.
    -- If a Workspace doesn't match this version, it will be prompted to rebuild.
    active_version_id uuid NOT NULL,
    -- Disallow projects to have the same name under
    -- the same organization.
    UNIQUE(organization_id, name)
);

-- Enforces no active projects have the same name.
CREATE UNIQUE INDEX ON project (organization_id, name) WHERE deleted = FALSE;

-- Project Versions store historical project data. When a Project Version is imported,
-- an "import" job is queued to parse parameters. A Project Version
-- can only be used if the import job succeeds.
CREATE TABLE project_version (
    id uuid NOT NULL UNIQUE,
    -- This should be indexed.
    project_id uuid REFERENCES project (id),
    organization_id text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    -- Name is generated for ease of differentiation.
    -- eg. TheCozyRabbit16
    name varchar(64) NOT NULL,
    -- Extracted from a README.md on import.
    -- Maximum of 1MB.
    description varchar(1048576) NOT NULL,
    -- The job ID for building the project version.
    job_id uuid NOT NULL,
    -- Disallow projects to have the same build name
    -- multiple times.
    UNIQUE(project_id, name)
);
