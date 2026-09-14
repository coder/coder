-- Widen ai_provider_type to carry the full chatd provider set so the
-- chatd-side migration can preserve type fidelity when it lands. The
-- aibridge runtime currently has native support only for OpenAI and
-- Anthropic (with a Bedrock variant on the Anthropic client); the new
-- non-Bedrock types route through the OpenAI fantasy client today
-- because chatd already configures these providers against their
-- OpenAI-compatible endpoints. Native gateway-side support for these
-- providers comes later, at which point this enum already carries the
-- right discriminator and no further migration is needed.
--
-- Recreate the type rather than using ALTER TYPE ... ADD VALUE. Postgres
-- forbids using a value added by ADD VALUE within the same transaction, and
-- all migrations run in one transaction. 000504 casts existing chat_providers
-- rows to these new values in that same transaction, so ADD VALUE fails with
-- "unsafe use of new value". A freshly created enum's values are usable
-- immediately, so the cast in 000504 succeeds.
CREATE TYPE new_ai_provider_type AS ENUM (
    'openai',
    'anthropic',
    'azure',
    'bedrock',
    'google',
    'openai-compat',
    'openrouter',
    'vercel'
);

ALTER TABLE ai_providers
    ALTER COLUMN type TYPE new_ai_provider_type USING (type::text::new_ai_provider_type);

DROP TYPE ai_provider_type;

ALTER TYPE new_ai_provider_type RENAME TO ai_provider_type;
