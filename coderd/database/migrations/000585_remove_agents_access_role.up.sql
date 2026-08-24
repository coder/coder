-- The 'agents-access' (Coder Agents User) role is removed; chat access is
-- now part of the implicit organization-member permission floor. Stale
-- grants no longer authorize anything, but they render as raw strings in
-- role UIs and fail role-update validation, so scrub stored role names.
-- users.rbac_roles needs no scrub: 000475 moved site grants to org scope
-- and role validation has rejected the name at the site level since.
UPDATE organization_members
SET roles = array_remove(roles, 'agents-access')
WHERE 'agents-access' = ANY(roles);

UPDATE organizations
SET default_org_member_roles = array_remove(default_org_member_roles, 'agents-access')
WHERE 'agents-access' = ANY(default_org_member_roles);
