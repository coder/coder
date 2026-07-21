DROP INDEX IF EXISTS idx_boundary_logs_session_seq;

CREATE INDEX idx_boundary_logs_session_seq
    ON boundary_logs (session_id, sequence_number);

DROP INDEX IF EXISTS idx_aibridge_interceptions_agent_firewall_session_seq;

CREATE INDEX idx_aibridge_interceptions_agent_firewall_session_id
    ON aibridge_interceptions (agent_firewall_session_id)
    WHERE agent_firewall_session_id IS NOT NULL;
