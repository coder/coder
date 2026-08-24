-- MinimalUser now requires an email (#28240), but the visible_users view has
-- no email column to source it from. Add email to the view so identity joins
-- can populate MinimalUser.email.
-- CREATE OR REPLACE may append columns; dependent views keep working.

CREATE OR REPLACE VIEW visible_users AS
SELECT
	users.id,
	users.username,
	users.name,
	users.avatar_url,
	users.email
FROM users;

COMMENT ON VIEW visible_users IS 'Visible fields of users are allowed to be joined with other tables for including context of other resources.';

-- Expose created_by_email on template versions (feeds the created_by
-- MinimalUser's email).
CREATE OR REPLACE VIEW template_version_with_user AS
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
	template_versions.has_ai_task,
	template_versions.has_external_agent,
	COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
	COALESCE(visible_users.username, ''::text) AS created_by_username,
	COALESCE(visible_users.name, ''::text) AS created_by_name,
	COALESCE(visible_users.email, ''::text) AS created_by_email
FROM (
	template_versions
	LEFT JOIN visible_users ON (template_versions.created_by = visible_users.id)
);

COMMENT ON VIEW template_version_with_user IS 'Joins in the username, name, avatar url, and email of the created by user.';
