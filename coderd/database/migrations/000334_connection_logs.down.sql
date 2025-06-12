DROP INDEX IF EXISTS idx_connection_logs_workspace_id;
DROP INDEX IF EXISTS idx_connection_logs_workspace_owner_id;
DROP INDEX IF EXISTS idx_connection_logs_organization_id;
DROP INDEX IF EXISTS idx_connection_logs_time_desc;

DROP TABLE IF EXISTS connection_logs;

DROP TYPE IF EXISTS connection_action;
