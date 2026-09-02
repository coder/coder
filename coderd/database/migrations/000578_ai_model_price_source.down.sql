-- Custom prices have no place in the single-row model, and a model may hold
-- both rows, so they are dropped to restore uniqueness on (provider, model).
DELETE FROM ai_model_prices WHERE source = 'custom';

ALTER TABLE ai_model_prices DROP CONSTRAINT ai_model_prices_pkey;

ALTER TABLE ai_model_prices ADD PRIMARY KEY (provider, model);

ALTER TABLE ai_model_prices DROP COLUMN source;

DROP TYPE ai_model_price_source;
