-- WARNING: this rollback is lossy. If an admin later revoked
-- agents-access from a specific org, rolling back will re-grant the
-- site-wide role (which covers ALL orgs) to any user who still holds
-- agents-access in at least one org.

-- Step 1: Move agents-access back to site-level for any user who has it in any org.
UPDATE users
SET rbac_roles = array_append(rbac_roles, 'agents-access')
WHERE id IN (
    SELECT DISTINCT user_id FROM organization_members
    WHERE 'agents-access' = ANY(roles)
)
AND NOT ('agents-access' = ANY(rbac_roles));

-- Step 2: Remove from org memberships.
UPDATE organization_members
SET roles = array_remove(roles, 'agents-access')
WHERE 'agents-access' = ANY(roles);
