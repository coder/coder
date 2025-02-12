ALTER TABLE notification_templates ADD COLUMN enabled_by_default boolean DEFAULT TRUE NOT NULL;

CREATE OR REPLACE FUNCTION inhibit_enqueue_if_disabled()
	RETURNS TRIGGER AS
$$
BEGIN
	-- Fail the insertion if one of the following:
	--  * the user has disabled this notification.
	--  * the notification template is disabled by default and hasn't
	--    been explicitly enabled by the user.
	IF EXISTS (
		SELECT 1 FROM notification_templates
		LEFT JOIN notification_preferences
			ON  notification_preferences.notification_template_id = notification_templates.id
			AND notification_preferences.user_id = NEW.user_id
		WHERE notification_templates.id = NEW.notification_template_id AND (
			-- Case 1: The user has explicitly disabled this template
			notification_preferences.disabled = TRUE
			OR
			-- Case 2: The template is disabled by default AND the user hasn't enabled it
			(notification_templates.enabled_by_default = FALSE AND notification_preferences.notification_template_id IS NULL)
		)
	) THEN
		RAISE EXCEPTION 'cannot enqueue message: notification is not enabled';
	END IF;

	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
