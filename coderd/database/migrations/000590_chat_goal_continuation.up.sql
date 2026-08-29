-- The goal continuation loop adds the blocked status, machine-readable
-- pause causes, and the auto-continuation budget. It ships separately
-- from 000589 so databases that already ran 000589 still get it.
-- ALTER TYPE ... ADD VALUE cannot be referenced later in the same
-- transaction, and the fresh-install generators run every migration in
-- one transaction, so the enum is rebuilt instead. The status checks
-- and the partial index reference the old type, so they are recreated
-- around the rebuild.
DROP INDEX idx_chat_goals_current;
ALTER TABLE chat_goals
    DROP CONSTRAINT chat_goals_completed_at_status_check,
    DROP CONSTRAINT chat_goals_cleared_at_status_check,
    DROP CONSTRAINT chat_goals_replaced_at_status_check,
    DROP CONSTRAINT chat_goals_completion_summary_status_check,
    DROP CONSTRAINT chat_goals_completed_by_user_status_check,
    DROP CONSTRAINT chat_goals_completed_by_agent_status_check;

ALTER TYPE chat_goal_status RENAME TO chat_goal_status_old;
CREATE TYPE chat_goal_status AS ENUM (
    'active',
    'paused',
    'blocked',
    'complete',
    'cleared',
    'replaced'
);
ALTER TABLE chat_goals
    ALTER COLUMN status TYPE chat_goal_status
    USING (status::text::chat_goal_status);
DROP TYPE chat_goal_status_old;

ALTER TABLE chat_goals
    -- Machine-readable cause for the current paused status
    -- (user, interrupt, turn_limit, usage_limit, error).
    ADD COLUMN paused_reason TEXT,
    -- Model-supplied explanation for the current blocked status.
    ADD COLUMN blocked_reason TEXT,
    -- Auto-continuation turns consumed since the goal last became
    -- active. Reset on resume; bounds the idle-driven continuation
    -- loop.
    ADD COLUMN continuation_count BIGINT NOT NULL DEFAULT 0;

-- Rows paused before this migration carry no recorded cause; the old
-- schema cannot distinguish a user pause from an interrupt, so leave
-- the reason absent instead of fabricating one. Active goals on idle
-- (waiting) or errored chats have no future turn finish to fire the
-- continuation hook, so pause them to make Resume available instead of
-- leaving them active but idle forever. Chats with a live or resumable
-- turn (running, requires_action, interrupting) keep their active
-- goals: their turns settle through the new binary, which continues or
-- pauses the goal itself.
UPDATE chat_goals
SET status = 'paused'
FROM chats
WHERE chat_goals.root_chat_id = chats.id
    AND chat_goals.status = 'active'
    AND chats.status IN ('waiting', 'error');

ALTER TABLE chat_goals
    ADD CONSTRAINT chat_goals_completed_at_status_check CHECK ((status = 'complete') = (completed_at IS NOT NULL)),
    ADD CONSTRAINT chat_goals_cleared_at_status_check CHECK ((status = 'cleared') = (cleared_at IS NOT NULL)),
    ADD CONSTRAINT chat_goals_replaced_at_status_check CHECK ((status = 'replaced') = (replaced_at IS NOT NULL)),
    ADD CONSTRAINT chat_goals_completion_summary_status_check CHECK (completion_summary IS NULL OR status = 'complete'),
    ADD CONSTRAINT chat_goals_completed_by_user_status_check CHECK (completed_by_user_id IS NULL OR status = 'complete'),
    ADD CONSTRAINT chat_goals_completed_by_agent_status_check CHECK (completed_by_agent = FALSE OR status = 'complete'),
    -- Reasons only exist on paused rows, but rows paused before this
    -- migration carry none, so paused rows may lack a reason.
    ADD CONSTRAINT chat_goals_paused_reason_status_check CHECK (paused_reason IS NULL OR status = 'paused'),
    ADD CONSTRAINT chat_goals_blocked_reason_status_check CHECK ((status = 'blocked') = (blocked_reason IS NOT NULL));

CREATE UNIQUE INDEX idx_chat_goals_current
    ON chat_goals(root_chat_id)
    WHERE status IN ('active', 'paused', 'blocked');
