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

-- name: GetHighestGroupAIBudgetByUser :one
-- Returns the highest group AI budget across the groups the user belongs to,
-- breaking ties by group name ascending. Implements the "highest" budget policy.
-- group_members_expanded is a UNION of group_members and organization_members,
-- so the implicit "Everyone" group (group_id == organization_id) is included.
-- Returns no rows when the user has no budgeted groups; callers should treat
-- sql.ErrNoRows as "no group budget".
SELECT
	gaib.group_id,
	gaib.spend_limit_micros
FROM group_ai_budgets gaib
JOIN group_members_expanded gme ON gme.group_id = gaib.group_id
WHERE gme.user_id = @user_id
ORDER BY
	gaib.spend_limit_micros DESC, -- highest wins
	gme.group_name ASC,           -- alphabetical tiebreak
	-- Final tiebreak on the group id makes the result deterministic when two
	-- groups share both name and limit, which is possible across organizations
	-- (groups are unique on (organization_id, name), not name alone).
	gaib.group_id ASC
LIMIT 1;
