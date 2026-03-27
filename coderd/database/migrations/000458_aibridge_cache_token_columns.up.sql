ALTER TABLE aibridge_token_usages
    ADD COLUMN cache_read_input_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN cache_write_input_tokens BIGINT NOT NULL DEFAULT 0;

-- Backfill from metadata JSONB. Old rows stored cache tokens under
-- provider-specific keys; new rows use the dedicated columns above.
UPDATE aibridge_token_usages
SET
    cache_read_input_tokens = GREATEST(
        COALESCE((metadata->>'cache_read_input')::bigint, 0),
        COALESCE((metadata->>'input_cached')::bigint, 0),
        COALESCE((metadata->>'prompt_cached')::bigint, 0)
    ),
    cache_write_input_tokens = COALESCE((metadata->>'cache_creation_input')::bigint, 0)
WHERE metadata IS NOT NULL
  AND cache_read_input_tokens = 0
  AND cache_write_input_tokens = 0;
