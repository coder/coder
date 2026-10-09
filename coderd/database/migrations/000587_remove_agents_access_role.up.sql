UPDATE users
SET rbac_roles = array_remove(rbac_roles, 'agents-access')
WHERE 'agents-access' = ANY(rbac_roles);

-- A release/2.37 backport restores agents-access defaults at startup and sets
-- this marker. Keep later admin changes to defaults and member grants.
UPDATE organization_members
SET roles = array_remove(roles, 'agents-access')
WHERE 'agents-access' = ANY(roles)
  AND NOT EXISTS (SELECT 1 FROM site_configs WHERE key = 'agents_access_default_role_backfilled');

UPDATE organizations
SET default_org_member_roles = array_remove(default_org_member_roles, 'agents-access')
WHERE 'agents-access' = ANY(default_org_member_roles)
  AND NOT EXISTS (SELECT 1 FROM site_configs WHERE key = 'agents_access_default_role_backfilled');
