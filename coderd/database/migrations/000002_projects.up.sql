-- Store arbitrary data like project source code or avatars.
CREATE TABLE files (
    hash varchar(64) NOT NULL,
    created_at timestamptz NOT NULL,
    -- foreign key?
    created_by uuid NOT NULL,
    mimetype varchar(64) NOT NULL,
    data bytea NOT NULL,
    PRIMARY KEY (hash)
);

CREATE TYPE provisioner_type AS ENUM ('echo', 'terraform');

-- Project defines infrastructure that your software project
-- requires for development.
CREATE TABLE projects (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    -- Projects must be scoped to an organization.
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    deleted boolean NOT NULL DEFAULT FALSE,
    name varchar(64) NOT NULL,
    provisioner provisioner_type NOT NULL,
    -- Target's a Project Version to use for Workspaces.
    -- If a Workspace doesn't match this version, it will be prompted to rebuild.
    active_version_id uuid NOT NULL,
    PRIMARY KEY (id),
    -- Disallow projects to have the same name under
    -- the same organization.
    UNIQUE(organization_id, name)
);

-- Enforces no active projects have the same name.
CREATE UNIQUE INDEX ON projects (organization_id, name) WHERE deleted = FALSE;
CREATE UNIQUE INDEX idx_projects_name_lower ON projects USING btree (lower(name));

-- Project Versions store historical project data. When a Project Version is imported,
-- an "import" job is queued to parse parameters. A Project Version
-- can only be used if the import job succeeds.
CREATE TABLE project_versions (
    id uuid NOT NULL,
    -- This should be indexed. It is intentionally nullable.
    project_id uuid REFERENCES projects (id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
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
    PRIMARY KEY (id),
    -- Disallow projects to have the same build name
    -- multiple times.
    UNIQUE(project_id, name)
);
