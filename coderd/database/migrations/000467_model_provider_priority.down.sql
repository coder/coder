-- Restore chat_model_configs.provider_config_id from the lowest-priority
-- attachment before dropping the join table.
UPDATE chat_model_configs cmc
SET provider_config_id = sub.provider_config_id
FROM (
    SELECT DISTINCT ON (cmpc.model_config_id)
        cmpc.model_config_id,
        cmpc.provider_config_id
    FROM chat_model_provider_configs cmpc
    JOIN chat_providers cp ON cp.id = cmpc.provider_config_id
    ORDER BY cmpc.model_config_id, cp.enabled DESC, cmpc.priority ASC
) sub
WHERE cmc.id = sub.model_config_id;

DROP TABLE IF EXISTS chat_model_provider_configs;
