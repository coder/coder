-- Replace the session-only index with a composite index on
-- (agent_firewall_session_id, agent_firewall_sequence_number). The sessions
-- list computes each interception's next firewall sequence number to bound the
-- boundary_logs it triggered; the composite index serves that lookup index-only
-- and still covers session-only lookups.
DROP INDEX IF EXISTS idx_aibridge_interceptions_agent_firewall_session_id;

CREATE INDEX idx_aibridge_interceptions_agent_firewall_session_seq
    ON aibridge_interceptions (agent_firewall_session_id, agent_firewall_sequence_number)
    WHERE agent_firewall_session_id IS NOT NULL;

DROP INDEX IF EXISTS idx_boundary_logs_session_seq;

CREATE INDEX idx_boundary_logs_session_seq
    ON boundary_logs (session_id, sequence_number) INCLUDE (matched_rule);
