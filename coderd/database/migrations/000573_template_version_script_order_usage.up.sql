ALTER TABLE template_version_terraform_values
	ADD COLUMN script_order_data_source_count INTEGER NOT NULL DEFAULT 0,
	ADD COLUMN script_order_rule_count INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN template_version_terraform_values.script_order_data_source_count IS
	'Number of coder_script_order data source declarations in the template configuration.';

COMMENT ON COLUMN template_version_terraform_values.script_order_rule_count IS
	'Number of rule blocks across coder_script_order declarations in the template configuration.';
