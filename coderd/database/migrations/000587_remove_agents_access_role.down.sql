-- Older binaries need agents-access unless another role grants chat access.
-- Exact prior grants cannot be reconstructed, so restore the role to all
-- current non-service-account memberships.
-- Skip when a release/2.37 backport set the marker; there is nothing to undo.
UPDATE organization_members om
SET roles = array_append(om.roles, 'agents-access')
FROM users u
WHERE u.id = om.user_id
  AND NOT u.is_service_account
  AND NOT ('agents-access' = ANY(om.roles))
  AND NOT EXISTS (SELECT 1 FROM site_configs WHERE key = 'agents_access_default_role_backfilled');

-- Defaults are not restored: older binaries union default_org_member_roles
-- into service-account memberships too. Pre-upgrade explicit service-account
-- grants stay lost; an admin can re-grant them.
