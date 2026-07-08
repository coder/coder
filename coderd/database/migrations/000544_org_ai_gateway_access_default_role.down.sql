UPDATE organizations
SET default_org_member_roles = array_remove(default_org_member_roles, 'organization-ai-gateway-access');
