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
    '2a7b0ac1-00e1-4625-9cd5-1e5933972c77',
    'User Approaching AI Budget Limit',
    E'{{.Labels.username}} is approaching their {{.Labels.period}} AI budget limit',
    $$User **{{.Labels.username}}** has used more than {{.Labels.threshold}}% of their {{.Labels.period}} AI budget ({{.Labels.limit}}).

Effective group: **{{.Labels.effective_group_name}}**
{{- if eq .Labels.limit_source "user_override"}}

This limit is a per-user override.
{{- end}}

AI budget period: {{.Labels.period_start}} - {{.Labels.period_end}}$$,
    '[]'::jsonb,
    'AI Cost Control Admin Events',
    NULL,
    'system'::notification_template_kind,
    true
);

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
    '0bafe0ea-a78b-4217-ad05-1ef12e92e025',
    'User Reached AI Budget Limit',
    E'{{.Labels.username}} has reached their {{.Labels.period}} AI budget limit',
    $$User **{{.Labels.username}}** has reached their {{.Labels.period}} AI budget limit ({{.Labels.limit}}). Subsequent requests will be blocked.

Effective group: **{{.Labels.effective_group_name}}**
{{- if eq .Labels.limit_source "user_override"}}

This limit is a per-user override.
{{- end}}

AI budget period: {{.Labels.period_start}} - {{.Labels.period_end}}$$,
    '[]'::jsonb,
    'AI Cost Control Admin Events',
    NULL,
    'system'::notification_template_kind,
    true
);
