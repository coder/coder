CREATE TYPE chat_goal_status AS ENUM (
    'active',
    'paused',
    'complete',
    'cleared',
    'replaced'
);

CREATE TABLE chat_goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    root_chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    created_from_chat_id UUID REFERENCES chats(id) ON DELETE SET NULL,
    objective TEXT NOT NULL,
    status chat_goal_status NOT NULL,
    completion_summary TEXT,
    created_by_user_id UUID NOT NULL REFERENCES users(id),
    completed_by_user_id UUID REFERENCES users(id),
    completed_by_agent BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    cleared_at TIMESTAMPTZ,
    replaced_at TIMESTAMPTZ,
    CONSTRAINT chat_goals_objective_not_empty CHECK (length(btrim(objective)) > 0),
    CONSTRAINT chat_goals_completed_at_status_check CHECK ((status = 'complete') = (completed_at IS NOT NULL)),
    CONSTRAINT chat_goals_cleared_at_status_check CHECK ((status = 'cleared') = (cleared_at IS NOT NULL)),
    CONSTRAINT chat_goals_replaced_at_status_check CHECK ((status = 'replaced') = (replaced_at IS NOT NULL)),
    CONSTRAINT chat_goals_completion_summary_status_check CHECK (completion_summary IS NULL OR status = 'complete'),
    CONSTRAINT chat_goals_completed_by_user_status_check CHECK (completed_by_user_id IS NULL OR status = 'complete'),
    CONSTRAINT chat_goals_completed_by_agent_status_check CHECK (completed_by_agent = FALSE OR status = 'complete')
);

CREATE UNIQUE INDEX idx_chat_goals_current
    ON chat_goals(root_chat_id)
    WHERE status IN ('active', 'paused');

CREATE INDEX idx_chat_goals_root_created
    ON chat_goals(root_chat_id, created_at DESC, id DESC);
