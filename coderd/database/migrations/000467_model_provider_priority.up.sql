CREATE TABLE chat_model_provider_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_config_id UUID NOT NULL REFERENCES chat_model_configs(id) ON DELETE CASCADE,
    provider_config_id UUID NOT NULL REFERENCES chat_providers(id) ON DELETE CASCADE,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (model_config_id, provider_config_id),
    -- Handlers replace attachment ordering with DELETE + INSERT, so keep
    -- priorities unique per model instead of relying on partial updates.
    UNIQUE (model_config_id, priority)
);

CREATE INDEX idx_chat_model_provider_configs_order
    ON chat_model_provider_configs (model_config_id, priority);

-- Preserve explicit provider_config_id bindings when present and enabled.
INSERT INTO chat_model_provider_configs (model_config_id, provider_config_id, priority)
SELECT
    cmc.id,
    cmc.provider_config_id,
    0
FROM
    chat_model_configs cmc
JOIN
    chat_providers cp ON cp.id = cmc.provider_config_id AND cp.enabled = TRUE
WHERE
    cmc.deleted = FALSE
    AND cmc.provider_config_id IS NOT NULL;

-- Fan out family-level providers for models without an explicit binding,
-- ordered by provider creation time.
INSERT INTO chat_model_provider_configs (model_config_id, provider_config_id, priority)
SELECT
    cmc.id,
    cp.id,
    ROW_NUMBER() OVER (PARTITION BY cmc.id ORDER BY cp.created_at) - 1
FROM
    chat_model_configs cmc
JOIN
    chat_providers cp ON cp.provider = cmc.provider AND cp.enabled = TRUE
WHERE
    cmc.deleted = FALSE
    AND cmc.provider_config_id IS NULL
ON CONFLICT (model_config_id, provider_config_id) DO NOTHING;
