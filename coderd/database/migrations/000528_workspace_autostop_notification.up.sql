ALTER TABLE templates ADD COLUMN time_til_autostop_notify bigint DEFAULT 0 NOT NULL;

COMMENT ON COLUMN templates.time_til_autostop_notify IS 'How long before the workspace autostop deadline to send a reminder notification, in nanoseconds. 0 disables the notification.';

DROP VIEW template_with_names;

CREATE VIEW template_with_names AS
SELECT templates.*,
	   COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
	   COALESCE(visible_users.username, ''::text) AS created_by_username,
	   COALESCE(visible_users.name, ''::text) AS created_by_name,
	   COALESCE(organizations.name, ''::text) AS organization_name,
	   COALESCE(organizations.display_name, ''::text) AS organization_display_name,
	   COALESCE(organizations.icon, ''::text) AS organization_icon
FROM ((templates
	LEFT JOIN visible_users ON ((templates.created_by = visible_users.id)))
	LEFT JOIN organizations ON ((templates.organization_id = organizations.id)));

COMMENT ON VIEW template_with_names IS 'Joins in the display name information such as username, avatar, and organization name.';

ALTER TABLE workspace_builds ADD COLUMN notified_autostop_deadline timestamptz DEFAULT '0001-01-01 00:00:00+00' NOT NULL;

COMMENT ON COLUMN workspace_builds.notified_autostop_deadline IS 'The autostop deadline value that an autostop reminder notification was last sent for. Used for idempotence: when it equals the build deadline the reminder has already been sent, and it re-arms automatically when the deadline changes.';

DROP VIEW workspace_build_with_user;

CREATE VIEW workspace_build_with_user AS
SELECT
    workspace_builds.id,
    workspace_builds.created_at,
    workspace_builds.updated_at,
    workspace_builds.workspace_id,
    workspace_builds.template_version_id,
    workspace_builds.build_number,
    workspace_builds.transition,
    workspace_builds.initiator_id,
    workspace_builds.job_id,
    workspace_builds.deadline,
    workspace_builds.reason,
    workspace_builds.daily_cost,
    workspace_builds.max_deadline,
    workspace_builds.template_version_preset_id,
    workspace_builds.has_ai_task,
    workspace_builds.has_external_agent,
    workspace_builds.notified_autostop_deadline,
    COALESCE(visible_users.avatar_url, ''::text) AS initiator_by_avatar_url,
    COALESCE(visible_users.username, ''::text) AS initiator_by_username,
    COALESCE(visible_users.name, ''::text) AS initiator_by_name
FROM
    workspace_builds
LEFT JOIN
    visible_users ON workspace_builds.initiator_id = visible_users.id;

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';

INSERT INTO notification_templates (
    id, name, title_template, body_template, actions, "group", method, kind, enabled_by_default
) VALUES (
    '6f6cb984-c167-4fa5-bb87-1058dd642779',
    'Workspace Autostop Reminder',
    E'Your workspace "{{.Labels.workspace}}" will stop soon',
    E'Your workspace **{{.Labels.workspace}}** is scheduled to automatically stop at {{.Labels.deadline}}.\n\nConnect to it or extend the deadline to keep it running.',
    '[{"label": "View workspace", "url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"}]'::jsonb,
    'Workspace Events',
    NULL,
    'system'::notification_template_kind,
    true
);
