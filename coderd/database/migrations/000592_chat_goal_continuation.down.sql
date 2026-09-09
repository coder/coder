DROP INDEX idx_chat_goals_current;
ALTER TABLE chat_goals
    DROP CONSTRAINT chat_goals_completed_at_status_check,
    DROP CONSTRAINT chat_goals_cleared_at_status_check,
    DROP CONSTRAINT chat_goals_replaced_at_status_check,
    DROP CONSTRAINT chat_goals_completion_summary_status_check,
    DROP CONSTRAINT chat_goals_completed_by_user_status_check,
    DROP CONSTRAINT chat_goals_completed_by_agent_status_check,
    DROP CONSTRAINT chat_goals_paused_reason_status_check,
    DROP CONSTRAINT chat_goals_blocked_reason_status_check;

UPDATE chat_goals SET status = 'paused' WHERE status = 'blocked';

ALTER TABLE chat_goals
    DROP COLUMN paused_reason,
    DROP COLUMN blocked_reason,
    DROP COLUMN continuation_count;

ALTER TYPE chat_goal_status RENAME TO chat_goal_status_old;
CREATE TYPE chat_goal_status AS ENUM (
    'active',
    'paused',
    'complete',
    'cleared',
    'replaced'
);
ALTER TABLE chat_goals
    ALTER COLUMN status TYPE chat_goal_status
    USING (status::text::chat_goal_status);
DROP TYPE chat_goal_status_old;

ALTER TABLE chat_goals
    ADD CONSTRAINT chat_goals_completed_at_status_check CHECK ((status = 'complete') = (completed_at IS NOT NULL)),
    ADD CONSTRAINT chat_goals_cleared_at_status_check CHECK ((status = 'cleared') = (cleared_at IS NOT NULL)),
    ADD CONSTRAINT chat_goals_replaced_at_status_check CHECK ((status = 'replaced') = (replaced_at IS NOT NULL)),
    ADD CONSTRAINT chat_goals_completion_summary_status_check CHECK (completion_summary IS NULL OR status = 'complete'),
    ADD CONSTRAINT chat_goals_completed_by_user_status_check CHECK (completed_by_user_id IS NULL OR status = 'complete'),
    ADD CONSTRAINT chat_goals_completed_by_agent_status_check CHECK (completed_by_agent = FALSE OR status = 'complete');

CREATE UNIQUE INDEX idx_chat_goals_current
    ON chat_goals(root_chat_id)
    WHERE status IN ('active', 'paused');
