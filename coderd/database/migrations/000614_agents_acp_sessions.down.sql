DROP TABLE agents_acp_sessions;

DELETE FROM workspace_agent_context_resources WHERE body_kind::text = 'acp_harness';
DELETE FROM chat_context_resources WHERE body_kind::text = 'acp_harness';

ALTER TYPE workspace_agent_context_body_kind RENAME TO workspace_agent_context_body_kind_old;
CREATE TYPE workspace_agent_context_body_kind AS ENUM (
    'instruction_file', 'skill', 'mcp_config', 'mcp_server',
    'plugin', 'hook', 'subagent', 'command'
);
ALTER TABLE workspace_agent_context_resources ALTER COLUMN body_kind
    TYPE workspace_agent_context_body_kind USING body_kind::text::workspace_agent_context_body_kind;
ALTER TABLE chat_context_resources ALTER COLUMN body_kind
    TYPE workspace_agent_context_body_kind USING body_kind::text::workspace_agent_context_body_kind;
DROP TYPE workspace_agent_context_body_kind_old;
