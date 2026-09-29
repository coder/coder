-- Record the model reported by the upstream provider, which may differ from
-- the requested model when the provider resolves an alias, and the model whose
-- price was used for the cost.
ALTER TABLE aibridge_token_usages
    ADD COLUMN provider_model TEXT NULL,
    ADD COLUMN priced_model TEXT NULL;

COMMENT ON COLUMN aibridge_token_usages.provider_model IS
    'The model reported by the upstream provider. NULL when the provider did not report one.';

COMMENT ON COLUMN aibridge_token_usages.priced_model IS
    'The model whose price was used to compute the cost, either the requested model or the model reported by the provider. NULL when no price was found for either.';
