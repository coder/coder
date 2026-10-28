ALTER TABLE workspace_builds
    ADD COLUMN state_template_resource_revision_id uuid REFERENCES template_resource_revisions(id),
    ADD COLUMN state_template_resource_outputs jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(state_template_resource_outputs) = 'object');

COMMENT ON COLUMN workspace_builds.state_template_resource_revision_id IS 'Foundation revision associated with the retained provisioner state, not the latest attempted build inputs.';
COMMENT ON COLUMN workspace_builds.state_template_resource_outputs IS 'Foundation output snapshot associated with the retained provisioner state, including partial apply checkpoints.';

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
    workspace_builds.template_resource_revision_id, workspace_builds.template_resource_outputs,
    workspace_builds.state_template_resource_revision_id, workspace_builds.state_template_resource_outputs
FROM workspace_builds LEFT JOIN visible_users ON workspace_builds.initiator_id = visible_users.id;
