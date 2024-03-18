-- We cannot alter the column type while a view depends on it, so we drop it and recreate it.
DROP VIEW template_version_with_user;


-- Does the opposite of `migrate_external_auth_providers_to_jsonb`
-- eg. `'[{"id": "github"}, {"id": "gitlab"}]'::jsonb` would become `'{github,gitlab}'::text[]`
CREATE OR REPLACE FUNCTION revert_migrate_external_auth_providers_to_jsonb(jsonb)
  RETURNS text[]
  LANGUAGE plpgsql
  AS $$
DECLARE
  result text[];
BEGIN
  SELECT
    array_agg(id::text) INTO result
  FROM (
    SELECT
      jsonb_array_elements($1) ->> 'id' AS id) AS external_auth_provider_ids;
  RETURN result;
END;
$$;


-- Remove the non-null constraint and default
ALTER TABLE template_versions
  ALTER COLUMN external_auth_providers DROP DEFAULT;
ALTER TABLE template_versions
  ALTER COLUMN external_auth_providers DROP NOT NULL;


-- Update the column type and migrate the values
ALTER TABLE template_versions
  ALTER COLUMN external_auth_providers TYPE text[]
  USING revert_migrate_external_auth_providers_to_jsonb(external_auth_providers);


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


-- Cleanup
DROP FUNCTION revert_migrate_external_auth_providers_to_jsonb;
