-- Restore unique constraint on oauth2_provider_apps.name for rollback
-- Note: This rollback may fail if duplicate names exist in the database
ALTER TABLE oauth2_provider_apps ADD CONSTRAINT oauth2_provider_apps_name_key UNIQUE (name);