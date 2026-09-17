ALTER TABLE chat_context_resources
    ADD COLUMN discovered BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN chat_context_resources.discovered IS 'True when chatd pinned the row from a directory a tool touched during the chat rather than copying it from the agent snapshot. Discovered rows are ignored by snapshot drift checks and are replaced by the snapshot copy once the agent starts publishing the same source.';

COMMENT ON COLUMN chat_context_resources.source_path IS 'User-declared scan root that produced this resource, or for a discovered row the directory chatd probed. Empty for built-in scan roots.';
