-- The 'agents-access' (Coder Agents User) role is removed. Chat access is
-- now part of the implicit organization-member permission floor, so stored
-- grants of the role must be scrubbed: once the built-in role no longer
-- exists, expanding a stale role string would fail authorization for the
-- affected user.
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
