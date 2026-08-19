-- File-transfer visibility in the connection log.
--
-- 'file_transfer' typed rows are connect/disconnect session pairs for
-- SFTP, SCP, and rsync sessions (previously logged as 'ssh').
-- 'file_operation' typed rows are point-in-time events for individual
-- file operations observed during a file-transfer session. They share
-- the session's connection_id so they can be grouped with it, and are
-- therefore excluded from the unique pairing index below.
ALTER TYPE connection_type ADD VALUE IF NOT EXISTS 'file_transfer';
ALTER TYPE connection_type ADD VALUE IF NOT EXISTS 'file_operation';

-- The protocol that carried a file operation.
CREATE TYPE connection_log_file_protocol AS ENUM (
	'sftp',
	'scp',
	'rsync'
);

-- The kind of file operation observed. 'download' is a transfer from
-- the workspace to the client, 'upload' is a transfer from the client
-- to the workspace, and 'bidirectional' is a file opened for both at
-- once (e.g. in-place editing), where either direction may have
-- occurred. For SFTP these record the requested access mode at open
-- time, not bytes actually transferred. 'setattr' records attribute
-- changes: truncation, permissions, ownership, and timestamps.
CREATE TYPE connection_log_file_action AS ENUM (
	'download',
	'upload',
	'bidirectional',
	'remove',
	'rmdir',
	'rename',
	'symlink',
	'setattr',
	'hardlink'
);

ALTER TABLE connection_logs
	ADD COLUMN file_protocol connection_log_file_protocol,
	ADD COLUMN file_action connection_log_file_action,
	ADD COLUMN file_path text,
	ADD COLUMN file_target text;

COMMENT ON COLUMN connection_logs.file_protocol IS 'Only set for file operation events. The protocol that carried the file operation (sftp, scp, or rsync).';

COMMENT ON COLUMN connection_logs.file_action IS 'Only set for file operation events. The kind of file operation observed.';

COMMENT ON COLUMN connection_logs.file_path IS 'Only set for file operation events. The path the operation was performed on. For SCP and rsync this is the requested root path from the command line, not necessarily every file transferred.';

COMMENT ON COLUMN connection_logs.file_target IS 'Only set for file operation events that have a second path, such as the destination of a rename or the target of a symlink.';

-- Recreate the connect/disconnect pairing index so it excludes file
-- operation rows, which share the connection_id of their parent session.
-- The predicate uses file_action rather than the new enum value because
-- enum values added in this transaction cannot be referenced within it.
--
-- Locking: the rebuild holds an ACCESS EXCLUSIVE lock and connection_logs
-- is unbounded (retention is opt-in), so it may take a while on large
-- deployments. CONCURRENTLY is not an option inside a migration
-- transaction; precedent: 000161.
DROP INDEX idx_connection_logs_connection_id_workspace_id_agent_name;
CREATE UNIQUE INDEX idx_connection_logs_connection_id_workspace_id_agent_name
ON connection_logs (connection_id, workspace_id, agent_name)
WHERE file_action IS NULL;

COMMENT ON INDEX idx_connection_logs_connection_id_workspace_id_agent_name IS 'Connection ID is NULL for web events, but present for SSH events. Therefore, this index allows multiple web events for the same workspace & agent. For SSH events, the upsertion query handles duplicates on this index by upserting the disconnect_time and disconnect_reason for the same connection_id when the connection is closed. File operation events share the connection_id of their parent file-transfer session and are excluded from the index via the file_action predicate.';
