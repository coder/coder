ALTER TABLE oauth2_provider_apps
    ALTER COLUMN client_type DROP NOT NULL;

ALTER TABLE oauth2_provider_apps
    DROP CONSTRAINT oauth2_provider_apps_client_type_check;
