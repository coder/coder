-- Enum values are retained on rollback, as PostgreSQL cannot remove them
-- without recreating the type and rewriting existing API keys.
-- Remove all explicit grants, including those assigned after the upgrade.
UPDATE users
SET rbac_roles = array_remove(rbac_roles, 'ai-gateway-unrestricted')
WHERE 'ai-gateway-unrestricted' = ANY(rbac_roles);
