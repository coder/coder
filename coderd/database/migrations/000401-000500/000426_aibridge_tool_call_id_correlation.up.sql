ALTER TABLE aibridge_tool_usages
ADD COLUMN provider_tool_call_id text NULL; -- nullable to allow existing data to be correct

CREATE INDEX idx_aibridge_tool_usages_provider_tool_call_id ON aibridge_tool_usages (provider_tool_call_id);

ALTER TABLE aibridge_interceptions
ADD COLUMN thread_parent_id UUID NULL,
ADD COLUMN thread_root_id UUID NULL;

COMMENT ON COLUMN aibridge_interceptions.thread_parent_id IS 'The interception which directly caused this interception to occur, usually through an agentic loop or threaded conversation.';
COMMENT ON COLUMN aibridge_interceptions.thread_root_id IS 'The root interception of the thread that this interception belongs to.';

CREATE INDEX idx_aibridge_interceptions_thread_parent_id ON aibridge_interceptions (thread_parent_id);
CREATE INDEX idx_aibridge_interceptions_thread_root_id ON aibridge_interceptions (thread_root_id);
