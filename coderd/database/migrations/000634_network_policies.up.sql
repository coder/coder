ALTER TYPE provisioner_job_type ADD VALUE IF NOT EXISTS 'network_policy';

ALTER TABLE templates ADD CONSTRAINT templates_id_organization_id_key UNIQUE (id, organization_id);
ALTER TABLE provisioner_jobs ADD CONSTRAINT provisioner_jobs_id_organization_id_key UNIQUE (id, organization_id);

CREATE TABLE network_policies (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
    published_revision_id uuid,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (id, organization_id)
);
CREATE UNIQUE INDEX network_policies_organization_id_name_idx ON network_policies (organization_id, lower(name));

CREATE TABLE network_policy_revisions (
    id uuid PRIMARY KEY,
    policy_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    revision_number bigint NOT NULL CHECK (revision_number > 0),
    hcl text NOT NULL,
    file_id uuid NOT NULL REFERENCES files(id),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (policy_id, organization_id) REFERENCES network_policies(id, organization_id),
    UNIQUE (policy_id, revision_number),
    UNIQUE (id, policy_id, organization_id)
);
ALTER TABLE network_policies ADD CONSTRAINT network_policies_published_revision_fkey
    FOREIGN KEY (published_revision_id, id, organization_id) REFERENCES network_policy_revisions(id, policy_id, organization_id);

CREATE TABLE network_policy_attachments (
    policy_id uuid NOT NULL,
    template_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (policy_id, template_id),
    FOREIGN KEY (policy_id, organization_id) REFERENCES network_policies(id, organization_id),
    FOREIGN KEY (template_id, organization_id) REFERENCES templates(id, organization_id)
);
CREATE INDEX network_policy_attachments_template_id_idx ON network_policy_attachments(template_id);

CREATE TABLE network_policy_targets (
    id uuid PRIMARY KEY,
    policy_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    cluster text NOT NULL,
    context text NOT NULL,
    namespace text NOT NULL,
    tags jsonb NOT NULL DEFAULT '{}',
    state bytea NOT NULL DEFAULT '',
    active_job_id uuid,
    last_job_id uuid,
    applied_revision_id uuid,
    desired_revision_id uuid,
    generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
    status text NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'pending', 'planning', 'applying', 'succeeded', 'failed', 'canceled')),
    drifted boolean NOT NULL DEFAULT false,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    FOREIGN KEY (policy_id, organization_id) REFERENCES network_policies(id, organization_id),
    FOREIGN KEY (active_job_id, organization_id) REFERENCES provisioner_jobs(id, organization_id),
    FOREIGN KEY (last_job_id, organization_id) REFERENCES provisioner_jobs(id, organization_id),
    FOREIGN KEY (applied_revision_id, policy_id, organization_id) REFERENCES network_policy_revisions(id, policy_id, organization_id),
    FOREIGN KEY (desired_revision_id, policy_id, organization_id) REFERENCES network_policy_revisions(id, policy_id, organization_id),
    UNIQUE (policy_id, cluster, namespace),
    UNIQUE (id, policy_id, organization_id)
);

CREATE UNIQUE INDEX network_policy_targets_active_job_id_idx ON network_policy_targets(active_job_id) WHERE active_job_id IS NOT NULL;

CREATE TABLE network_policy_runs (
    id uuid PRIMARY KEY,
    job_id uuid NOT NULL UNIQUE,
    policy_id uuid NOT NULL,
    target_id uuid NOT NULL,
    revision_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    template_ids uuid[] NOT NULL,
    operation text NOT NULL CHECK (operation IN ('plan', 'apply')),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (job_id, organization_id) REFERENCES provisioner_jobs(id, organization_id),
    FOREIGN KEY (policy_id, organization_id) REFERENCES network_policies(id, organization_id),
    FOREIGN KEY (target_id, policy_id, organization_id) REFERENCES network_policy_targets(id, policy_id, organization_id),
    FOREIGN KEY (revision_id, policy_id, organization_id) REFERENCES network_policy_revisions(id, policy_id, organization_id)
);
CREATE INDEX network_policy_runs_policy_id_created_at_idx ON network_policy_runs(policy_id, created_at);
CREATE INDEX network_policy_runs_target_id_idx ON network_policy_runs(target_id);
