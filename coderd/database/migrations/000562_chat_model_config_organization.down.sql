ALTER TABLE chat_model_configs
    DROP CONSTRAINT chat_model_configs_user_acl_is_object,
    DROP CONSTRAINT chat_model_configs_group_acl_is_object,
    DROP COLUMN user_acl,
    DROP COLUMN group_acl;

DROP INDEX idx_chat_model_configs_single_default;
CREATE UNIQUE INDEX idx_chat_model_configs_single_default
    ON chat_model_configs ((1))
    WHERE is_default = true AND deleted = false;

DROP INDEX idx_chat_model_configs_organization_id;

ALTER TABLE chat_model_configs DROP COLUMN organization_id;
