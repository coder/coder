INSERT INTO notification_templates (
    id,
    name,
    title_template,
    body_template,
    actions,
    "group",
    method,
    kind,
    enabled_by_default
)
VALUES (
    '1b7d9fa7-f5a8-4e46-8078-5cf53abfed94',
    'Report: Unpriced AI Models',
    E'Missing prices for AI models',
    $$These models were used in the last {{.Data.report_frequency}}, but they have no price, so their usage is missing from AI spend and does not count toward any AI budget. Reported spend is lower than actual, and a user who calls only these models has no effective limit.

{{range $model := .Data.models}}
* {{$model.provider}}/{{$model.model}}
{{- end}}
{{if .Data.total_count}}
{{len .Data.models}} of {{.Data.total_count}} models with no price are shown, ordered by usage.
{{end}}
Every Coder release ships with prices for most models, so only the models above need one: see [how spend is calculated](https://coder.com/docs/ai-coder/ai-gateway/cost-controls#how-spend-is-calculated) and [how to configure prices](https://coder.com/docs/ai-coder/ai-gateway/cost-controls#configure-model-prices). Prices are not retroactive, so usage recorded before you set a price stays unpriced.$$,
    '[]'::jsonb,
    'AI Cost Control Admin Events',
    NULL,
    'system'::notification_template_kind,
    true
);
