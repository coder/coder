-- Determines if a coder_ai_task resource is defined in a template version.
ALTER TABLE
    template_versions
ADD
    COLUMN has_external_agents BOOLEAN;

DROP VIEW template_version_with_user;

-- We're adding the external_agents column.
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
    template_versions.has_ai_task,
	template_versions.has_external_agents,
    COALESCE(visible_users.avatar_url, '' :: text) AS created_by_avatar_url,
    COALESCE(visible_users.username, '' :: text) AS created_by_username,
    COALESCE(visible_users.name, '' :: text) AS created_by_name
FROM
    (
        template_versions
        LEFT JOIN visible_users ON (
            (template_versions.created_by = visible_users.id)
        )
    );

COMMENT ON VIEW template_version_with_user IS 'Joins in the username + avatar url of the created by user.';
