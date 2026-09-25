ALTER TABLE chat_messages ADD COLUMN queued_message_id bigint;

COMMENT ON COLUMN chat_messages.queued_message_id IS
    'ID of the chat_queued_messages row this message was promoted from. NULL when the message was not promoted from the queue, or when a version that did not record the link wrote it. Not a foreign key: promotion deletes the queued row in the same transaction.';
