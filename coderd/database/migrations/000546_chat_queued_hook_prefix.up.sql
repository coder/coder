-- Lifecycle hook prefix messages (model_context / user_message rows)
-- produced by user_prompt_submit for a queued prompt must not enter
-- active history until that prompt is promoted, so they are stored on
-- the queued row instead.
ALTER TABLE chat_queued_messages ADD COLUMN hook_prefix jsonb;
