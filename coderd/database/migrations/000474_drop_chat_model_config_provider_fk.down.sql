INSERT INTO chat_providers (provider, enabled)
SELECT DISTINCT
    cmc.provider,
    FALSE
FROM
    chat_model_configs cmc
LEFT JOIN
    chat_providers cp ON cp.provider = cmc.provider
WHERE
    cp.provider IS NULL;

ALTER TABLE chat_model_configs
    ADD CONSTRAINT chat_model_configs_provider_fkey
    FOREIGN KEY (provider) REFERENCES chat_providers(provider) ON DELETE CASCADE;
