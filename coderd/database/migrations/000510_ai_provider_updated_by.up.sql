ALTER TABLE ai_providers
    ADD COLUMN updated_by UUID REFERENCES users(id) ON DELETE SET NULL;
