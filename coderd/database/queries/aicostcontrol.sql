-- name: UpsertAIModelPrices :exec
-- Upsert a batch of model prices from a JSON array, all recorded under the
-- given source. Each element must have provider, model, and the four price
-- fields, and null prices are written as SQL NULL.
-- Each source keeps its own row, so the price book and a custom price never
-- overwrite each other. A conflicting row is only rewritten when a price
-- differs, so updated_at records when a price last changed. Prices are
-- nullable and a NULL on either side counts as a difference.
INSERT INTO ai_model_prices (
	provider, model, input_price, output_price, cache_read_price, cache_write_price, source
)
SELECT
	elem->>'provider',
	elem->>'model',
	(elem->>'input_price')::bigint,
	(elem->>'output_price')::bigint,
	(elem->>'cache_read_price')::bigint,
	(elem->>'cache_write_price')::bigint,
	@source::ai_model_price_source
FROM jsonb_array_elements(@seed::jsonb) AS elem
ON CONFLICT (provider, model, source) DO UPDATE SET
	input_price       = EXCLUDED.input_price,
	output_price      = EXCLUDED.output_price,
	cache_read_price  = EXCLUDED.cache_read_price,
	cache_write_price = EXCLUDED.cache_write_price,
	updated_at        = NOW()
WHERE (
	ai_model_prices.input_price,
	ai_model_prices.output_price,
	ai_model_prices.cache_read_price,
	ai_model_prices.cache_write_price
) IS DISTINCT FROM (
	EXCLUDED.input_price,
	EXCLUDED.output_price,
	EXCLUDED.cache_read_price,
	EXCLUDED.cache_write_price
);

-- name: GetAIModelPriceByProviderModel :one
-- Returns the price in effect for the model, preferring a custom price over
-- the price book.
SELECT *
FROM ai_model_prices
WHERE provider = @provider AND model = @model
ORDER BY CASE WHEN source = 'custom' THEN 0 ELSE 1 END ASC
LIMIT 1;

-- name: GetAIModelPrices :many
-- Returns the price in effect for each model, preferring a custom price over
-- the price book. Filtering by source narrows the rows considered first, so a
-- model carrying both prices reports the one from the named source.
-- The source 'all' reports every row instead. It joins the DISTINCT ON key, so
-- each source forms its own group and nothing collapses. Every other source
-- contributes the same constant, leaving the key as (provider, model).
SELECT DISTINCT ON (
    provider,
    model,
    CASE WHEN @source::text = 'all' THEN source::text ELSE '' END
) *
FROM ai_model_prices
    -- Filter by provider
WHERE CASE
        WHEN @provider::text != '' THEN
            provider = @provider
        ELSE true
    END
    -- Filter by model
    AND CASE
        WHEN @model::text != '' THEN
            model = @model
        ELSE true
    END
    -- Filter by source
    AND CASE
        WHEN @source::text NOT IN ('', 'all') THEN
            source = @source::ai_model_price_source
        ELSE true
    END
ORDER BY
    provider ASC,
    model ASC,
    CASE WHEN @source::text = 'all' THEN source::text ELSE '' END ASC,
    CASE WHEN source = 'custom' THEN 0 ELSE 1 END ASC;

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
-- breaking ties by the earliest organization membership. Implements the
-- "highest" budget policy. group_members_expanded is a UNION of group_members
-- and organization_members, so the implicit "Everyone" group
-- (group_id == organization_id) is included. Returns no rows when the user has
-- no budgeted groups. Callers should treat sql.ErrNoRows as "no group budget".
SELECT
	budget.group_id,
	budget.spend_limit_micros
FROM group_ai_budgets budget
JOIN group_members_expanded member ON member.group_id = budget.group_id
JOIN organizations ON organizations.id = member.organization_id
JOIN organization_members
	ON organization_members.user_id = member.user_id
	AND organization_members.organization_id = member.organization_id
WHERE member.user_id = @user_id
	AND organizations.deleted = false
ORDER BY
	budget.spend_limit_micros DESC,       -- highest wins
	organization_members.created_at ASC,  -- earliest organization membership
	budget.group_id ASC                   -- deterministic tiebreak
LIMIT 1;

-- name: GetUserEveryoneFallbackGroup :one
-- Returns the "Everyone" group (id == organization_id) to attribute a user's
-- spend to when no override or budgeted group applies. Prefers the default org,
-- then the earliest organization membership. Returns no rows when the user has
-- no organization membership.
SELECT organizations.id AS group_id
FROM organization_members
JOIN organizations ON organizations.id = organization_members.organization_id
WHERE organization_members.user_id = @user_id
	AND organizations.deleted = false
ORDER BY
	organizations.is_default DESC,        -- prefer the default org
	organization_members.created_at ASC,  -- earliest organization membership
	organizations.id ASC                  -- deterministic tiebreak
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
-- belong to @organization_id, on or after period_start until NOW.
-- spend_limit_micros is the per-member limit, null when the group has no budget.
-- total_spend_limit_micros is the combined budget of the members attributed to
-- the group, with each member's override replacing their share. It is null when
-- the group has no budget.
-- The period_start parameter is normalized to its UTC calendar day.
-- TODO(AIGOV-527): unify effective group resolution in a single place.
WITH queried_groups AS (
	-- The requested groups that belong to the queried organization.
	SELECT groups.id, groups.organization_id
	FROM groups
	WHERE groups.organization_id = @organization_id
		AND groups.id = ANY(@group_ids::uuid[])
),
candidate_users AS (
	-- Members of the queried groups. Uses group_members_expanded so the implicit
	-- Everyone group counts.
	SELECT DISTINCT member.user_id
	FROM group_members_expanded member
	WHERE member.group_id IN (SELECT id FROM queried_groups)
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
	JOIN organizations ON organizations.id = member.organization_id
	JOIN organization_members
		ON organization_members.user_id = member.user_id
		AND organization_members.organization_id = member.organization_id
	WHERE member.user_id IN (SELECT user_id FROM candidate_users)
		AND organizations.deleted = false
	ORDER BY member.user_id, budget.spend_limit_micros DESC, organization_members.created_at ASC, budget.group_id ASC
),
effective AS (
	-- Effective budget group per user: an override wins over the highest-limit
	-- group they belong to. Users with neither are left out, since the group they
	-- fall back to has no budget and reports null.
	SELECT
		candidate_users.user_id,
		COALESCE(override.group_id, user_highest_group.group_id) AS effective_group_id,
		override.spend_limit_micros AS override_limit_micros
	FROM candidate_users
	LEFT JOIN user_ai_budget_overrides override ON override.user_id = candidate_users.user_id
	LEFT JOIN user_highest_group ON user_highest_group.user_id = candidate_users.user_id
),
group_limits AS (
	-- Per attributed group, how many members take the group's own limit and the
	-- combined limit of those carrying an override.
	SELECT
		effective.effective_group_id AS group_id,
		count(*) FILTER (WHERE effective.override_limit_micros IS NULL) AS plain_member_count,
		COALESCE(SUM(effective.override_limit_micros), 0)::BIGINT AS override_limit_sum
	FROM effective
	WHERE effective.effective_group_id IS NOT NULL
	GROUP BY effective.effective_group_id
),
group_totals AS (
	-- Combined limit per budgeted group, counting members with no override at the
	-- group's own limit and adding the overrides on top. Unbudgeted groups are
	-- absent here, so the join below leaves their total null.
	SELECT
		queried_groups.id AS group_id,
		(budget.spend_limit_micros * COALESCE(group_limits.plain_member_count, 0)
			+ COALESCE(group_limits.override_limit_sum, 0))::BIGINT AS total_spend_limit_micros
	FROM queried_groups
	JOIN group_ai_budgets budget ON budget.group_id = queried_groups.id
	LEFT JOIN group_limits ON group_limits.group_id = queried_groups.id
),
group_spend AS (
	-- Spend per queried group over the period.
	SELECT
		spend.effective_group_id AS group_id,
		COALESCE(SUM(spend.spend_micros), 0)::BIGINT AS current_spend_micros
	FROM ai_user_daily_spend spend
	WHERE spend.effective_group_id IN (SELECT id FROM queried_groups)
		AND spend.day >= ((@period_start::timestamptz) AT TIME ZONE 'UTC')::date
	GROUP BY spend.effective_group_id
)
SELECT
	queried_groups.id AS group_id,
	queried_groups.organization_id AS organization_id,
	budget.spend_limit_micros AS spend_limit_micros,
	group_totals.total_spend_limit_micros AS total_spend_limit_micros,
	COALESCE(group_spend.current_spend_micros, 0)::BIGINT AS current_spend_micros
FROM queried_groups
LEFT JOIN group_ai_budgets budget ON budget.group_id = queried_groups.id
LEFT JOIN group_totals ON group_totals.group_id = queried_groups.id
LEFT JOIN group_spend ON group_spend.group_id = queried_groups.id
ORDER BY queried_groups.id;

-- name: GetGroupMembersAISpend :many
-- Returns each user's AI spend attributed to the queried group, on or after
-- period_start until NOW. Only current members of the queried group are
-- returned. effective_spend_limit_micros and effective_limit_source describe
-- the user's effective budget when its group belongs to the queried group's
-- organization. group_spend_limit_micros and limit_source are populated only
-- when the queried group is the user's effective budget source. The
-- effective group falls back to the Everyone group, and effective_group_id is
-- null only when that group belongs to a different organization than the
-- queried group.
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
	JOIN organizations ON organizations.id = member.organization_id
	JOIN organization_members
		ON organization_members.user_id = member.user_id
		AND organization_members.organization_id = member.organization_id
	WHERE member.user_id IN (SELECT user_id FROM filtered_users)
		AND organizations.deleted = false
	ORDER BY member.user_id, budget.spend_limit_micros DESC, organization_members.created_at ASC, budget.group_id ASC
),
user_fallback_group AS (
	-- Per user, the Everyone group to fall back to when no override or budgeted
	-- group applies. The Everyone group has id == organization_id. Prefers the
	-- default org, then the earliest organization membership.
	SELECT DISTINCT ON (organization_members.user_id)
		organization_members.user_id,
		organizations.id AS group_id
	FROM organization_members
	JOIN organizations ON organizations.id = organization_members.organization_id
	WHERE organization_members.user_id IN (SELECT user_id FROM filtered_users)
		AND organizations.deleted = false
	ORDER BY organization_members.user_id, organizations.is_default DESC, organization_members.created_at ASC, organizations.id ASC
),
effective AS (
	-- Effective budget per user: a per-user override wins over the highest-limit
	-- group, which wins over the Everyone group fallback.
	SELECT
		filtered_users.user_id,
		COALESCE(override.group_id, user_highest_group.group_id, user_fallback_group.group_id) AS raw_effective_group_id,
		COALESCE(override.spend_limit_micros, user_highest_group.spend_limit_micros) AS spend_limit_micros,
		(CASE
			WHEN override.group_id IS NOT NULL THEN 'user_override'
			WHEN user_highest_group.group_id IS NOT NULL THEN 'group'
		END)::text AS limit_source
	FROM filtered_users
	LEFT JOIN user_ai_budget_overrides override ON override.user_id = filtered_users.user_id
	LEFT JOIN user_highest_group ON user_highest_group.user_id = filtered_users.user_id
	LEFT JOIN user_fallback_group ON user_fallback_group.user_id = filtered_users.user_id
),
applied_budget AS (
	-- The limit and source only for users whose effective budget source is the
	-- queried group.
	SELECT user_id, spend_limit_micros, limit_source
	FROM effective
	WHERE raw_effective_group_id = @group_id
),
visible_effective_budget AS (
	-- The effective budget only when its group belongs to the queried group's
	-- organization.
	SELECT effective.user_id, effective.spend_limit_micros, effective.limit_source
	FROM effective
	JOIN groups ON groups.id = effective.raw_effective_group_id
	CROSS JOIN queried_group
	WHERE groups.organization_id = queried_group.organization_id
)
-- Spend is aggregated for the queried group, not the user's effective group.
SELECT
	effective.user_id,
	queried_group.organization_id,
	effective_group.id AS effective_group_id,
	visible_effective_budget.spend_limit_micros AS effective_spend_limit_micros,
	visible_effective_budget.limit_source AS effective_limit_source,
	applied_budget.spend_limit_micros AS group_spend_limit_micros,
	applied_budget.limit_source,
	COALESCE(SUM(spend.spend_micros), 0)::BIGINT AS group_spend_micros
FROM effective
CROSS JOIN queried_group
LEFT JOIN groups effective_group
	ON effective_group.id = effective.raw_effective_group_id
	AND effective_group.organization_id = queried_group.organization_id
LEFT JOIN visible_effective_budget
	ON visible_effective_budget.user_id = effective.user_id
-- A LEFT JOIN leaves group_spend_limit_micros and limit_source null for users
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
	visible_effective_budget.spend_limit_micros,
	visible_effective_budget.limit_source,
	applied_budget.spend_limit_micros,
	applied_budget.limit_source
ORDER BY effective.user_id;

-- name: GetOverBudgetUsersPerGroup :many
-- Returns, per effective group, the number of users at or over their spend
-- limit since period_start. Only users with an enforceable limit (override or
-- budgeted group) count, and the unlimited Everyone fallback does not.
-- TODO(AIGOV-527): unify effective group resolution in a single place.
WITH budgeted_users AS (
	-- Users with an override or membership in a budgeted group.
	SELECT user_id FROM user_ai_budget_overrides
	UNION
	SELECT DISTINCT member.user_id
	FROM group_ai_budgets budget
	JOIN group_members_expanded member ON member.group_id = budget.group_id
),
user_highest_group AS (
	-- Per user, their highest-limit group ("highest" budget policy).
	SELECT DISTINCT ON (member.user_id)
		member.user_id,
		budget.group_id,
		budget.spend_limit_micros
	FROM group_ai_budgets budget
	JOIN group_members_expanded member ON member.group_id = budget.group_id
	JOIN organizations ON organizations.id = member.organization_id
	JOIN organization_members
		ON organization_members.user_id = member.user_id
		AND organization_members.organization_id = member.organization_id
	WHERE member.user_id IN (SELECT user_id FROM budgeted_users)
		AND organizations.deleted = false
	ORDER BY member.user_id, budget.spend_limit_micros DESC, organization_members.created_at ASC, budget.group_id ASC
),
effective AS (
	-- An override wins over the highest-limit group, and users with neither drop.
	SELECT
		budgeted_users.user_id,
		COALESCE(override.group_id, user_highest_group.group_id) AS effective_group_id,
		COALESCE(override.spend_limit_micros, user_highest_group.spend_limit_micros) AS spend_limit_micros
	FROM budgeted_users
	LEFT JOIN user_ai_budget_overrides override ON override.user_id = budgeted_users.user_id
	LEFT JOIN user_highest_group ON user_highest_group.user_id = budgeted_users.user_id
	WHERE COALESCE(override.group_id, user_highest_group.group_id) IS NOT NULL
),
user_spend AS (
	-- Each user's spend against their effective group since period_start.
	SELECT
		effective.user_id,
		effective.effective_group_id,
		effective.spend_limit_micros,
		COALESCE(SUM(spend.spend_micros), 0)::BIGINT AS current_spend_micros
	FROM effective
	LEFT JOIN ai_user_daily_spend spend
		ON spend.user_id = effective.user_id
		AND spend.effective_group_id = effective.effective_group_id
		AND spend.day >= ((@period_start::timestamptz) AT TIME ZONE 'UTC')::date
	GROUP BY effective.user_id, effective.effective_group_id, effective.spend_limit_micros
)
SELECT
	effective_group_id AS group_id,
	COUNT(*)::BIGINT AS over_budget_users
FROM user_spend
WHERE current_spend_micros >= spend_limit_micros
GROUP BY effective_group_id
ORDER BY effective_group_id;

-- name: ExportOrganizationAISpend :many
-- Returns per-user, per-group, per-model, per-provider aggregated AI spend for
-- @organization_id over the [period_start, period_end) window. Spend is
-- attributed through the token usage's effective group, and rows are bucketed
-- by the token usage created_at, matching how ai_user_daily_spend is derived.
SELECT
	ai.initiator_id AS user_id,
	users.username AS username,
	tu.effective_group_id AS group_id,
	groups.name AS group_name,
	groups.organization_id AS organization_id,
	organizations.name AS organization_name,
	ai.model AS model,
	ai.provider AS provider,
	ai.provider_name AS provider_name,
	COALESCE(SUM(tu.input_tokens), 0)::BIGINT AS input_tokens,
	COALESCE(SUM(tu.output_tokens), 0)::BIGINT AS output_tokens,
	COALESCE(SUM(tu.cache_read_input_tokens), 0)::BIGINT AS cache_read_tokens,
	COALESCE(SUM(tu.cache_write_input_tokens), 0)::BIGINT AS cache_write_tokens,
	COALESCE(SUM(tu.cost_micros), 0)::BIGINT AS cost_micros
FROM aibridge_token_usages tu
JOIN aibridge_interceptions ai ON ai.id = tu.interception_id
JOIN users ON users.id = ai.initiator_id
JOIN groups ON groups.id = tu.effective_group_id
JOIN organizations ON organizations.id = groups.organization_id
WHERE groups.organization_id = @organization_id
	AND tu.created_at >= @period_start::timestamptz
	AND tu.created_at < @period_end::timestamptz
GROUP BY
	ai.initiator_id,
	users.username,
	tu.effective_group_id,
	groups.name,
	groups.organization_id,
	organizations.name,
	ai.model,
	ai.provider,
	ai.provider_name
ORDER BY ai.initiator_id, tu.effective_group_id, ai.provider, ai.provider_name, ai.model;
