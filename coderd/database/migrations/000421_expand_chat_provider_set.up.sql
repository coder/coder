ALTER TABLE chat_providers
    DROP CONSTRAINT IF EXISTS chat_providers_provider_check;

ALTER TABLE chat_providers
    ADD CONSTRAINT chat_providers_provider_check CHECK (
        provider = ANY (
            ARRAY[
                'anthropic'::text,
                'azure'::text,
                'bedrock'::text,
                'google'::text,
                'openai'::text,
                'openai-compat'::text,
                'openrouter'::text,
                'vercel'::text
            ]
        )
    );
