DROP INDEX IF EXISTS idx_connection_logs_workspace_id;
DROP INDEX IF EXISTS idx_connection_logs_workspace_owner_id;
DROP INDEX IF EXISTS idx_connection_logs_organization_id;
DROP INDEX IF EXISTS idx_connection_logs_connect_time_desc;
DROP INDEX IF EXISTS idx_connection_logs_connection_id_workspace_id_agent_name;

DROP TABLE IF EXISTS connection_logs;

DROP TYPE IF EXISTS connection_type;

DROP TYPE IF EXISTS connection_status;
