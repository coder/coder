CREATE TEMP TABLE mcp_server_config_restore_organization (id uuid PRIMARY KEY) ON COMMIT DROP;
INSERT INTO mcp_server_config_restore_organization
SELECT id FROM organizations WHERE deleted = false
ORDER BY CASE WHEN is_default THEN 0 ELSE 1 END, created_at, id LIMIT 1;
CREATE TEMP TABLE mcp_server_config_restore_map (
    config_id uuid PRIMARY KEY, retained_config_id uuid NOT NULL
) ON COMMIT DROP;
WITH ranked AS (
    SELECT msc.id, first_value(msc.id) OVER (
        PARTITION BY msc.slug ORDER BY
        CASE WHEN msc.organization_id = ro.id THEN 0 ELSE 1 END,
        CASE WHEN o.deleted = false THEN 0 ELSE 1 END,
        o.created_at, o.id, msc.created_at, msc.id
    ) retained_config_id
    FROM mcp_server_configs msc
    JOIN organizations o ON o.id = msc.organization_id
    LEFT JOIN mcp_server_config_restore_organization ro ON true
)
INSERT INTO mcp_server_config_restore_map SELECT id, retained_config_id FROM ranked;
UPDATE chats c SET mcp_server_ids = mapped.ids
FROM (
    SELECT c2.id,
        COALESCE(array_agg(map.retained_config_id ORDER BY item.position)
            FILTER (WHERE map.retained_config_id IS NOT NULL), '{}'::uuid[]) ids
    FROM chats c2
    LEFT JOIN LATERAL unnest(c2.mcp_server_ids) WITH ORDINALITY
        AS item(config_id, position) ON true
    LEFT JOIN mcp_server_config_restore_map map ON map.config_id = item.config_id
    GROUP BY c2.id
) mapped WHERE mapped.id = c.id;
CREATE TEMP TABLE mcp_server_token_restore_winners (
    token_id uuid PRIMARY KEY, retained_config_id uuid NOT NULL
) ON COMMIT DROP;
WITH ranked AS (
    SELECT tok.id, map.retained_config_id, row_number() OVER (
        PARTITION BY map.retained_config_id, tok.user_id ORDER BY
        CASE WHEN tok.mcp_server_config_id = map.retained_config_id THEN 0 ELSE 1 END,
        CASE WHEN msc.organization_id = ro.id THEN 0 ELSE 1 END,
        CASE WHEN o.deleted = false THEN 0 ELSE 1 END,
        o.created_at, o.id, tok.updated_at DESC, tok.id
    ) token_rank
    FROM mcp_server_user_tokens tok
    JOIN mcp_server_config_restore_map map ON map.config_id = tok.mcp_server_config_id
    JOIN mcp_server_configs msc ON msc.id = tok.mcp_server_config_id
    JOIN organizations o ON o.id = msc.organization_id
    LEFT JOIN mcp_server_config_restore_organization ro ON true
)
INSERT INTO mcp_server_token_restore_winners
SELECT id, retained_config_id FROM ranked WHERE token_rank = 1;
DELETE FROM mcp_server_user_tokens tok WHERE NOT EXISTS (
    SELECT 1 FROM mcp_server_token_restore_winners w WHERE w.token_id = tok.id
);
UPDATE mcp_server_user_tokens tok SET mcp_server_config_id = w.retained_config_id
FROM mcp_server_token_restore_winners w
WHERE w.token_id = tok.id AND tok.mcp_server_config_id != w.retained_config_id;
DELETE FROM mcp_server_configs msc WHERE NOT EXISTS (
    SELECT 1 FROM mcp_server_config_restore_map map
    WHERE map.config_id = msc.id AND map.retained_config_id = msc.id
);
DROP INDEX idx_mcp_server_configs_organization_id;
DROP INDEX idx_mcp_server_configs_enabled;
DROP INDEX idx_mcp_server_configs_forced;
ALTER TABLE mcp_server_configs
    DROP CONSTRAINT mcp_server_configs_id_organization_id_key,
    DROP CONSTRAINT mcp_server_configs_organization_id_slug_key,
    DROP COLUMN organization_id,
    ADD CONSTRAINT mcp_server_configs_slug_key UNIQUE (slug);
CREATE INDEX idx_mcp_server_configs_enabled ON mcp_server_configs (enabled) WHERE enabled = true;
CREATE INDEX idx_mcp_server_configs_forced ON mcp_server_configs (enabled, availability)
    WHERE enabled = true AND availability = 'force_on';
