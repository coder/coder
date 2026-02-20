ALTER TABLE chat_model_configs
    ADD COLUMN IF NOT EXISTS context_limit BIGINT,
    ADD COLUMN IF NOT EXISTS compression_threshold INTEGER;

-- Backfill existing rows so context compression can operate safely by default.
UPDATE chat_model_configs
SET
    context_limit = COALESCE(context_limit, 200000),
    compression_threshold = COALESCE(compression_threshold, 70);

ALTER TABLE chat_model_configs
    ALTER COLUMN context_limit SET NOT NULL,
    ALTER COLUMN compression_threshold SET NOT NULL;

ALTER TABLE chat_model_configs
    ADD CONSTRAINT chat_model_configs_context_limit_check
        CHECK (context_limit > 0),
    ADD CONSTRAINT chat_model_configs_compression_threshold_check
        CHECK (compression_threshold >= 0 AND compression_threshold <= 100);

ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS compressed BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_chat_messages_compressed_summary_boundary
    ON chat_messages(chat_id, created_at DESC, id DESC)
    WHERE compressed = TRUE
        AND role = 'system'
        AND hidden = TRUE;
