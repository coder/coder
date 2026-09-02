-- Active summary generation is tracked separately so chat reads stay stable
-- while late watch subscribers can recover the transient loading state.
CREATE TABLE chat_summary_generations (
    chat_id UUID PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
