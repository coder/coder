UPDATE notification_templates
SET
    body_template = $$User **{{.Labels.username}}** has used more than {{.Labels.threshold}}% of their {{.Labels.period}} AI budget ({{.Labels.limit}}).

Effective group: **{{.Labels.effective_group_name}}**
{{- if eq .Labels.limit_source "user_override"}}

This limit is a per-user override.
{{- end}}

AI budget period: {{.Labels.period_start}} - {{.Labels.period_end}}$$,
    actions = '[]'::jsonb
WHERE
    id = '2a7b0ac1-00e1-4625-9cd5-1e5933972c77';

UPDATE notification_templates
SET
    body_template = $$User **{{.Labels.username}}** has reached their {{.Labels.period}} AI budget limit ({{.Labels.limit}}). Subsequent requests will be blocked.

Effective group: **{{.Labels.effective_group_name}}**
{{- if eq .Labels.limit_source "user_override"}}

This limit is a per-user override.
{{- end}}

AI budget period: {{.Labels.period_start}} - {{.Labels.period_end}}$$,
    actions = '[]'::jsonb
WHERE
    id = '0bafe0ea-a78b-4217-ad05-1ef12e92e025';
