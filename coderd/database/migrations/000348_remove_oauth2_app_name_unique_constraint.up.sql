-- Remove unique constraint on oauth2_provider_apps.name to comply with RFC 7591
-- RFC 7591 does not require unique client names, only unique client IDs
ALTER TABLE oauth2_provider_apps DROP CONSTRAINT oauth2_provider_apps_name_key;