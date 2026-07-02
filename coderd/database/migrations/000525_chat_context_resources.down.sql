-- The workspace_agent_context_* enum types are owned by migration
-- 000522 and are still in use by workspace_agent_context_resources, so
-- they are intentionally left in place here.
DROP TABLE IF EXISTS chat_context_resources;
