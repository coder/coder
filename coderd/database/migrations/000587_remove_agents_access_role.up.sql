UPDATE users
SET rbac_roles = array_remove(rbac_roles, 'agents-access')
WHERE 'agents-access' = ANY(rbac_roles);

UPDATE organization_members
SET roles = array_remove(roles, 'agents-access')
WHERE 'agents-access' = ANY(roles);

UPDATE organizations
SET default_org_member_roles = array_remove(default_org_member_roles, 'agents-access')
WHERE 'agents-access' = ANY(default_org_member_roles);
