-- Remove file operation rows so the non-partial unique index can be
-- recreated (they share connection_id values with their parent sessions
-- and would otherwise collide). This loses file-operation audit data on
-- downgrade, which is unavoidable because the old schema cannot
-- represent it.
DELETE FROM connection_logs WHERE file_action IS NOT NULL;

-- The rebuild must come after the DELETE above, since the non-partial
-- unique index cannot contain the removed file operation rows. See the
-- up migration for locking notes.
DROP INDEX idx_connection_logs_connection_id_workspace_id_agent_name;
CREATE UNIQUE INDEX idx_connection_logs_connection_id_workspace_id_agent_name
ON connection_logs (connection_id, workspace_id, agent_name);

COMMENT ON INDEX idx_connection_logs_connection_id_workspace_id_agent_name IS 'Connection ID is NULL for web events, but present for SSH events. Therefore, this index allows multiple web events for the same workspace & agent. For SSH events, the upsertion query handles duplicates on this index by upserting the disconnect_time and disconnect_reason for the same connection_id when the connection is closed.';

ALTER TABLE connection_logs
	DROP COLUMN file_protocol,
	DROP COLUMN file_action,
	DROP COLUMN file_path,
	DROP COLUMN file_target;

DROP TYPE connection_log_file_action;
DROP TYPE connection_log_file_protocol;

-- The 'file_transfer' and 'file_operation' enum values are intentionally
-- not removed. Postgres cannot drop an enum value in place; removing
-- them would require recreating connection_type and rewriting the
-- connection_logs type column, which takes an exclusive lock on the
-- table and would have to DELETE all rows using the values (audit data)
-- because they cannot exist in the old type. Leaving the values in place
-- is harmless: old code never queries for them and renders unknown types
-- without error. This matches the precedent of 000557.
