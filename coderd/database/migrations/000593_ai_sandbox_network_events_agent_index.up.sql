-- Reading one AI agent's egress decisions filters on the attribution snapshot
-- rather than joining through the session, because that snapshot is what lets
-- an event outlive the session it belonged to. Carrying id in the index lets
-- the paged read take its ordering from the index rather than a sort.
--
-- ai_sandbox_sessions already indexes ai_agent_id. This is the same read on the
-- higher volume of the two tables.
CREATE INDEX idx_ai_sandbox_network_events_ai_agent_id
	ON ai_sandbox_network_events (ai_agent_id, id);
