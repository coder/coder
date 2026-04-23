CREATE TABLE chat_debug_runs (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id                UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    -- root_chat_id and parent_chat_id are intentionally NOT
    -- foreign-keyed to chats(id). They are snapshot values that
    -- record the subchat hierarchy at run time. The referenced
    -- chat may be archived or deleted independently, and we want
    -- to preserve the historical lineage in debug rows rather
    -- than cascade-delete them.
    root_chat_id           UUID,
    parent_chat_id         UUID,
    -- model_config_id follows the same snapshot rationale as
    -- root_chat_id / parent_chat_id above: it records the model
    -- configuration in effect at run time and must survive if
    -- the referenced config is later deleted or rotated.
    model_config_id        UUID,
    trigger_message_id     BIGINT,
    history_tip_message_id BIGINT,
    kind                   TEXT        NOT NULL,
    status                 TEXT        NOT NULL,
    provider               TEXT,
    model                  TEXT,
    summary                JSONB       NOT NULL DEFAULT '{}'::jsonb,
    started_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at            TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_chat_debug_runs_id_chat ON chat_debug_runs(id, chat_id);
CREATE INDEX idx_chat_debug_runs_chat_started ON chat_debug_runs(chat_id, started_at DESC);

CREATE TABLE chat_debug_steps (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id                 UUID        NOT NULL,
    chat_id                UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    step_number            INT         NOT NULL,
    operation              TEXT        NOT NULL,
    status                 TEXT        NOT NULL,
    history_tip_message_id BIGINT,
    assistant_message_id   BIGINT,
    normalized_request     JSONB       NOT NULL,
    normalized_response    JSONB,
    usage                  JSONB,
    attempts               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    error                  JSONB,
    metadata               JSONB       NOT NULL DEFAULT '{}'::jsonb,
    started_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at            TIMESTAMPTZ,
    CONSTRAINT fk_chat_debug_steps_run_chat
        FOREIGN KEY (run_id, chat_id)
        REFERENCES chat_debug_runs(id, chat_id)
        ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_chat_debug_steps_run_step ON chat_debug_steps(run_id, step_number);
CREATE INDEX idx_chat_debug_steps_chat_tip ON chat_debug_steps(chat_id, history_tip_message_id);
-- Supports DeleteChatDebugDataAfterMessageID assistant_message_id branch.
CREATE INDEX idx_chat_debug_steps_chat_assistant_msg ON chat_debug_steps(chat_id, assistant_message_id) WHERE assistant_message_id IS NOT NULL;

-- Supports FinalizeStaleChatDebugRows worker query.
CREATE INDEX idx_chat_debug_runs_stale ON chat_debug_runs(updated_at) WHERE finished_at IS NULL;
CREATE INDEX idx_chat_debug_steps_stale ON chat_debug_steps(updated_at) WHERE finished_at IS NULL;
