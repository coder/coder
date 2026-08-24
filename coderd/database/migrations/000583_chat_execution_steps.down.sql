DROP INDEX idx_chat_messages_execution_step_id;
ALTER TABLE chat_messages DROP COLUMN execution_step_id;

DROP TABLE chat_execution_steps;

DROP TYPE chat_execution_step_outcome;
DROP TYPE chat_execution_step_operation;
