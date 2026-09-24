ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_unrestricted:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_unrestricted:use';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:use';

-- Built-in roles take precedence during expansion. Do not reinterpret an
-- existing custom role as an unrestricted Gateway grant.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM custom_roles
        WHERE name IN ('ai-gateway-unrestricted', 'organization-ai-gateway-unrestricted')
    ) THEN
        RAISE EXCEPTION 'AI Gateway role name conflicts with an existing custom role'
            USING HINT = 'Rename the custom role and its assignments before upgrading: ai-gateway-unrestricted or organization-ai-gateway-unrestricted.';
    END IF;
END;
$$;

UPDATE organizations
SET default_org_member_roles = array_append(
    default_org_member_roles,
    'organization-ai-gateway-unrestricted'
)
WHERE NOT ('organization-ai-gateway-unrestricted' = ANY(default_org_member_roles));
