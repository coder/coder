-- We cannot alter the column type while a view depends on it, so we drop it and recreate it.
DROP VIEW template_version_with_user;


-- Turns the list of provider names into JSONB with the type `Array<{ id: string; optional?: boolean }>`
-- eg. `'{github,gitlab}'::text[]` would become `'[{"id": "github"}, {"id": "gitlab"}]'::jsonb`
CREATE OR REPLACE FUNCTION migrate_external_auth_providers_to_jsonb(text[])
  RETURNS jsonb
  LANGUAGE plpgsql
  AS $$
DECLARE
  result jsonb;
BEGIN
  SELECT
    jsonb_agg(jsonb_build_object('id', value::text)) INTO result
  FROM
    unnest($1) AS value;
  RETURN result;
END;
$$;


-- Update the column type and migrate the values
ALTER TABLE template_versions
  ALTER COLUMN external_auth_providers TYPE jsonb
  USING migrate_external_auth_providers_to_jsonb(external_auth_providers);


-- Make the column non-nullable to make the types nicer on the Go side
UPDATE template_versions
  SET external_auth_providers = '[]'::jsonb
  WHERE external_auth_providers IS NULL;
ALTER TABLE template_versions
  ALTER COLUMN external_auth_providers SET DEFAULT '[]'::jsonb;
ALTER TABLE template_versions
  ALTER COLUMN external_auth_providers SET NOT NULL;


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
FROM (public.template_versions
  LEFT JOIN visible_users ON (template_versions.created_by = visible_users.id));

COMMENT ON VIEW template_version_with_user IS 'Joins in the username + avatar url of the created by user.';


-- Cleanup
DROP FUNCTION migrate_external_auth_providers_to_jsonb;
