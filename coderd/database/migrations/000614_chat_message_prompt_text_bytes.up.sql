ALTER TABLE chat_messages ADD COLUMN prompt_text_bytes bigint;

COMMENT ON COLUMN chat_messages.prompt_text_bytes IS 'Text bytes of the prompt and tool definitions sent in the model request that produced this assistant message. Paired with the message''s prompt token counts to convert bytes to tokens. NULL when unknown, including requests that carried media.';
