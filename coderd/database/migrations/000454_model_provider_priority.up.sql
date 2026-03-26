CREATE TABLE chat_model_provider_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_config_id UUID NOT NULL REFERENCES chat_model_configs(id) ON DELETE CASCADE,
    provider_config_id UUID NOT NULL REFERENCES chat_providers(id) ON DELETE CASCADE,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (model_config_id, provider_config_id),
    UNIQUE (model_config_id, priority)
);

CREATE INDEX idx_chat_model_provider_configs_order
    ON chat_model_provider_configs (model_config_id, priority);

INSERT INTO chat_model_provider_configs (model_config_id, provider_config_id, priority)
SELECT
    cmc.id,
    cp.id,
    0
FROM
    chat_model_configs cmc
JOIN
    chat_providers cp ON cp.provider = cmc.provider AND cp.enabled = TRUE
WHERE
    cmc.deleted = FALSE;

DROP INDEX IF EXISTS idx_chat_providers_one_enabled_per_family;
