-- The organization-ai-gateway-access role now carries the AI Bridge
-- interception create/update permissions that were previously granted by
-- the site member role. Append it to every organization's default member
-- roles so existing members keep AI Gateway access after the permission
-- moves. Guarded for idempotency in case an organization already has it.
UPDATE organizations
SET default_org_member_roles = array_append(default_org_member_roles, 'organization-ai-gateway-access')
WHERE NOT ('organization-ai-gateway-access' = ANY (default_org_member_roles));
