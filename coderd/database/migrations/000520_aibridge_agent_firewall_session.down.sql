DROP INDEX IF EXISTS idx_aibridge_interceptions_agent_firewall_session_id;

ALTER TABLE aibridge_interceptions
    DROP COLUMN IF EXISTS agent_firewall_sequence_number,
    DROP COLUMN IF EXISTS agent_firewall_session_id;
