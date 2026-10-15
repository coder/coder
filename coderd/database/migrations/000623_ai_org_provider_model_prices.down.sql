ALTER TABLE aibridge_token_usages
    DROP COLUMN price_provider,
    DROP COLUMN price_provider_id,
    DROP COLUMN price_organization_id,
    DROP COLUMN price_kind;

DROP TYPE ai_model_price_kind;

DROP TABLE ai_org_provider_model_prices;
