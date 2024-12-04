-- We cannot alter the column type while a view depends on it, so we drop it and recreate it.
DROP VIEW template_version_with_user;

ALTER TABLE
    template_versions
DROP COLUMN source_example_id;

-- Recreate `template_version_with_user` as described in dump.sql
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
  COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
  COALESCE(visible_users.username, ''::text) AS created_by_username
FROM (template_versions
  LEFT JOIN visible_users ON (template_versions.created_by = visible_users.id));

COMMENT ON VIEW template_version_with_user IS 'Joins in the username + avatar url of the created by user.';
