ALTER TABLE ai_providers
    ADD COLUMN icon text NOT NULL DEFAULT '';

COMMENT ON COLUMN ai_providers.icon IS 'Optional icon URL for display in provider lists and model pickers.';
