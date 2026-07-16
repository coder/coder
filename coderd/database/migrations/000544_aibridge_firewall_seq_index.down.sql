DROP INDEX IF EXISTS idx_aibridge_interceptions_agent_firewall_session_seq;

CREATE INDEX idx_aibridge_interceptions_agent_firewall_session_id
    ON aibridge_interceptions (agent_firewall_session_id)
    WHERE agent_firewall_session_id IS NOT NULL;
