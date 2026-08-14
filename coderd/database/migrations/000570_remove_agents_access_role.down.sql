-- Restore effective chat access when rolling back to a version that requires
-- the role. The previous assignments cannot be recovered after migration up.
UPDATE organization_members
SET roles = array_append(roles, 'agents-access')
WHERE NOT ('agents-access' = ANY(roles));

UPDATE organizations
SET default_org_member_roles = array_append(default_org_member_roles, 'agents-access')
WHERE NOT ('agents-access' = ANY(default_org_member_roles));
