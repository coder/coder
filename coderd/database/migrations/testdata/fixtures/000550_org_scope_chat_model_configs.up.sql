-- Seed legacy global chat model configuration data immediately before 000548.
-- The fixture covers active and soft-deleted configs, a live and a deleted
-- organization, chat history, model-bearing deployment settings, and personal
-- model settings.
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
        'model-fixture-org',
        '',
        '2026-01-01 00:00:00+00',
        '2026-01-01 00:00:00+00',
        false,
        'Model Fixture Org',
        '',
        false,
        '{}'
    ),
    (
        '84f7df20-0d04-4dd7-97e9-7c276c337a02',
        'deleted-model-fixture-org',
        '',
        '2026-01-01 00:00:00+00',
        '2026-01-01 00:00:00+00',
        false,
        'Deleted Model Fixture Org',
        '',
        true,
        '{}'
    );

INSERT INTO organization_members (
    user_id,
    organization_id,
    created_at,
    updated_at,
    roles
) VALUES (
    '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
    '84f7df20-0d04-4dd7-97e9-7c276c337a01',
    '2026-01-01 00:00:00+00',
    '2026-01-01 00:00:00+00',
    '{}'
);

INSERT INTO chat_model_configs (
    id,
    model,
    display_name,
    enabled,
    is_default,
    deleted,
    context_limit,
    compression_threshold,
    options,
    ai_provider_id,
    created_at,
    updated_at
) VALUES
    (
        '84f7df20-0d04-4dd7-97e9-7c276c337a11',
        'model-fixture-active',
        'Model Fixture Active',
        true,
        true,
        false,
        200000,
        70,
        '{}'::jsonb,
        '0a8b2f84-b5a8-4c44-8c9f-e58c44a534a7',
        '2026-01-01 00:00:00+00',
        '2026-01-01 00:00:00+00'
    ),
    (
        '84f7df20-0d04-4dd7-97e9-7c276c337a12',
        'model-fixture-deleted',
        'Model Fixture Deleted',
        false,
        false,
        true,
        200000,
        70,
        '{}'::jsonb,
        null,
        '2026-01-01 00:00:00+00',
        '2026-01-01 00:00:00+00'
    );

UPDATE chats
SET last_model_config_id = '84f7df20-0d04-4dd7-97e9-7c276c337a11'
WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

-- Organization deletion does not check chats, so a soft-deleted organization
-- may retain them. This row proves their model references are also remapped.
INSERT INTO chats (
    id,
    owner_id,
    last_model_config_id,
    organization_id
)
SELECT
    '72c0438a-18eb-4688-ab80-e4c6a126ef97',
    owner_id,
    '84f7df20-0d04-4dd7-97e9-7c276c337a11',
    '84f7df20-0d04-4dd7-97e9-7c276c337a02'
FROM chats
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

INSERT INTO site_configs (key, value) VALUES
    ('agents_chat_explore_model_override', '84f7df20-0d04-4dd7-97e9-7c276c337a11:low'),
    ('agents_chat_general_model_override', '84f7df20-0d04-4dd7-97e9-7c276c337a11:high'),
    ('agents_chat_title_generation_model_override', '84f7df20-0d04-4dd7-97e9-7c276c337a11:minimal'),
    ('agents_chat_compaction_model_override', '84f7df20-0d04-4dd7-97e9-7c276c337a11:max'),
    (
        'agents_advisor_config',
        '{"enabled":true,"max_uses_per_run":3,"max_output_tokens":4096,"model_config_id":"84f7df20-0d04-4dd7-97e9-7c276c337a11","reasoning_effort":"high"}'
    ),
    ('agents_chat_personal_model_overrides_enabled', 'true');

INSERT INTO user_configs (user_id, key, value) VALUES
    (
        '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
        'chat_personal_model_override:root',
        'model:84f7df20-0d04-4dd7-97e9-7c276c337a11:high'
    ),
    (
        '0ed9befc-4911-4ccf-a8e2-559bf72daa94',
        'chat_compaction_threshold_pct:84f7df20-0d04-4dd7-97e9-7c276c337a11',
        '75'
    );
