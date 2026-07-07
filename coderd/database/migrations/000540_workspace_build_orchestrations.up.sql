-- Postgres requires the referenced column set of a composite foreign
-- key to have its own unique constraint, even though id is already
-- unique.
ALTER TABLE workspace_builds
    ADD CONSTRAINT workspace_builds_id_workspace_id_key
        UNIQUE (id, workspace_id);

ALTER TABLE template_version_presets
    ADD CONSTRAINT template_version_presets_id_template_version_id_key
        UNIQUE (id, template_version_id);

CREATE TABLE workspace_build_orchestrations (
    id UUID PRIMARY KEY NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    workspace_id UUID NOT NULL,
    parent_build_id UUID UNIQUE NOT NULL,
    child_build_id UUID UNIQUE,
    child_transition workspace_transition NOT NULL,
    child_template_version_id UUID REFERENCES template_versions(id) ON DELETE CASCADE,
    child_template_version_preset_id UUID, -- a constraint is added below
    child_rich_parameter_values JSONB DEFAULT '[]'::JSONB NOT NULL,
    child_log_level TEXT DEFAULT '' NOT NULL,
    child_reason build_reason,
    attempt_count INTEGER DEFAULT 0 NOT NULL,
    next_retry_after TIMESTAMPTZ,
    status TEXT DEFAULT 'pending' NOT NULL,
    error TEXT,
    CONSTRAINT workspace_build_orchestrations_status_check CHECK (
        status IN ('pending', 'completed', 'failed', 'canceled')
    ),
    CONSTRAINT workspace_build_orchestrations_completed_child_check CHECK (
        status <> 'completed' OR child_build_id IS NOT NULL
    ),
    CONSTRAINT workspace_build_orchestrations_child_parameters_check CHECK (
        jsonb_typeof(child_rich_parameter_values) = 'array'
    ),
    CONSTRAINT workspace_build_orchestrations_attempt_count_check CHECK (
        attempt_count >= 0
    ),
    CONSTRAINT workspace_build_orchestrations_next_retry_after_check CHECK (
        status = 'pending' OR next_retry_after IS NULL
    ),
    CONSTRAINT workspace_build_orchestrations_child_preset_version_check CHECK (
        child_template_version_preset_id IS NULL OR child_template_version_id IS NOT NULL
    ),
    -- Mirrors CreateWorkspaceBuildRequest validation, where the optional
    -- log level is either unset or debug.
    CONSTRAINT workspace_build_orchestrations_child_log_level_check CHECK (
        child_log_level IN ('', 'debug')
    ),
    -- These constraints enforce that any stored child preset belongs to
    -- the requested child template version, while preset deletion still
    -- clears only the preset column.
    CONSTRAINT workspace_build_orchestrations_child_preset_id_fkey
        FOREIGN KEY (child_template_version_preset_id)
        REFERENCES template_version_presets(id)
        ON DELETE SET NULL,
    CONSTRAINT workspace_build_orchestrations_child_preset_version_fkey
        FOREIGN KEY (child_template_version_preset_id, child_template_version_id)
        REFERENCES template_version_presets(id, template_version_id),
    -- Composite foreign keys enforce that the parent and child builds
    -- belong to the same workspace.
    CONSTRAINT workspace_build_orchestrations_parent_build_workspace_id_fkey
        FOREIGN KEY (parent_build_id, workspace_id)
        REFERENCES workspace_builds(id, workspace_id)
        ON DELETE CASCADE,
    CONSTRAINT workspace_build_orchestrations_child_build_workspace_id_fkey
        FOREIGN KEY (child_build_id, workspace_id)
        REFERENCES workspace_builds(id, workspace_id)
        ON DELETE CASCADE
);

-- The orchestrator scans eligible pending rows oldest first and skips
-- terminal rows and retry rows whose delay has not elapsed.
CREATE INDEX idx_workspace_build_orchestrations_pending
    ON workspace_build_orchestrations (created_at)
    WHERE status = 'pending';

COMMENT ON TABLE workspace_build_orchestrations IS
    'Tracks durable follow-up workspace build operations, such as server-side restart, where one child build is created after a parent build completes successfully.';

COMMENT ON COLUMN workspace_build_orchestrations.parent_build_id IS
    'Unique because we only support sequences with one child build per parent build.';

COMMENT ON COLUMN workspace_build_orchestrations.workspace_id IS
    'Copied from the parent build so the database can enforce that parent and child builds belong to the same workspace.';

COMMENT ON COLUMN workspace_build_orchestrations.child_build_id IS
    'Nullable because the child build is created only after the parent build completes successfully.';

COMMENT ON COLUMN workspace_build_orchestrations.attempt_count IS
    'Counts retryable child build creation failures for this orchestration row.';

COMMENT ON COLUMN workspace_build_orchestrations.next_retry_after IS
    'When set, the orchestrator skips this pending row until the timestamp has passed.';

-- Add workspace_build_orchestration scopes for RBAC.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_build_orchestration:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_build_orchestration:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_build_orchestration:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_build_orchestration:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_build_orchestration:update';
