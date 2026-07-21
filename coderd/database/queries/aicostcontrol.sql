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

-- name: IncrementUserAIDailySpend :one
-- Adds cost_micros to the spend for (user_id, effective_group_id, day).
-- The day parameter is normalized to its UTC calendar day before storage.
INSERT INTO ai_user_daily_spend (user_id, effective_group_id, day, spend_micros)
VALUES (@user_id, @effective_group_id, ((@day::timestamptz) AT TIME ZONE 'UTC')::date, @cost_micros)
ON CONFLICT (user_id, effective_group_id, day) DO UPDATE SET
	spend_micros = ai_user_daily_spend.spend_micros + EXCLUDED.spend_micros
RETURNING *;

-- name: GetUserAISpendSince :one
-- Total spend for (user_id, effective_group_id) on or after period_start until NOW.
-- The period_start parameter is normalized to its UTC calendar day.
SELECT
	@user_id::uuid AS user_id,
	@effective_group_id::uuid AS effective_group_id,
	((@period_start::timestamptz) AT TIME ZONE 'UTC')::date AS period_start,
	COALESCE(SUM(spend_micros), 0)::BIGINT AS spend_micros
FROM ai_user_daily_spend
WHERE user_id = @user_id
	AND effective_group_id = @effective_group_id
	AND day >= ((@period_start::timestamptz) AT TIME ZONE 'UTC')::date;

-- name: GetOrganizationGroupsAISpend :many
-- Returns AI spend limits and aggregate spend for groups in @group_ids that
-- belong to @organization_id, on or after period_start until NOW. The spend
-- limit is null when the group has no configured budget.
-- The period_start parameter is normalized to its UTC calendar day.
SELECT
	groups.id AS group_id,
	groups.organization_id AS organization_id,
	budget.spend_limit_micros AS spend_limit_micros,
	COALESCE(SUM(spend.spend_micros), 0)::BIGINT AS current_spend_micros
FROM groups
LEFT JOIN group_ai_budgets budget ON budget.group_id = groups.id
LEFT JOIN ai_user_daily_spend spend
	ON spend.effective_group_id = groups.id
	AND spend.day >= ((@period_start::timestamptz) AT TIME ZONE 'UTC')::date
WHERE groups.organization_id = @organization_id
	AND groups.id = ANY(@group_ids::uuid[])
GROUP BY groups.id, budget.spend_limit_micros
ORDER BY groups.id;

-- name: GetGroupMembersAISpend :many
-- Returns each user's AI spend attributed to the queried group, on or after
-- period_start until NOW. Only current members of the queried group are
-- returned. spend_limit_micros and limit_source are populated only when the
-- queried group is the user's effective budget source. The effective_group_id
-- is null when the user has no configured budget or when the effective group
-- belongs to a different organization than the queried group.
-- The period_start parameter is normalized to its UTC calendar day.
-- TODO(AIGOV-527): unify effective group resolution in a single place.
WITH queried_group AS (
	-- The queried group's org, used to detect cross-org effective groups.
	SELECT organization_id
	FROM groups
	WHERE id = @group_id
),
filtered_users AS (
	-- Users from @user_ids that are members of the queried group. Uses
	-- group_members_expanded so the implicit Everyone group counts.
	SELECT DISTINCT user_id
	FROM group_members_expanded
	WHERE group_id = @group_id
		AND user_id = ANY(@user_ids::uuid[])
),
user_highest_group AS (
	-- Per user, the highest-limit group they belong to. Uses
	-- group_members_expanded so the implicit Everyone group counts.
	SELECT DISTINCT ON (member.user_id)
		member.user_id,
		budget.group_id,
		budget.spend_limit_micros
	FROM group_ai_budgets budget
	JOIN group_members_expanded member ON member.group_id = budget.group_id
	WHERE member.user_id IN (SELECT user_id FROM filtered_users)
	ORDER BY member.user_id, budget.spend_limit_micros DESC, member.group_name ASC, budget.group_id ASC
),
effective AS (
	-- Effective budget per user: a per-user override wins over the
	-- highest-limit group.
	SELECT
		filtered_users.user_id,
		COALESCE(override.group_id, user_highest_group.group_id) AS raw_effective_group_id,
		COALESCE(override.spend_limit_micros, user_highest_group.spend_limit_micros) AS spend_limit_micros,
		(CASE
			WHEN override.group_id IS NOT NULL THEN 'user_override'
			WHEN user_highest_group.group_id IS NOT NULL THEN 'group'
		END)::text AS limit_source
	FROM filtered_users
	LEFT JOIN user_ai_budget_overrides override ON override.user_id = filtered_users.user_id
	LEFT JOIN user_highest_group ON user_highest_group.user_id = filtered_users.user_id
),
applied_budget AS (
	-- The limit and source only for users whose effective budget source is the
	-- queried group.
	SELECT user_id, spend_limit_micros, limit_source
	FROM effective
	WHERE raw_effective_group_id = @group_id
)
-- Spend is aggregated for the queried group, not the user's effective group.
SELECT
	effective.user_id,
	queried_group.organization_id,
	effective_group.id AS effective_group_id,
	applied_budget.spend_limit_micros,
	applied_budget.limit_source,
	COALESCE(SUM(spend.spend_micros), 0)::BIGINT AS group_spend_micros
FROM effective
CROSS JOIN queried_group
LEFT JOIN groups effective_group
	ON effective_group.id = effective.raw_effective_group_id
	AND effective_group.organization_id = queried_group.organization_id
-- A LEFT JOIN leaves spend_limit_micros and limit_source null for users
-- whose effective budget source is not the queried group.
LEFT JOIN applied_budget ON applied_budget.user_id = effective.user_id
LEFT JOIN ai_user_daily_spend spend
	ON spend.user_id = effective.user_id
	AND spend.effective_group_id = @group_id
	AND spend.day >= ((@period_start::timestamptz) AT TIME ZONE 'UTC')::date
GROUP BY
	effective.user_id,
	queried_group.organization_id,
	effective_group.id,
	applied_budget.spend_limit_micros,
	applied_budget.limit_source
ORDER BY effective.user_id;
