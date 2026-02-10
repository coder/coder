ALTER TABLE workspace_app_audit_sessions DROP COLUMN IF EXISTS connection_id;
ALTER TABLE connection_logs DROP COLUMN IF EXISTS updated_at;
