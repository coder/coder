ALTER TYPE resource_type ADD VALUE 'github_runner_pool';

CREATE TABLE github_runner_pools (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    owner_id uuid NOT NULL REFERENCES users(id),
    template_id uuid NOT NULL REFERENCES templates(id),
    name text NOT NULL,
    github_config_url text NOT NULL,
    github_token text NOT NULL,
    github_token_key_id text REFERENCES dbcrypt_keys(active_key_digest),
    runner_group_id bigint NOT NULL DEFAULT 1,
    scale_set_id bigint NOT NULL DEFAULT 0,
    max_runners integer NOT NULL CHECK (max_runners BETWEEN 1 AND 1000),
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'paused', 'draining', 'deleted')),
    startup_timeout_seconds integer NOT NULL DEFAULT 600 CHECK (startup_timeout_seconds BETWEEN 60 AND 3600),
    job_timeout_seconds integer NOT NULL DEFAULT 21600 CHECK (job_timeout_seconds BETWEEN 60 AND 86400),
    assigned_jobs integer NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT '',
    lease_owner uuid,
    lease_expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (id, organization_id),
    UNIQUE (organization_id, name)
);

CREATE UNIQUE INDEX github_runner_pools_github_identity ON github_runner_pools (github_config_url, runner_group_id, name) WHERE state <> 'deleted';

CREATE TABLE github_runners (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    pool_id uuid NOT NULL,
    workspace_id uuid REFERENCES workspaces(id),
    name text NOT NULL UNIQUE,
    github_runner_id bigint NOT NULL DEFAULT 0,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'provisioning', 'idle', 'busy', 'finishing', 'deleting', 'deleted')),
    job_request_id bigint NOT NULL DEFAULT 0,
    job_result text NOT NULL DEFAULT '',
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (pool_id, organization_id) REFERENCES github_runner_pools(id, organization_id)
);

CREATE INDEX github_runners_pool_id ON github_runners (pool_id);
CREATE UNIQUE INDEX github_runners_workspace_id ON github_runners (workspace_id) WHERE workspace_id IS NOT NULL;
