DROP TABLE chat_user_model_overrides;
DROP TABLE chat_organization_model_overrides;

ALTER TABLE chat_model_configs
    DROP CONSTRAINT chat_model_configs_organization_id_id_key;
