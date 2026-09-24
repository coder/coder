-- Enum values are retained on rollback, as PostgreSQL cannot remove them
-- without recreating the type and rewriting existing API keys.
UPDATE organizations
SET default_org_member_roles = array_remove(
    default_org_member_roles,
    'organization-ai-gateway-unrestricted'
)
WHERE 'organization-ai-gateway-unrestricted' = ANY(default_org_member_roles);
