-- Older binaries need agents-access unless another role grants chat access.
-- Exact prior grants cannot be reconstructed, so restore the role to all
-- current non-service-account memberships.
UPDATE organization_members om
SET roles = array_append(om.roles, 'agents-access')
FROM users u
WHERE u.id = om.user_id
  AND NOT u.is_service_account
  AND NOT ('agents-access' = ANY(om.roles));

-- Defaults are not restored: older binaries union default_org_member_roles
-- into service-account memberships too. Pre-upgrade explicit service-account
-- grants stay lost; an admin can re-grant them.
