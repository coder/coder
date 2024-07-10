CREATE TYPE notification_message_status AS ENUM (
    'pending',
    'leased',
    'sent',
    'permanent_failure',
    'temporary_failure',
    'unknown'
    );

CREATE TYPE notification_method AS ENUM (
    'smtp',
    'webhook'
    );

CREATE TABLE notification_templates
(
    id             uuid                 NOT NULL,
    name           text                 NOT NULL,
    title_template text                 NOT NULL,
    body_template  text                 NOT NULL,
    actions        jsonb,
    "group"        text,
    PRIMARY KEY (id),
    UNIQUE (name)
);

COMMENT ON TABLE notification_templates IS 'Templates from which to create notification messages.';

CREATE TABLE notification_messages
(
    id                       uuid                        NOT NULL,
    notification_template_id uuid                        NOT NULL,
    user_id                  uuid                        NOT NULL,
    method                   notification_method         NOT NULL,
    status                   notification_message_status NOT NULL DEFAULT 'pending'::notification_message_status,
    status_reason            text,
    created_by               text                        NOT NULL,
    payload                  jsonb                       NOT NULL,
    attempt_count            int                                  DEFAULT 0,
    targets                  uuid[],
    created_at               timestamp with time zone    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               timestamp with time zone,
    leased_until             timestamp with time zone,
    next_retry_after         timestamp with time zone,
    PRIMARY KEY (id),
    FOREIGN KEY (notification_template_id) REFERENCES notification_templates (id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE INDEX idx_notification_messages_status ON notification_messages (status);

-- TODO: autogenerate constants which reference the UUIDs
INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('f517da0b-cdc9-410f-ab89-a86107c420ed', 'Workspace Deleted', E'Workspace "{{.Labels.name}}" deleted',
        E'Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** was deleted.\nThe specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} ({{ .Labels.initiator }}){{end}}**".',
        'Workspace Events', '[
        {
            "label": "View workspaces",
            "url": "{{ base_url }}/workspaces"
        },
        {
            "label": "View templates",
            "url": "{{ base_url }}/templates"
        }
    ]'::jsonb);
