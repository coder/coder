DROP INDEX IF EXISTS idx_chat_messages_compressed_summary_boundary;

ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS compressed;

ALTER TABLE chat_model_configs
    DROP CONSTRAINT IF EXISTS chat_model_configs_compression_threshold_check,
    DROP CONSTRAINT IF EXISTS chat_model_configs_context_limit_check;

ALTER TABLE chat_model_configs
    DROP COLUMN IF EXISTS compression_threshold,
    DROP COLUMN IF EXISTS context_limit;
