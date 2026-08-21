-- Fixture for 000580 (org-scope chat model configs). Fixtures apply at
-- the post-migration schema (migrate_test.go applies fixture N right
-- after migration N runs), so rows carry the final shape: an explicit
-- organization_id and the seeded everyone-in-org group_acl entry (the
-- Everyone group shares the org ID).
--   1. the default org's single live default config,
--   2. a live non-default config in the default org,
--   3. a soft-deleted config (outside the partial default index).
INSERT INTO ai_providers (
    id,
    type,
    name,
    enabled,
    base_url,
    created_at,
    updated_at
) VALUES (
    'a52c6f0e-7d4b-4e1a-9c3f-2b8d5e6f7a8b',
    'openai',
    'fixture-openai-580',
    TRUE,
    'https://api.openai.com/v1',
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

INSERT INTO chat_model_configs (
    id,
    model,
    display_name,
    enabled,
    is_default,
    context_limit,
    compression_threshold,
    ai_provider_id,
    organization_id,
    group_acl,
    created_at,
    updated_at
)
SELECT
    v.id,
    v.model,
    v.display_name,
    v.enabled,
    v.is_default,
    v.context_limit,
    v.compression_threshold,
    'a52c6f0e-7d4b-4e1a-9c3f-2b8d5e6f7a8b',
    o.id,
    jsonb_build_object(o.id::text, jsonb_build_object('permissions', jsonb_build_array('read'))),
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
FROM
    organizations o,
    (
        VALUES
            (
                '580c0001-0000-4000-8000-000000000001'::uuid,
                'gpt-5.2',
                'Fixture Default 580',
                TRUE,
                TRUE,
                200000,
                70
            ),
            (
                '580c0002-0000-4000-8000-000000000002'::uuid,
                'gpt-5.2-mini',
                'Fixture Non Default 580',
                TRUE,
                FALSE,
                128000,
                70
            ),
            (
                '580c0003-0000-4000-8000-000000000003'::uuid,
                'gpt-4-legacy',
                'Fixture Deleted 580',
                FALSE,
                FALSE,
                128000,
                70
            )
    ) AS v (id, model, display_name, enabled, is_default, context_limit, compression_threshold)
WHERE
    o.is_default = TRUE;

UPDATE chat_model_configs
SET deleted = TRUE, deleted_at = '2024-06-01 00:00:00+00'
WHERE id = '580c0003-0000-4000-8000-000000000003';
