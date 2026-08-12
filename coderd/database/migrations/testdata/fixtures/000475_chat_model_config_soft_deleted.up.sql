-- Soft-deleted chat model config whose provider never had an ai_providers
-- backfill match, so it reaches later migrations with ai_provider_id IS NULL.
--
-- This row exercises the 000534 down migration's
-- `UPDATE ... SET provider = '' WHERE provider IS NULL` sweep: its NULL
-- ai_provider_id means the backfill join leaves provider NULL, and the sweep
-- must populate it before `ALTER COLUMN provider SET NOT NULL`.
--
-- It is inserted at version 000475 (after 000474 dropped the provider foreign
-- key) so the provider value need not reference a chat_providers row, and the
-- 000504/000505 backfill (which matches on `cmc.provider = cp.provider`) skips
-- it. `deleted = TRUE` keeps it out of idx_chat_model_configs_single_default
-- and satisfies chat_model_configs_ai_provider_required_when_active (added in
-- 000505), which permits a NULL ai_provider_id only for deleted rows.
INSERT INTO chat_model_configs (
    id,
    provider,
    model,
    display_name,
    enabled,
    is_default,
    deleted,
    deleted_at,
    context_limit,
    compression_threshold,
    created_at,
    updated_at
) VALUES (
    'b3a1d2c4-5e6f-4a7b-8c9d-0e1f2a3b4c5d',
    'legacy-removed',
    'legacy-model',
    'Legacy Soft Deleted',
    FALSE,
    FALSE,
    TRUE,
    '2024-01-01 00:00:00+00',
    200000,
    70,
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);
