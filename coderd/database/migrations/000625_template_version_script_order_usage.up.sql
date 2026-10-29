ALTER TABLE template_versions
	ADD COLUMN script_order_data_source_count INTEGER NOT NULL DEFAULT 0 CHECK (script_order_data_source_count >= 0),
	ADD COLUMN script_order_rule_count INTEGER NOT NULL DEFAULT 0 CHECK (script_order_rule_count >= 0);

COMMENT ON COLUMN template_versions.script_order_data_source_count IS
	'Number of coder_script_order data source declarations in the template configuration.';

COMMENT ON COLUMN template_versions.script_order_rule_count IS
	'Number of rule blocks across coder_script_order declarations in the template configuration.';

-- The view must expose every base-table column to keep the generated
-- models consistent, as enforced by TestViewSubsetTemplateVersion.
DROP VIEW template_version_with_user;

CREATE VIEW template_version_with_user AS
SELECT
	template_versions.id,
	template_versions.template_id,
	template_versions.organization_id,
	template_versions.created_at,
	template_versions.updated_at,
	template_versions.name,
	template_versions.readme,
	template_versions.job_id,
	template_versions.created_by,
	template_versions.external_auth_providers,
	template_versions.message,
	template_versions.archived,
	template_versions.source_example_id,
	template_versions.has_external_agent,
	template_versions.script_order_data_source_count,
	template_versions.script_order_rule_count,
	COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
	COALESCE(visible_users.username, ''::text) AS created_by_username,
	COALESCE(visible_users.name, ''::text) AS created_by_name
FROM template_versions
LEFT JOIN visible_users ON template_versions.created_by = visible_users.id;

COMMENT ON VIEW template_version_with_user IS 'Joins in the username + avatar url of the created by user.';

-- Elect one replica per snapshot period to report script-order adoption.
ALTER TABLE telemetry_locks DROP CONSTRAINT telemetry_lock_event_type_constraint;
ALTER TABLE telemetry_locks ADD CONSTRAINT telemetry_lock_event_type_constraint
	CHECK (event_type IN ('aibridge_interceptions_summary', 'boundary_usage_summary', 'user_secrets_summary', 'script_order_summary'));
