-- source records where a price came from, either the price book embedded in
-- each Coder release or a price set through the API. The startup seeder
-- re-applies the book on every boot and reads source to leave custom rows
-- alone.

CREATE TYPE ai_model_price_source AS ENUM ('default', 'custom');

ALTER TABLE ai_model_prices ADD COLUMN source ai_model_price_source;

UPDATE ai_model_prices SET source = 'default' WHERE source IS NULL;

ALTER TABLE ai_model_prices ALTER COLUMN source SET NOT NULL;

COMMENT ON COLUMN ai_model_prices.source IS 'Where the price came from: default for the embedded price book, custom for a price an operator set. The startup seeder never overwrites a custom row.';
