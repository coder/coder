CREATE TABLE aibridge_token_usage_hourly (
    organization_id UUID NOT NULL,
    hour TIMESTAMPTZ NOT NULL,
    effective_group_id UUID NOT NULL,
    initiator_id UUID NOT NULL,
    provider TEXT NOT NULL,
    provider_name TEXT NOT NULL,
    model TEXT NOT NULL,
    client TEXT NOT NULL,
    cost_micros BIGINT NOT NULL DEFAULT 0,
    unpriced_usage_count BIGINT NOT NULL DEFAULT 0 CHECK (unpriced_usage_count >= 0),
    usage_count BIGINT NOT NULL DEFAULT 0 CHECK (usage_count >= 0),
    PRIMARY KEY (organization_id, hour, effective_group_id, initiator_id, provider, provider_name, model, client)
);

CREATE INDEX idx_aibridge_token_usage_hourly_group
    ON aibridge_token_usage_hourly (effective_group_id, hour);

INSERT INTO aibridge_token_usage_hourly (
    organization_id, hour, effective_group_id, initiator_id, provider, provider_name, model, client,
    cost_micros, unpriced_usage_count, usage_count
)
SELECT
    g.organization_id,
    date_trunc('hour', tu.created_at, 'UTC'),
    tu.effective_group_id, ai.initiator_id, ai.provider, ai.provider_name, ai.model,
    COALESCE(ai.client, 'Unknown'),
    COALESCE(SUM(tu.cost_micros), 0)::bigint,
    COUNT(*) FILTER (WHERE tu.cost_micros IS NULL)::bigint,
    COUNT(*)::bigint
FROM aibridge_token_usages tu
JOIN aibridge_interceptions ai ON ai.id = tu.interception_id
JOIN groups g ON g.id = tu.effective_group_id
GROUP BY g.organization_id, 2, tu.effective_group_id, ai.initiator_id,
    ai.provider, ai.provider_name, ai.model, COALESCE(ai.client, 'Unknown');
