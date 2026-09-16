-- Make redirect_uris the source of truth for an OAuth2 app's redirect URIs,
-- with the primary as its first entry. callback_url held the primary until
-- now and stays, deprecated, until a later migration drops it.
--
-- Admin-created apps have an empty or NULL list, apps registered through
-- dynamic client registration already have the callback first, and apps
-- whose callback an admin edited may have it missing or not first. Removing
-- the callback from wherever it appears and prepending it handles all three
-- and is safe to run again.
UPDATE oauth2_provider_apps
SET redirect_uris = array_prepend(
    callback_url,
    array_remove(COALESCE(redirect_uris, '{}'::text[]), callback_url)
)
WHERE redirect_uris[1] IS DISTINCT FROM callback_url;

COMMENT ON COLUMN oauth2_provider_apps.callback_url IS 'Deprecated: the primary redirect URI is redirect_uris[1]. Every writer keeps this column equal to it until the column is dropped.';
COMMENT ON COLUMN oauth2_provider_apps.redirect_uris IS 'Redirect URIs the authorize and token endpoints accept. The first entry is the primary, used when a request omits redirect_uri.';
