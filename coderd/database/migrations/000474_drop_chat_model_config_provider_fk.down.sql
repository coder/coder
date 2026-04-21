-- Restore placeholder provider rows before re-adding the provider FK.
--
-- The companion up migration dropped chat_model_configs.provider's foreign
-- key, so historical model-config rows can outlive a deleted provider row.
-- These backfilled providers are deliberately disabled stubs with empty
-- credential fields, which lets rollback restore referential integrity
-- without re-enabling a provider. This insert depends on the current
-- provider whitelist still admitting every historical
-- chat_model_configs.provider value, and on the omitted columns keeping
-- compatible defaults. Operators restoring a real provider should update the
-- stub row, including credential-policy flags such as
-- central_api_key_enabled, before enabling it, rather than insert a second
-- row with the same provider name.
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
