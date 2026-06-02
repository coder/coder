-- name: UpsertAIModelPrices :exec
-- Upsert a batch of (provider, model) rows from a JSON array. Each element
-- must have provider, model, and the four price fields; null prices are
-- written as SQL NULL.
INSERT INTO ai_model_prices (
	provider, model, input_price, output_price, cache_read_price, cache_write_price
)
SELECT
	elem->>'provider',
	elem->>'model',
	(elem->>'input_price')::bigint,
	(elem->>'output_price')::bigint,
	(elem->>'cache_read_price')::bigint,
	(elem->>'cache_write_price')::bigint
FROM jsonb_array_elements(@seed::jsonb) AS elem
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

-- name: GetGroupAIBudget :one
SELECT *
FROM group_ai_budgets
WHERE group_id = @group_id;

-- name: UpsertGroupAIBudget :one
INSERT INTO group_ai_budgets (group_id, spend_limit_micros)
VALUES (@group_id, @spend_limit_micros)
ON CONFLICT (group_id) DO UPDATE SET
	spend_limit_micros = EXCLUDED.spend_limit_micros,
	updated_at  = NOW()
RETURNING *;

-- name: DeleteGroupAIBudget :one
DELETE FROM group_ai_budgets WHERE group_id = @group_id RETURNING *;

-- name: GetUserAIBudgetOverride :one
SELECT *
FROM user_ai_budget_overrides
WHERE user_id = @user_id;

-- name: UpsertUserAIBudgetOverride :one
INSERT INTO user_ai_budget_overrides (user_id, group_id, spend_limit_micros)
VALUES (@user_id, @group_id, @spend_limit_micros)
ON CONFLICT (user_id) DO UPDATE SET
	group_id           = EXCLUDED.group_id,
	spend_limit_micros = EXCLUDED.spend_limit_micros,
	updated_at         = NOW()
RETURNING *;

-- name: DeleteUserAIBudgetOverride :one
DELETE FROM user_ai_budget_overrides WHERE user_id = @user_id RETURNING *;
