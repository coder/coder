-- Transition 'agents-access' from a site-wide role to a per-org role.

-- For every user who has 'agents-access' in users.rbac_roles,
-- grant the org-scoped role in each org they belong to.
UPDATE organization_members
SET roles = array_append(roles, 'agents-access')
WHERE user_id IN (
    SELECT id FROM users
    WHERE 'agents-access' = ANY(rbac_roles)
)
AND NOT ('agents-access' = ANY(roles));

-- Remove 'agents-access' from site-level roles.
UPDATE users
SET rbac_roles = array_remove(rbac_roles, 'agents-access')
WHERE 'agents-access' = ANY(rbac_roles);
