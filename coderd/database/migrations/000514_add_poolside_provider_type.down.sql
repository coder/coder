-- Remove the 'poolside' value from ai_provider_type by recreating the enum
-- without it. Any ai_providers rows that reference 'poolside' must be
-- converted (or deleted) before running this migration.
CREATE TYPE new_ai_provider_type AS ENUM (
    'openai',
    'anthropic',
    'azure',
    'bedrock',
    'google',
    'openai-compat',
    'openrouter',
    'vercel',
    'copilot'
);

ALTER TABLE ai_providers
    ALTER COLUMN type TYPE new_ai_provider_type USING (type::text::new_ai_provider_type);

DROP TYPE ai_provider_type;

ALTER TYPE new_ai_provider_type RENAME TO ai_provider_type;
