-- source records where a price came from, either the price book embedded in
-- each Coder release or a price set through the API. Both can exist for the
-- same model, so source joins the primary key.

CREATE TYPE ai_model_price_source AS ENUM ('default', 'custom');

ALTER TABLE ai_model_prices ADD COLUMN source ai_model_price_source;

UPDATE ai_model_prices SET source = 'default' WHERE source IS NULL;

ALTER TABLE ai_model_prices ALTER COLUMN source SET NOT NULL;

ALTER TABLE ai_model_prices DROP CONSTRAINT ai_model_prices_pkey;

ALTER TABLE ai_model_prices ADD PRIMARY KEY (provider, model, source);

COMMENT ON COLUMN ai_model_prices.source IS 'Where the price came from: default for the embedded price book, custom for a price set through the API. Both can exist for the same model.';
