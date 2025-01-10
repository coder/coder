ALTER TABLE notification_templates ADD COLUMN enabled_by_default boolean DEFAULT TRUE NOT NULL;

CREATE OR REPLACE FUNCTION inhibit_enqueue_if_disabled()
	RETURNS TRIGGER AS
$$
BEGIN
	-- Fail the insertion if the user has disabled this notification.
	IF EXISTS (SELECT 1
			   FROM notification_preferences
			   WHERE disabled = TRUE
				 AND user_id = NEW.user_id
				 AND notification_template_id = NEW.notification_template_id) THEN
		RAISE EXCEPTION 'cannot enqueue message: user has disabled this notification';
	END IF;

	-- Fails if the notification template is disabled by default and the
	-- user hasn't explicitly enabled it.
	IF (NOT EXISTS (SELECT 1
				   FROM notification_preferences
				   WHERE user_id = NEW.user_id
					 AND notification_template_id = NEW.notification_template_id))
		AND (EXISTS (SELECT 1
					 FROM notification_templates
					 WHERE id = NEW.notification_template_id
				        AND enabled_by_default = FALSE)) THEN
		RAISE EXCEPTION 'cannot enqueue message: user has not enabled this notification';
	END IF;

	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
