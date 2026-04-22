-- Add "echo" to the chat_providers provider check constraint for
-- the built-in test provider used in development mode.
ALTER TABLE chat_providers DROP CONSTRAINT chat_providers_provider_check;
ALTER TABLE chat_providers ADD CONSTRAINT chat_providers_provider_check
    CHECK (provider = ANY (ARRAY[
        'anthropic'::text,
        'azure'::text,
        'bedrock'::text,
        'echo'::text,
        'google'::text,
        'openai'::text,
        'openai-compat'::text,
        'openrouter'::text,
        'vercel'::text
    ]));
