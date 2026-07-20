INSERT INTO organizations (
	id,
	name,
	description,
	created_at,
	updated_at,
	is_default,
	display_name,
	icon,
	deleted,
	default_org_member_roles
) VALUES
	(
		'84f7df20-0d04-4dd7-97e9-7c276c337a01',
		'model-coexistence-org',
		'',
		'2026-01-01 00:00:00+00',
		'2026-01-01 00:00:00+00',
		false,
		'Model Coexistence Org',
		'',
		false,
		'{}'
	),
	(
		'84f7df20-0d04-4dd7-97e9-7c276c337a02',
		'second-model-coexistence-org',
		'',
		'2026-01-01 00:00:01+00',
		'2026-01-01 00:00:01+00',
		false,
		'Second Model Coexistence Org',
		'',
		false,
		'{}'
	),
	(
		'84f7df20-0d04-4dd7-97e9-7c276c337a03',
		'deleted-model-coexistence-org',
		'',
		'2026-01-01 00:00:02+00',
		'2026-01-01 00:00:02+00',
		false,
		'Deleted Model Coexistence Org',
		'',
		true,
		'{}'
	);

UPDATE chat_model_configs
SET is_default = false
WHERE is_default = true
	AND deleted = false;

INSERT INTO chat_model_configs (
	id,
	model,
	display_name,
	enabled,
	is_default,
	deleted,
	deleted_at,
	context_limit,
	compression_threshold,
	options,
	ai_provider_id,
	created_at,
	updated_at
) VALUES
	(
		'84f7df20-0d04-4dd7-97e9-7c276c337a11',
		'model-coexistence-active',
		'Model Coexistence Active',
		true,
		true,
		false,
		null,
		200000,
		70,
		'{}'::jsonb,
		'8e3c6e18-2b75-4c3f-9b35-9d1c6f4e1a01',
		'2026-01-01 00:00:00+00',
		'2026-01-01 00:00:00+00'
	),
	(
		'84f7df20-0d04-4dd7-97e9-7c276c337a12',
		'model-coexistence-deleted',
		'Model Coexistence Deleted',
		false,
		false,
		true,
		'2026-01-02 00:00:00+00',
		200000,
		70,
		'{}'::jsonb,
		null,
		'2026-01-01 00:00:00+00',
		'2026-01-02 00:00:00+00'
	);

UPDATE chats
SET last_model_config_id = '84f7df20-0d04-4dd7-97e9-7c276c337a11'
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

UPDATE chat_messages
SET model_config_id = '84f7df20-0d04-4dd7-97e9-7c276c337a11'
WHERE chat_id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

UPDATE chat_queued_messages
SET model_config_id = '84f7df20-0d04-4dd7-97e9-7c276c337a11'
WHERE chat_id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

UPDATE chat_debug_runs
SET model_config_id = '84f7df20-0d04-4dd7-97e9-7c276c337a11'
WHERE chat_id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';
