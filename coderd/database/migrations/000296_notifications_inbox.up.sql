CREATE TABLE inbox_notifications (
	id			UUID						PRIMARY KEY,
	user_id			UUID						NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	template_id		UUID						NOT NULL REFERENCES notification_templates(id) ON DELETE CASCADE,
	targets			UUID[],
	title			TEXT						NOT NULL,
	content			TEXT						NOT NULL,
	icon			TEXT						NOT NULL,
	actions			JSONB						NOT NULL,
	read_at			TIMESTAMP WITH TIME ZONE,
	created_at		TIMESTAMP WITH TIME ZONE	NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_inbox_notifications_user_id_read_at ON inbox_notifications(user_id, read_at);
CREATE INDEX idx_inbox_notifications_user_id_template_id_targets ON inbox_notifications(user_id, template_id, targets);
