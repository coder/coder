-- Deleted legacy custom roles named agents-access are not restored.
UPDATE organizations
SET default_org_member_roles = array_remove(default_org_member_roles, 'agents-access')
WHERE 'agents-access' = ANY(default_org_member_roles);

UPDATE organization_members
SET roles = array_remove(roles, 'agents-access')
WHERE 'agents-access' = ANY(roles);
