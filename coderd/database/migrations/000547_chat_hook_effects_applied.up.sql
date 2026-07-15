-- Marks pre_tool_use dispatch rows whose response effects (prefix
-- messages, allowed_tools, end_chat) were committed, so decision reuse
-- after a crash can replay effects exactly once.
ALTER TABLE chat_hook_dispatches ADD COLUMN effects_applied_at timestamptz;

-- Rows finalized before this column existed had their effects applied
-- by the old non-replaying code path; grandfather them as applied so
-- the upgrade does not replay stale responses into active chats.
UPDATE chat_hook_dispatches
SET effects_applied_at = finished_at
WHERE event = 'pre_tool_use' AND finished_at IS NOT NULL;
