ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS input_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS output_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS total_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS reasoning_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS cache_creation_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS cache_read_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS context_limit BIGINT;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
            AND table_name = 'chat_messages'
            AND column_name = 'cached_output_tokens'
    ) THEN
        UPDATE chat_messages
        SET cache_creation_tokens = COALESCE(cache_creation_tokens, cached_output_tokens)
        WHERE cached_output_tokens IS NOT NULL;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
            AND table_name = 'chat_messages'
            AND column_name = 'cached_input_tokens'
    ) THEN
        UPDATE chat_messages
        SET cache_read_tokens = COALESCE(cache_read_tokens, cached_input_tokens)
        WHERE cached_input_tokens IS NOT NULL;
    END IF;
END $$;

ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS cached_output_tokens,
    DROP COLUMN IF EXISTS cached_input_tokens;
