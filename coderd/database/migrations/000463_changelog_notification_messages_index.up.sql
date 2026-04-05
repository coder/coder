CREATE INDEX IF NOT EXISTS notification_messages_changelog_version_user_idx
ON notification_messages (
    notification_template_id,
    (payload -> 'labels' ->> 'version'),
    user_id
);
