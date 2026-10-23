ALTER TYPE provisioner_job_type ADD VALUE IF NOT EXISTS 'template_resources';

CREATE TABLE template_resource_states (
    template_id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    current_revision_id uuid,
    applied_revision_id uuid,
    active_run_id uuid,
    status text NOT NULL DEFAULT 'unconfigured' CHECK (status IN ('unconfigured', 'applied', 'failed', 'unknown', 'destroyed')),
    state bytea NOT NULL DEFAULT '',
    outputs jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(outputs) = 'object'),
    updated_at timestamptz NOT NULL,
    FOREIGN KEY (template_id, organization_id) REFERENCES templates(id, organization_id)
);

CREATE TABLE template_resource_revisions (
    id uuid PRIMARY KEY,
    template_id uuid NOT NULL,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    file_id uuid NOT NULL REFERENCES files(id),
    source text NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (template_id, organization_id) REFERENCES templates(id, organization_id),
    UNIQUE (id, template_id, organization_id)
);
CREATE INDEX template_resource_revisions_template_created_idx ON template_resource_revisions(template_id, created_at DESC);

CREATE TABLE template_resource_runs (
    id uuid PRIMARY KEY,
    template_id uuid NOT NULL,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    revision_id uuid NOT NULL,
    job_id uuid NOT NULL UNIQUE,
    action text NOT NULL CHECK (action IN ('plan', 'apply', 'destroy')),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (template_id, organization_id) REFERENCES templates(id, organization_id),
    FOREIGN KEY (revision_id, template_id, organization_id) REFERENCES template_resource_revisions(id, template_id, organization_id),
    FOREIGN KEY (job_id, organization_id) REFERENCES provisioner_jobs(id, organization_id),
    UNIQUE (id, template_id, organization_id)
);
CREATE INDEX template_resource_runs_template_created_idx ON template_resource_runs(template_id, created_at DESC);
ALTER TABLE template_resource_states
    ADD FOREIGN KEY (current_revision_id, template_id, organization_id) REFERENCES template_resource_revisions(id, template_id, organization_id),
    ADD FOREIGN KEY (applied_revision_id, template_id, organization_id) REFERENCES template_resource_revisions(id, template_id, organization_id),
    ADD FOREIGN KEY (active_run_id, template_id, organization_id) REFERENCES template_resource_runs(id, template_id, organization_id);

ALTER TABLE workspace_builds
    ADD COLUMN template_resource_revision_id uuid REFERENCES template_resource_revisions(id),
    ADD COLUMN template_resource_outputs jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(template_resource_outputs) = 'object');

-- Appending columns preserves all existing view column positions.
CREATE OR REPLACE VIEW workspace_build_with_user AS
SELECT workspace_builds.id, workspace_builds.created_at, workspace_builds.updated_at,
    workspace_builds.workspace_id, workspace_builds.template_version_id, workspace_builds.build_number,
    workspace_builds.transition, workspace_builds.initiator_id, workspace_builds.job_id,
    workspace_builds.deadline, workspace_builds.reason, workspace_builds.daily_cost,
    workspace_builds.max_deadline, workspace_builds.template_version_preset_id,
    workspace_builds.has_external_agent, workspace_builds.notified_autostop_deadline,
    coalesce(visible_users.avatar_url, '') AS initiator_by_avatar_url,
    coalesce(visible_users.username, '') AS initiator_by_username,
    coalesce(visible_users.name, '') AS initiator_by_name,
    workspace_builds.template_resource_revision_id, workspace_builds.template_resource_outputs
FROM workspace_builds LEFT JOIN visible_users ON workspace_builds.initiator_id = visible_users.id;
