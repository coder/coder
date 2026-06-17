-- No FK to agent firewall sessions: Bridge interceptions may be recorded
-- before the session row exists, since Agent Firewall log delivery is async.
-- agent_firewall_session_id is a soft reference resolved at query time.
ALTER TABLE aibridge_interceptions
    ADD COLUMN agent_firewall_session_id UUID NULL,
    ADD COLUMN agent_firewall_sequence_number INT NULL;

COMMENT ON COLUMN aibridge_interceptions.agent_firewall_session_id IS
    'The Agent Firewall session ID, linking this Bridge interception to an Agent Firewall confinement session.';
COMMENT ON COLUMN aibridge_interceptions.agent_firewall_sequence_number IS
    'The Agent Firewall sequence number from the request header. Used to determine exact ordering of network requests relative to Agent Firewall audit events. NULL when the request did not pass through Agent Firewall.';

CREATE INDEX idx_aibridge_interceptions_agent_firewall_session_id
    ON aibridge_interceptions (agent_firewall_session_id)
    WHERE agent_firewall_session_id IS NOT NULL;
