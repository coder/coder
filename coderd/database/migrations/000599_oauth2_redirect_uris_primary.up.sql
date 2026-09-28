-- Make redirect_uris the source of truth for an OAuth2 app's redirect URIs,
-- with the primary as its first entry. callback_url held the primary until
-- now and stays, deprecated, until a later migration drops it.
--
-- Admin-created apps have an empty or NULL list, apps registered through
-- dynamic client registration already have the callback first, and apps
-- whose callback an admin edited may have it missing or not first. Removing
-- the callback from wherever it appears and prepending it handles all three
-- and is safe to run again.
--
-- During a rolling upgrade, a replica older than this migration still
-- writes only callback_url when an admin edits the callback. Replicas at
-- this version read redirect_uris instead, so the edit is silently lost:
-- the app keeps its old callback and keeps accepting every previous
-- redirect URI, including any the admin meant to remove. The new replicas
-- also show the old callback in the form, so saving the app unchanged
-- does not restore the edit. Drain older replicas before upgrading. If a
-- callback was edited during the upgrade, type the intended value in
-- again and save once every replica runs this version.
UPDATE oauth2_provider_apps
SET redirect_uris = array_prepend(
    callback_url,
    array_remove(COALESCE(redirect_uris, '{}'::text[]), callback_url)
)
WHERE redirect_uris[1] IS DISTINCT FROM callback_url;

COMMENT ON COLUMN oauth2_provider_apps.callback_url IS 'Deprecated: the primary redirect URI is the first entry of redirect_uris. Every writer keeps this column equal to it until the column is dropped.';
COMMENT ON COLUMN oauth2_provider_apps.redirect_uris IS 'Redirect URIs the authorize and token endpoints accept. The first entry is the primary, used when a request omits redirect_uri.';
