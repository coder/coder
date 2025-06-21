CREATE TABLE template_prebuild_notification_cooldowns (
	template_id UUID NOT NULL,
	notification_type TEXT NOT NULL,
	last_notification_sent TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (template_id, notification_type)
);

COMMENT ON TABLE template_prebuild_notification_cooldowns IS 'Tracks when prebuild failure notifications were last sent to prevent notification noise';
COMMENT ON COLUMN template_prebuild_notification_cooldowns.notification_type IS 'Type of notification: admin or author';
