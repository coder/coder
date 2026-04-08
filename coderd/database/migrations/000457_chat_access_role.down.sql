-- Remove 'agents-access' from all users who have it.
UPDATE users
SET rbac_roles = array_remove(rbac_roles, 'agents-access')
WHERE 'agents-access' = ANY(rbac_roles);
