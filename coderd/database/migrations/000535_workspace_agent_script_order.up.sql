CREATE TYPE workspace_agent_script_order_requires AS ENUM (
    'success',
    'completion'
);

COMMENT ON TYPE workspace_agent_script_order_requires IS 'What state the awaited script must reach before the dependent script starts: success means the dependent is skipped unless the awaited script succeeds, completion means the dependent runs after the awaited script reaches any terminal state.';

CREATE TABLE workspace_agent_script_order (
    script_id uuid NOT NULL REFERENCES workspace_agent_scripts (id) ON DELETE CASCADE,
    after_script_id uuid NOT NULL REFERENCES workspace_agent_scripts (id) ON DELETE CASCADE,
    requires workspace_agent_script_order_requires NOT NULL DEFAULT 'success',
    PRIMARY KEY (script_id, after_script_id),
    CONSTRAINT workspace_agent_script_order_no_self CHECK (script_id <> after_script_id)
);

COMMENT ON TABLE workspace_agent_script_order IS 'Resolved coder_script_order rules: the script identified by script_id runs after the script identified by after_script_id reaches a terminal state.';
