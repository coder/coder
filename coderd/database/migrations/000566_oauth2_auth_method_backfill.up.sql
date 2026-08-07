-- token_endpoint_auth_method is the client's own declaration, registered client
-- metadata under RFC 7591 section 2, where "none" is defined to mean the client
-- is public and has no secret. client_type is Coder's derived copy of that same
-- fact, and it is what the token endpoint actually enforces on. RFC 7591 defines
-- no client_type metadata field, so the column is a denormalization that can
-- contradict its source.
--
-- Registration used to persist the declaration verbatim while hardcoding
-- client_type to 'confidential', so rows exist saying "auth method: none" on a
-- client stored confidential that was issued, and still requires, a real secret.
-- A client that reads its own metadata and believes it is public will drop that
-- secret and stop being able to exchange codes.
--
-- Align the declaration to what is enforced, not the reverse. Deriving
-- enforcement from the declaration would reclassify every such client as public
-- and stop requiring the secret it holds, which is a silent authentication
-- downgrade.
-- Both branches name the values that are valid for their client type and repair
-- everything else, rather than enumerating the bad values known today. A
-- confidential row holding '' or an unrecognized method would otherwise survive
-- untouched, and it would also satisfy the cross-column constraint this is
-- heading towards, since ('' = 'none') and (client_type = 'public') are both
-- false. Nothing would ever look at it again while RFC 7592 GET kept handing
-- the client a declaration it cannot use.
--
-- The IS NULL arm is not redundant: NULL NOT IN (...) evaluates to NULL, and
-- WHERE admits only true, so without it NULL rows would stop being repaired.
UPDATE oauth2_provider_apps
SET token_endpoint_auth_method = 'client_secret_basic' -- the RFC 7591 section 2 default for a client with a secret
WHERE client_type = 'confidential'
  AND (token_endpoint_auth_method IS NULL
       OR token_endpoint_auth_method NOT IN ('client_secret_basic', 'client_secret_post'));

-- The mirror case is not reachable through any current code path, since a public
-- client is only ever created by requesting 'none'. Included so the invariant
-- holds for the whole table rather than for the half that had a known bug.
UPDATE oauth2_provider_apps
SET token_endpoint_auth_method = 'none'
WHERE client_type = 'public'
  AND (token_endpoint_auth_method IS NULL OR token_endpoint_auth_method <> 'none');
