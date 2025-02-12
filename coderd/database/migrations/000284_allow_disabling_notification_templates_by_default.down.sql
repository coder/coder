ALTER TABLE notification_templates DROP COLUMN enabled_by_default;

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

	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
