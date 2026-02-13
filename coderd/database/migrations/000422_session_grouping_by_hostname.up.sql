-- Make workspace_sessions.ip nullable since sessions now group by
-- hostname (with IP fallback), and a session may span multiple IPs.
ALTER TABLE workspace_sessions ALTER COLUMN ip DROP NOT NULL;

-- Replace the IP-based lookup index with hostname-based indexes
-- to support the new grouping logic.
DROP INDEX IF EXISTS idx_workspace_sessions_lookup;
CREATE INDEX idx_workspace_sessions_hostname_lookup
    ON workspace_sessions (workspace_id, client_hostname, started_at)
    WHERE client_hostname IS NOT NULL;
CREATE INDEX idx_workspace_sessions_ip_lookup
    ON workspace_sessions (workspace_id, ip, started_at)
    WHERE ip IS NOT NULL AND client_hostname IS NULL;
