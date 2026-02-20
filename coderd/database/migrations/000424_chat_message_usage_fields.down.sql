ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS context_limit,
    DROP COLUMN IF EXISTS cache_read_tokens,
    DROP COLUMN IF EXISTS cache_creation_tokens,
    DROP COLUMN IF EXISTS reasoning_tokens,
    DROP COLUMN IF EXISTS total_tokens,
    DROP COLUMN IF EXISTS output_tokens,
    DROP COLUMN IF EXISTS input_tokens,
    DROP COLUMN IF EXISTS cached_output_tokens,
    DROP COLUMN IF EXISTS cached_input_tokens;
