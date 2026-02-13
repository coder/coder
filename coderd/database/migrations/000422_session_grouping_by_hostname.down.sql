UPDATE workspace_sessions SET ip = '0.0.0.0'::inet WHERE ip IS NULL;
ALTER TABLE workspace_sessions ALTER COLUMN ip SET NOT NULL;

DROP INDEX IF EXISTS idx_workspace_sessions_hostname_lookup;
DROP INDEX IF EXISTS idx_workspace_sessions_ip_lookup;
CREATE INDEX idx_workspace_sessions_lookup ON workspace_sessions (workspace_id, ip, started_at);
