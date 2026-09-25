UPDATE aibridge_token_usages
SET effective_group_id = (SELECT id FROM organizations WHERE is_default = true)
WHERE id = 'c56ca89d-af65-47b0-871f-0b9cd2af6575';

INSERT INTO aibridge_token_usage_hourly (
    organization_id, hour, effective_group_id, initiator_id, provider, provider_name, model, client,
    cost_micros, unpriced_usage_count, usage_count
)
SELECT g.organization_id, date_trunc('hour', tu.created_at, 'UTC'), g.id,
    ai.initiator_id, ai.provider, ai.provider_name, ai.model, COALESCE(ai.client, 'Unknown'),
    COALESCE(tu.cost_micros, 0), CASE WHEN tu.cost_micros IS NULL THEN 1 ELSE 0 END, 1
FROM aibridge_token_usages tu
JOIN aibridge_interceptions ai ON ai.id = tu.interception_id
JOIN groups g ON g.id = tu.effective_group_id
WHERE tu.id = 'c56ca89d-af65-47b0-871f-0b9cd2af6575';
