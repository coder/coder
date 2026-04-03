CREATE TYPE credential_kind AS ENUM ('centralized', 'byok_api_key', 'byok_subscription');

-- Records how each LLM request was authenticated and a masked credential
-- identifier for audit purposes. Existing rows default to 'centralized'
-- with an empty hint since we cannot retroactively determine their values.
ALTER TABLE aibridge_interceptions
    ADD COLUMN credential_kind credential_kind NOT NULL DEFAULT 'centralized',
    ADD COLUMN credential_hint TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN aibridge_interceptions.credential_kind IS 'How the request was authenticated: centralized, byok_api_key, or byok_subscription.';
COMMENT ON COLUMN aibridge_interceptions.credential_hint IS 'Masked credential identifier for audit (e.g. sk-****efgh).';
