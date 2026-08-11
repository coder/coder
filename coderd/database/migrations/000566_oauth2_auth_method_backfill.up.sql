-- token_endpoint_auth_method is the client's declared auth method (RFC 7591
-- section 2); client_type is Coder's own field, and it is what the token
-- endpoint actually enforces. The two can disagree: registration used to
-- persist a client's "none" declaration while hardcoding client_type to
-- confidential, so some clients declare themselves public while still
-- holding, and needing, a real secret.
--
-- Align the declaration to what is enforced, not the reverse: deriving
-- enforcement from the declaration would silently stop requiring the secret
-- those clients already hold.
--
-- Each branch repairs anything outside the valid set for its client_type,
-- not just the one bad value seen so far, so a stray '' or unrecognized
-- method can't slip through and later pass a client_type/auth_method
-- consistency check unnoticed.
--
-- The IS NULL arm matters: NULL NOT IN (...) evaluates to NULL, and WHERE
-- only admits true, so without it NULL rows would stop being repaired.
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
