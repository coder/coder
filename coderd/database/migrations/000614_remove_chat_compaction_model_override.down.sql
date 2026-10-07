ALTER TABLE chat_organization_model_overrides
    DROP CONSTRAINT chat_organization_model_overrides_context_check;

ALTER TABLE chat_organization_model_overrides
    ADD CONSTRAINT chat_organization_model_overrides_context_check
        CHECK (context IN ('general', 'explore', 'title_generation', 'compaction', 'advisor'));
