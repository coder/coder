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
    E'AI models used without a price',
    $$The following models were used in the last {{.Data.report_frequency}} without a price. Their token usage is recorded, but it adds nothing to spend, so it is neither reported nor enforced against any budget.

{{range $model := .Data.models}}
* {{$model.provider}}/{{$model.model}}
{{- end}}
{{if .Data.overflow_count}}
...and {{.Data.overflow_count}} more.
{{end}}
Set a price for these models to bring their usage into spend reporting.$$,
    '[{"label": "Configure model prices", "url": "https://coder.com/docs/ai-coder/ai-gateway/cost-controls#configure-model-prices"}]'::jsonb,
    'AI Cost Control Admin Events',
    NULL,
    'system'::notification_template_kind,
    true
);
