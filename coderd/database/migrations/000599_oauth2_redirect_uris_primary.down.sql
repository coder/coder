-- The backfilled list is a superset the previous code already handles, so
-- only the column comments are restored.
COMMENT ON COLUMN oauth2_provider_apps.callback_url IS NULL;
COMMENT ON COLUMN oauth2_provider_apps.redirect_uris IS 'List of valid redirect URIs for the application';
