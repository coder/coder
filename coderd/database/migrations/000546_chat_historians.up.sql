ALTER TYPE chat_mode ADD VALUE IF NOT EXISTS 'historian';

CREATE TABLE chat_historian_states (
    root_chat_id uuid PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    historian_chat_id uuid UNIQUE REFERENCES chats(id) ON DELETE SET NULL,
    last_processed_history_version bigint NOT NULL DEFAULT 0,
    processing_history_version bigint,
    processing_started_at timestamptz,
    dispatch_id uuid,
    dispatched_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_historian_states_last_processed_nonnegative
        CHECK (last_processed_history_version >= 0),
    CONSTRAINT chat_historian_states_processing_nonnegative
        CHECK (
            processing_history_version IS NULL
            OR processing_history_version > last_processed_history_version
        ),
    CONSTRAINT chat_historian_states_processing_fields
        CHECK (
            (processing_history_version IS NULL
                AND processing_started_at IS NULL
                AND dispatch_id IS NULL
                AND dispatched_at IS NULL)
            OR
            (processing_history_version IS NOT NULL
                AND processing_started_at IS NOT NULL
                AND dispatch_id IS NOT NULL)
        )
);

CREATE INDEX chats_historian_candidates_idx
    ON chats (updated_at, owner_id, id)
    WHERE parent_chat_id IS NULL
      AND archived = false
      AND status IN ('waiting', 'error');
