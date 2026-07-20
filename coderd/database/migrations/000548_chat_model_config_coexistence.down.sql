DROP TRIGGER provision_chat_model_configs_after_organization_insert ON organizations;
DROP FUNCTION provision_chat_model_configs_for_organization();

DELETE FROM chat_model_configs
WHERE organization_id IS NOT NULL;

DROP TABLE chat_model_config_org_default_inheritance;

DROP INDEX idx_chat_model_configs_organization_id;
DROP INDEX idx_chat_model_configs_organization_legacy_model_config;
DROP INDEX idx_chat_model_configs_single_organization_default;
DROP INDEX idx_chat_model_configs_single_global_default;

ALTER TABLE chat_model_configs
	DROP CONSTRAINT chat_model_configs_coexistence_row_form,
	DROP CONSTRAINT chat_model_configs_group_acl_is_object,
	DROP CONSTRAINT chat_model_configs_user_acl_is_object,
	DROP COLUMN inherits_legacy_config,
	DROP COLUMN legacy_model_config_id,
	DROP COLUMN group_acl,
	DROP COLUMN user_acl,
	DROP COLUMN organization_id;

CREATE UNIQUE INDEX idx_chat_model_configs_single_default
	ON chat_model_configs ((1))
	WHERE is_default = true
		AND deleted = false;
