-- name: UpsertAIModelPrice :exec
-- Insert a row for (provider, model), or replace its price columns if the
-- pair already exists.
INSERT INTO ai_model_prices (
	provider, model, input_price, output_price, cache_read_price, cache_write_price
) VALUES (
	@provider, @model,
	sqlc.narg('input_price')::bigint,
	sqlc.narg('output_price')::bigint,
	sqlc.narg('cache_read_price')::bigint,
	sqlc.narg('cache_write_price')::bigint
)
ON CONFLICT (provider, model) DO UPDATE SET
	input_price       = EXCLUDED.input_price,
	output_price      = EXCLUDED.output_price,
	cache_read_price  = EXCLUDED.cache_read_price,
	cache_write_price = EXCLUDED.cache_write_price,
	updated_at        = NOW();

-- name: GetAIModelPriceByProviderModel :one
SELECT *
FROM ai_model_prices
WHERE provider = @provider AND model = @model;
