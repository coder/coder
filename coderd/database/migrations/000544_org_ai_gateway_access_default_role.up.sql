-- Append organization-ai-gateway-access to every organization's default
-- member roles so existing members hold the role's AI Bridge interception
-- perms. Guarded for idempotency in case an organization already has it.
UPDATE organizations
SET default_org_member_roles = array_append(default_org_member_roles, 'organization-ai-gateway-access')
WHERE NOT ('organization-ai-gateway-access' = ANY (default_org_member_roles));
