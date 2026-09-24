ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_unrestricted:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_gateway_unrestricted:use';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:use';

-- Built-in roles take precedence during expansion. Do not reinterpret an
-- existing custom role as an unrestricted Gateway grant.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM custom_roles
        WHERE name = 'ai-gateway-unrestricted'
    ) THEN
        RAISE EXCEPTION 'AI Gateway role name conflicts with an existing custom role'
            USING HINT = 'Rename the custom role and its assignments before upgrading: ai-gateway-unrestricted.';
    END IF;
END;
$$;

-- Preserve unrestricted Gateway access for existing users, including service
-- accounts and inactive users. Future users receive no automatic explicit grant.
UPDATE users
SET rbac_roles = array_append(rbac_roles, 'ai-gateway-unrestricted')
WHERE NOT deleted
    AND NOT is_system
    AND NOT ('ai-gateway-unrestricted' = ANY(rbac_roles));
