ALTER TABLE chat_model_configs
	ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
	ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
	ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
	ADD COLUMN legacy_model_config_id uuid,
	ADD COLUMN inherits_legacy_config boolean NOT NULL DEFAULT false,
	ADD CONSTRAINT chat_model_configs_user_acl_is_object
		CHECK (jsonb_typeof(user_acl) = 'object'),
	ADD CONSTRAINT chat_model_configs_group_acl_is_object
		CHECK (jsonb_typeof(group_acl) = 'object'),
	ADD CONSTRAINT chat_model_configs_coexistence_row_form
		CHECK (
			(
				organization_id IS NULL
				AND legacy_model_config_id IS NULL
				AND inherits_legacy_config = false
			)
			OR (
				organization_id IS NOT NULL
				AND (
					(legacy_model_config_id IS NOT NULL)
					OR (
						legacy_model_config_id IS NULL
						AND inherits_legacy_config = false
					)
				)
			)
		);

CREATE TABLE chat_model_config_org_default_inheritance (
	organization_id uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
	inherits_legacy_default boolean NOT NULL
);

DROP INDEX idx_chat_model_configs_single_default;

CREATE UNIQUE INDEX idx_chat_model_configs_single_global_default
	ON chat_model_configs ((1))
	WHERE organization_id IS NULL
		AND is_default = true
		AND deleted = false;

CREATE UNIQUE INDEX idx_chat_model_configs_single_organization_default
	ON chat_model_configs (organization_id)
	WHERE organization_id IS NOT NULL
		AND is_default = true
		AND deleted = false;

CREATE UNIQUE INDEX idx_chat_model_configs_organization_legacy_model_config
	ON chat_model_configs (organization_id, legacy_model_config_id)
	WHERE organization_id IS NOT NULL
		AND legacy_model_config_id IS NOT NULL;

CREATE INDEX idx_chat_model_configs_organization_id
	ON chat_model_configs (organization_id)
	WHERE organization_id IS NOT NULL;

-- Existing organizations receive historical copies so lineage remains complete
-- for every legacy row, including soft-deleted model configurations.
INSERT INTO chat_model_configs (
	id,
	model,
	display_name,
	created_by,
	updated_by,
	enabled,
	is_default,
	deleted,
	deleted_at,
	created_at,
	updated_at,
	context_limit,
	compression_threshold,
	options,
	ai_provider_id,
	organization_id,
	user_acl,
	group_acl,
	legacy_model_config_id,
	inherits_legacy_config
)
SELECT
	gen_random_uuid(),
	cmc.model,
	cmc.display_name,
	cmc.created_by,
	cmc.updated_by,
	cmc.enabled,
	cmc.is_default,
	cmc.deleted,
	cmc.deleted_at,
	cmc.created_at,
	cmc.updated_at,
	cmc.context_limit,
	cmc.compression_threshold,
	cmc.options,
	cmc.ai_provider_id,
	o.id,
	'{}'::jsonb,
	jsonb_build_object(o.id::text, jsonb_build_array('read')),
	cmc.id,
	true
FROM chat_model_configs cmc
CROSS JOIN organizations o
WHERE cmc.organization_id IS NULL
	AND o.deleted = false;

INSERT INTO chat_model_config_org_default_inheritance (
	organization_id,
	inherits_legacy_default
)
SELECT
	o.id,
	true
FROM organizations o
WHERE o.deleted = false;

-- Organizations created during coexistence receive only the current global
-- catalog. Historical deleted rows remain limited to organizations that
-- existed when coexistence began.
CREATE FUNCTION provision_chat_model_configs_for_organization()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF NEW.deleted = false THEN
		INSERT INTO chat_model_configs (
			id,
			model,
			display_name,
			created_by,
			updated_by,
			enabled,
			is_default,
			deleted,
			deleted_at,
			created_at,
			updated_at,
			context_limit,
			compression_threshold,
			options,
			ai_provider_id,
			organization_id,
			user_acl,
			group_acl,
			legacy_model_config_id,
			inherits_legacy_config
		)
		SELECT
			gen_random_uuid(),
			cmc.model,
			cmc.display_name,
			cmc.created_by,
			cmc.updated_by,
			cmc.enabled,
			cmc.is_default,
			cmc.deleted,
			cmc.deleted_at,
			cmc.created_at,
			cmc.updated_at,
			cmc.context_limit,
			cmc.compression_threshold,
			cmc.options,
			cmc.ai_provider_id,
			NEW.id,
			'{}'::jsonb,
			jsonb_build_object(NEW.id::text, jsonb_build_array('read')),
			cmc.id,
			true
		FROM chat_model_configs cmc
		WHERE cmc.organization_id IS NULL
			AND cmc.deleted = false;

		INSERT INTO chat_model_config_org_default_inheritance (
			organization_id,
			inherits_legacy_default
		) VALUES (
			NEW.id,
			true
		);
	END IF;

	RETURN NEW;
END;
$$;

CREATE TRIGGER provision_chat_model_configs_after_organization_insert
	AFTER INSERT ON organizations
	FOR EACH ROW
	EXECUTE FUNCTION provision_chat_model_configs_for_organization();
