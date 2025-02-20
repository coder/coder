CREATE TABLE notifications_inbox (
	id 				UUID 						PRIMARY KEY,
	user_id 		UUID 						NOT NULL,
	template_id 	UUID 						NOT NULL,
	targets 		UUID[],
	title			TEXT 						NOT NULL,
	content 		TEXT 						NOT NULL,
	icon 			TEXT 						NOT NULL,
	actions 		JSONB 						NOT NULL,
	read_at 		TIMESTAMP WITH TIME ZONE,
	created_at 		TIMESTAMP WITH TIME ZONE 	NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_inbox_user_id_read_at ON notifications_inbox(user_id, read_at);
CREATE INDEX idx_notifications_inbox_user_id_template_id_targets ON notifications_inbox(user_id, template_id, targets);
