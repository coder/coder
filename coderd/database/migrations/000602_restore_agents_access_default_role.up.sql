-- Custom roles named agents-access could predate the built-in role. The
-- built-in would shadow them and make them undeletable.
DELETE FROM custom_roles WHERE name = 'agents-access';

-- A release/2.37 backport already appended this at startup and set this marker.
-- Keep later admin changes, such as orgs that removed agents-access.
UPDATE organizations
SET default_org_member_roles = array_append(default_org_member_roles, 'agents-access')
WHERE NOT ('agents-access' = ANY(default_org_member_roles))
  AND NOT EXISTS (SELECT 1 FROM site_configs WHERE key = 'agents_access_default_role_backfilled');
