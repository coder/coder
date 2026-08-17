-- The 'agents-access' (Coder Agents User) role is removed. Chat access is
-- now part of the implicit organization-member permission floor. Stale
-- stored grants are inert for authorization (rolestore.Expand skips role
-- names it cannot resolve), but they still render as raw strings in role
-- UIs and fail validation on role-update paths that re-submit the existing
-- set, so scrub them from every column that can store role names.
UPDATE organization_members
SET roles = array_remove(roles, 'agents-access')
WHERE 'agents-access' = ANY(roles);

-- Site-level grants were already migrated to org scope by 000475, but remove
-- any stragglers defensively.
UPDATE users
SET rbac_roles = array_remove(rbac_roles, 'agents-access')
WHERE 'agents-access' = ANY(rbac_roles);

-- default_org_member_roles is unioned into every member's effective roles at
-- authorization time, so a stale name here would also fail role expansion.
UPDATE organizations
SET default_org_member_roles = array_remove(default_org_member_roles, 'agents-access')
WHERE 'agents-access' = ANY(default_org_member_roles);
