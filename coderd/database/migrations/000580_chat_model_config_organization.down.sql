ALTER TABLE chat_model_configs
    DROP CONSTRAINT chat_model_configs_user_acl_is_object,
    DROP CONSTRAINT chat_model_configs_group_acl_is_object,
    DROP COLUMN user_acl,
    DROP COLUMN group_acl;

-- Rollback permanently demotes defaults outside the default organization.
-- Reapplying the up migration does not restore those default selections.
-- The old schema permits only one deployment-wide default. Preserve the
-- default organization's selection and demote defaults from other orgs.
UPDATE chat_model_configs
SET is_default = false,
    updated_at = NOW()
WHERE is_default = true
    AND deleted = false
    AND organization_id IS DISTINCT FROM (
        SELECT id FROM organizations WHERE is_default = true LIMIT 1
    );

DROP INDEX idx_chat_model_configs_single_default;
CREATE UNIQUE INDEX idx_chat_model_configs_single_default
    ON chat_model_configs ((1))
    WHERE is_default = true AND deleted = false;

DROP INDEX idx_chat_model_configs_organization_id;

ALTER TABLE chat_model_configs DROP COLUMN organization_id;
