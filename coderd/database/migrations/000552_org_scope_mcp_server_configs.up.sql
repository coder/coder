ALTER TABLE mcp_server_configs
    ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE mcp_server_configs DROP CONSTRAINT mcp_server_configs_slug_key;
CREATE TEMP TABLE mcp_server_config_org_map (
    legacy_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    config_id uuid NOT NULL,
    PRIMARY KEY (legacy_id, organization_id),
    UNIQUE (config_id)
) ON COMMIT DROP;
INSERT INTO mcp_server_config_org_map
SELECT msc.id, o.id, gen_random_uuid()
FROM mcp_server_configs msc CROSS JOIN organizations o;
INSERT INTO mcp_server_configs (
    id, display_name, slug, description, icon_url, transport, url, auth_type,
    oauth2_client_id, oauth2_client_secret, oauth2_client_secret_key_id,
    oauth2_auth_url, oauth2_token_url, oauth2_scopes, api_key_header,
    api_key_value, api_key_value_key_id, custom_headers, custom_headers_key_id,
    tool_allow_list, tool_deny_list, availability, enabled, created_by,
    updated_by, created_at, updated_at, model_intent, allow_in_plan_mode,
    forward_coder_headers, oauth2_revocation_url, organization_id
)
SELECT map.config_id, msc.display_name, msc.slug, msc.description, msc.icon_url,
    msc.transport, msc.url, msc.auth_type, msc.oauth2_client_id,
    msc.oauth2_client_secret, msc.oauth2_client_secret_key_id,
    msc.oauth2_auth_url, msc.oauth2_token_url, msc.oauth2_scopes,
    msc.api_key_header, msc.api_key_value, msc.api_key_value_key_id,
    msc.custom_headers, msc.custom_headers_key_id, msc.tool_allow_list,
    msc.tool_deny_list, msc.availability, msc.enabled, msc.created_by,
    msc.updated_by, msc.created_at, msc.updated_at, msc.model_intent,
    msc.allow_in_plan_mode, msc.forward_coder_headers,
    msc.oauth2_revocation_url, map.organization_id
FROM mcp_server_configs msc
JOIN mcp_server_config_org_map map ON map.legacy_id = msc.id
WHERE msc.organization_id IS NULL;
UPDATE chats c SET mcp_server_ids = mapped.ids
FROM (
    SELECT c2.id,
        COALESCE(array_agg(map.config_id ORDER BY item.position)
            FILTER (WHERE map.config_id IS NOT NULL), '{}'::uuid[]) AS ids
    FROM chats c2
    LEFT JOIN LATERAL unnest(c2.mcp_server_ids) WITH ORDINALITY
        AS item(legacy_id, position) ON true
    LEFT JOIN mcp_server_config_org_map map
        ON map.legacy_id = item.legacy_id
        AND map.organization_id = c2.organization_id
    GROUP BY c2.id
) mapped WHERE mapped.id = c.id;
INSERT INTO mcp_server_user_tokens (
    id, mcp_server_config_id, user_id, access_token, access_token_key_id,
    refresh_token, refresh_token_key_id, token_type, expiry, created_at,
    updated_at, oauth_refresh_failure_reason
)
SELECT gen_random_uuid(), map.config_id, tok.user_id, tok.access_token,
    tok.access_token_key_id, tok.refresh_token, tok.refresh_token_key_id,
    tok.token_type, tok.expiry, tok.created_at, tok.updated_at,
    tok.oauth_refresh_failure_reason
FROM mcp_server_user_tokens tok
JOIN mcp_server_config_org_map map ON map.legacy_id = tok.mcp_server_config_id
JOIN organization_members om ON om.user_id = tok.user_id
    AND om.organization_id = map.organization_id
JOIN organizations o ON o.id = om.organization_id AND o.deleted = false;
DELETE FROM mcp_server_configs WHERE organization_id IS NULL;
ALTER TABLE mcp_server_configs
    ALTER COLUMN organization_id SET NOT NULL,
    ADD CONSTRAINT mcp_server_configs_id_organization_id_key UNIQUE (id, organization_id),
    ADD CONSTRAINT mcp_server_configs_organization_id_slug_key UNIQUE (organization_id, slug);
DROP INDEX idx_mcp_server_configs_enabled;
DROP INDEX idx_mcp_server_configs_forced;
CREATE INDEX idx_mcp_server_configs_organization_id ON mcp_server_configs (organization_id);
CREATE INDEX idx_mcp_server_configs_enabled ON mcp_server_configs (organization_id, enabled) WHERE enabled = true;
CREATE INDEX idx_mcp_server_configs_forced ON mcp_server_configs (organization_id, enabled, availability)
    WHERE enabled = true AND availability = 'force_on';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:*';
