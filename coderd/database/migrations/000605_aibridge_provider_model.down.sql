ALTER TABLE aibridge_token_usages
    DROP COLUMN IF EXISTS priced_model,
    DROP COLUMN IF EXISTS provider_model;
