-- A tool policy produced by user_prompt_submit for a queued prompt
-- must not affect the active turn; it is stored on the queued row and
-- applied to the chat when that prompt is promoted.
ALTER TABLE chat_queued_messages ADD COLUMN hook_allowed_tools jsonb;

COMMENT ON COLUMN chat_queued_messages.hook_allowed_tools IS 'Tool policy from the queued prompt''s user_prompt_submit hook; NULL means no policy. Copied to chats.hook_allowed_tools at promotion.';
