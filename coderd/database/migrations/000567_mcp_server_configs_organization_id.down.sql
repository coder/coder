-- Drop the remap trigger before the chat update below: it would rewrite the
-- restored default-organization IDs back to the per-organization copies.
DROP TRIGGER IF EXISTS remap_chat_mcp_server_ids ON chats;
DROP FUNCTION IF EXISTS remap_chat_mcp_server_ids_to_chat_org();

CREATE TEMP TABLE mcp_server_config_restore_map (
    config_id UUID PRIMARY KEY,
    default_config_id UUID NOT NULL
) ON COMMIT DROP;

INSERT INTO mcp_server_config_restore_map (config_id, default_config_id)
SELECT config.id, default_config.id
FROM mcp_server_configs AS config
JOIN mcp_server_configs AS default_config
    ON default_config.slug = config.slug
    AND default_config.organization_id = (
        SELECT id FROM organizations WHERE is_default = true LIMIT 1
    )
WHERE config.organization_id != default_config.organization_id;

UPDATE chats AS chat
SET mcp_server_ids = remapped.ids
FROM (
    SELECT
        source.id,
        COALESCE(
            array_agg(COALESCE(mapping.default_config_id, item.config_id) ORDER BY item.position)
                FILTER (WHERE item.config_id IS NOT NULL),
            '{}'::UUID[]
        ) AS ids
    FROM chats AS source
    LEFT JOIN LATERAL unnest(source.mcp_server_ids) WITH ORDINALITY
        AS item(config_id, position) ON true
    LEFT JOIN mcp_server_config_restore_map AS mapping
        ON mapping.config_id = item.config_id
    GROUP BY source.id
) AS remapped
WHERE remapped.id = chat.id;

DELETE FROM mcp_server_configs
WHERE organization_id != (
    SELECT id FROM organizations WHERE is_default = true LIMIT 1
);

DROP INDEX idx_mcp_server_configs_organization_id;

ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_organization_id_slug_key,
    DROP COLUMN organization_id,
    ADD CONSTRAINT mcp_server_configs_slug_key UNIQUE (slug);

-- Enum values cannot be removed safely from api_key_scope.
