ALTER TABLE aibridge_tool_usages
ADD COLUMN provider_tool_call_id text NULL; -- nullable to allow existing data to be correct

CREATE INDEX idx_aibridge_tool_usages_provider_tool_call_id ON aibridge_tool_usages (provider_tool_call_id);

ALTER TABLE aibridge_interceptions
ADD COLUMN parent_id UUID NULL,
ADD COLUMN ancestor_id UUID NULL;

COMMENT ON COLUMN aibridge_interceptions.parent_id IS 'The interception which directly caused this interception to occur, usually through an agentic loop or threaded conversation.';
COMMENT ON COLUMN aibridge_interceptions.ancestor_id IS 'The first interception which directly caused a series of interceptions to occur (including this one), usually through an agentic loop or threaded conversation.';

CREATE INDEX idx_aibridge_interceptions_parent_id ON aibridge_interceptions (parent_id);
