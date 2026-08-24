-- The mirror's AI agent row goes. Nothing reads it: identity, resolution,
-- revocation, the sweep and the AI agents endpoint all read ai_agent_ledger,
-- and the only writer left was an insert whose result nobody consulted.
--
-- No data is carried across. Every row in this table was written by the
-- identity code, which exists only on this branch, so there is no deployment
-- with rows to preserve and no backfill to argue about.
--
-- The users row survives this. Six places still route on
-- users.kind = 'ai_agent', and the mirrored username is what the endpoint and
-- the authorizer's friendly name are read from. That is a separate removal.
DROP TABLE ai_agents;

-- The enum existed for ai_agents.origin_type and had no other column. The
-- ledger states the same closed set as text with a CHECK, for the reason in
-- "An actor type column on a core table is text with a CHECK" in
-- poc_audit/implementation_patterns.md, so nothing replaces this.
DROP TYPE ai_agent_origin;
