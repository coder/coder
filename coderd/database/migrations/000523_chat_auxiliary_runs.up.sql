CREATE TABLE chat_auxiliary_runs (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    kind                    TEXT        NOT NULL,
    chat_id                 UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    owner_id                UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    model_config_id         UUID        REFERENCES chat_model_configs(id),
    provider                TEXT,
    model                   TEXT,
    status                  TEXT        NOT NULL,
    input_tokens            BIGINT,
    output_tokens           BIGINT,
    total_tokens            BIGINT,
    reasoning_tokens        BIGINT,
    cache_creation_tokens   BIGINT,
    cache_read_tokens       BIGINT,
    context_limit           BIGINT,
    total_cost_micros       BIGINT,
    runtime_ms              BIGINT,
    provider_response_id    TEXT,
    error_code              TEXT,
    question_chars          INTEGER,
    transient_context_chars INTEGER,
    metadata                JSONB       NOT NULL DEFAULT '{}'::jsonb,
    started_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at             TIMESTAMPTZ,
    CONSTRAINT chat_auxiliary_runs_kind_check
        CHECK (kind IN ('side_question')),
    CONSTRAINT chat_auxiliary_runs_status_check
        CHECK (status IN ('running', 'succeeded', 'failed', 'canceled')),
    CONSTRAINT chat_auxiliary_runs_finished_status_check
        CHECK ((status = 'running' AND finished_at IS NULL) OR (status <> 'running' AND finished_at IS NOT NULL))
);

CREATE UNIQUE INDEX idx_chat_auxiliary_runs_active_side_question
    ON chat_auxiliary_runs(chat_id, owner_id, kind)
    WHERE kind = 'side_question' AND status = 'running';
CREATE INDEX idx_chat_auxiliary_runs_stale
    ON chat_auxiliary_runs(updated_at)
    WHERE status = 'running';
CREATE INDEX idx_chat_auxiliary_runs_chat_started
    ON chat_auxiliary_runs(chat_id, started_at DESC);
CREATE INDEX idx_chat_auxiliary_runs_owner_spend
    ON chat_auxiliary_runs(owner_id, started_at)
    WHERE total_cost_micros IS NOT NULL;
