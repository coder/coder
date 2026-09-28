-- Deleted legacy custom roles named agents-access are not restored.
-- Skip when a release/2.37 backport set the marker; there is nothing to undo.
UPDATE organizations
SET default_org_member_roles = array_remove(default_org_member_roles, 'agents-access')
WHERE 'agents-access' = ANY(default_org_member_roles)
  AND NOT EXISTS (SELECT 1 FROM site_configs WHERE key = 'agents_access_default_role_backfilled');

UPDATE organization_members
SET roles = array_remove(roles, 'agents-access')
WHERE 'agents-access' = ANY(roles)
  AND NOT EXISTS (SELECT 1 FROM site_configs WHERE key = 'agents_access_default_role_backfilled');
