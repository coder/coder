ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'mcp_server_config:delete';

ALTER TABLE mcp_server_configs
    ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM organizations WHERE is_default = true) THEN
        RAISE EXCEPTION 'cannot scope mcp_server_configs: no default organization exists';
    END IF;
END $$;

-- Originals move to the default organization with credentials intact:
-- it succeeds the deployment scope, so its operator-appointed admins
-- take over the inherited secrets. Copies below never get credentials.
UPDATE mcp_server_configs
SET organization_id = (SELECT id FROM organizations WHERE is_default = true LIMIT 1);

ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_slug_key;

CREATE TEMP TABLE mcp_server_config_org_map (
    old_id UUID NOT NULL,
    organization_id UUID NOT NULL,
    new_id UUID NOT NULL,
    PRIMARY KEY (old_id, organization_id),
    UNIQUE (new_id)
) ON COMMIT DROP;

INSERT INTO mcp_server_config_org_map (old_id, organization_id, new_id)
SELECT config.id, organization.id, gen_random_uuid()
FROM mcp_server_configs AS config
CROSS JOIN organizations AS organization
WHERE organization.deleted = false
    AND organization.is_default = false;

INSERT INTO mcp_server_configs (
    id, organization_id, display_name, slug, description, icon_url, transport,
    url, auth_type, oauth2_client_id, oauth2_client_secret,
    oauth2_client_secret_key_id, oauth2_auth_url, oauth2_token_url,
    oauth2_revocation_url, oauth2_scopes, api_key_header, api_key_value,
    api_key_value_key_id, custom_headers, custom_headers_key_id,
    tool_allow_list, tool_deny_list, availability, enabled, model_intent,
    allow_in_plan_mode, forward_coder_headers, created_by, updated_by,
    created_at, updated_at
)
SELECT
    mapping.new_id,
    mapping.organization_id,
    config.display_name,
    config.slug,
    config.description,
    config.icon_url,
    config.transport,
    config.url,
    config.auth_type,
    -- Never copy admin-entered credentials: the copy's admins could repoint
    -- its URL and reuse the inherited secret. OAuth identity is cleared for
    -- every auth type because the API stores it regardless of auth_type.
    '',
    '',
    NULL,
    config.oauth2_auth_url,
    config.oauth2_token_url,
    config.oauth2_revocation_url,
    config.oauth2_scopes,
    config.api_key_header,
    '',
    NULL,
    '{}',
    NULL,
    config.tool_allow_list,
    config.tool_deny_list,
    config.availability,
    -- Copies that lost required credentials start disabled so each
    -- organization's admin re-enters them deliberately.
    CASE
        WHEN config.auth_type IN ('oauth2', 'api_key', 'custom_headers')
            -- dbcrypt stores every nonblank value as ciphertext, so this
            -- plaintext check only applies to unencrypted rows. Encrypted
            -- rows fall back to the auth_type decision above; header
            -- values are only sent for auth_type custom_headers anyway.
            OR (config.custom_headers_key_id IS NULL
                AND config.custom_headers NOT IN ('', '{}'))
            THEN false
        ELSE config.enabled
    END,
    config.model_intent,
    config.allow_in_plan_mode,
    config.forward_coder_headers,
    config.created_by,
    config.updated_by,
    config.created_at,
    config.updated_at
FROM mcp_server_config_org_map AS mapping
JOIN mcp_server_configs AS config ON config.id = mapping.old_id;

UPDATE chats AS chat
SET mcp_server_ids = remapped.ids
FROM (
    SELECT
        source.id,
        COALESCE(
            array_agg(COALESCE(mapping.new_id, item.config_id) ORDER BY item.position)
                FILTER (WHERE item.config_id IS NOT NULL),
            '{}'::UUID[]
        ) AS ids
    FROM chats AS source
    LEFT JOIN LATERAL unnest(source.mcp_server_ids) WITH ORDINALITY
        AS item(config_id, position) ON true
    LEFT JOIN mcp_server_config_org_map AS mapping
        ON mapping.old_id = item.config_id
        AND mapping.organization_id = source.organization_id
    WHERE source.organization_id != (
        SELECT id FROM organizations WHERE is_default = true LIMIT 1
    )
    GROUP BY source.id
) AS remapped
WHERE remapped.id = chat.id;

ALTER TABLE mcp_server_configs
    ALTER COLUMN organization_id SET NOT NULL,
    ADD CONSTRAINT mcp_server_configs_organization_id_slug_key UNIQUE (organization_id, slug);

CREATE INDEX idx_mcp_server_configs_organization_id
    ON mcp_server_configs (organization_id);

-- Pre-scoping replicas resolve configs globally and can write another org's
-- config ID into a chat during a rolling upgrade. Remap to the same-slug
-- local config (dropping unmappable IDs); remove once those replicas are gone.
CREATE FUNCTION remap_chat_mcp_server_ids_to_chat_org()
    RETURNS TRIGGER AS
$$
BEGIN
    IF NEW.mcp_server_ids IS NULL OR cardinality(NEW.mcp_server_ids) = 0 THEN
        RETURN NEW;
    END IF;
    -- same_org_config.id is non-NULL only for the foreign, remappable case,
    -- so COALESCE keeps missing and same-org IDs as written.
    SELECT COALESCE(
        array_agg(
            COALESCE(same_org_config.id, item.config_id)
            ORDER BY item.position
        ) FILTER (
            WHERE config.id IS NULL
                OR config.organization_id = NEW.organization_id
                OR same_org_config.id IS NOT NULL
        ),
        '{}'::uuid[]
    )
    INTO NEW.mcp_server_ids
    FROM unnest(NEW.mcp_server_ids) WITH ORDINALITY AS item(config_id, position)
    LEFT JOIN mcp_server_configs AS config ON config.id = item.config_id
    LEFT JOIN mcp_server_configs AS same_org_config
        ON config.organization_id != NEW.organization_id
        AND same_org_config.organization_id = NEW.organization_id
        AND same_org_config.slug = config.slug;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER remap_chat_mcp_server_ids
    BEFORE INSERT OR UPDATE OF mcp_server_ids ON chats FOR EACH ROW
    EXECUTE PROCEDURE remap_chat_mcp_server_ids_to_chat_org();

COMMENT ON TRIGGER remap_chat_mcp_server_ids ON chats IS
    'Rolling-upgrade compatibility: remaps config IDs written by pre-organization-scoping replicas to the chat organization''s same-slug config.';
